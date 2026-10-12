package formatters

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// sqlfluffFormatter shells out to the system `sqlfluff` binary (Python
// tool, not npm-published — no bunx path, and no Go equivalent exists).
//
// `sqlfluff fix` exits non-zero for two very different reasons, and only
// one of them is a failure:
//
//   - It rewrote the file but some violations were not auto-fixable. The
//     file on disk is correct and improved; nothing here can act on the
//     remainder. Reporting that as a hook failure put a diagnostic on every
//     such .sql write.
//   - It could not parse the file at all. Nothing was fixed and the SQL is
//     genuinely broken — worth surfacing.
//
// Exit codes do not distinguish them (both are 1), so the output does: a
// parse failure is reported under sqlfluff's PRS rule and counted as a
// templating/parsing error, neither of which appears for ordinary
// unfixable-lint output.
type sqlfluffFormatter struct{}

// NewSQLFluff returns the sqlfluffFormatter for .sql and .sql.example.
func NewSQLFluff() sqlfluffFormatter { return sqlfluffFormatter{} }

func (sqlfluffFormatter) Name() string { return "sqlfluff" }

func (sqlfluffFormatter) Tools() []string { return []string{"sqlfluff"} }

// sqlfluffParseFailureMarkers appear in `sqlfluff fix` output only when the
// file could not be parsed. Matching two independent markers rather than
// one keeps a wording change in either from silently reclassifying parse
// errors as success.
var sqlfluffParseFailureMarkers = []string{
	"templating/parsing errors",
	"Found unparsable section",
}

func (sqlfluffFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	if _, err := lookPath("sqlfluff"); err != nil {
		return Result{Skipped: true}
	}
	userSQLFluffConfig()

	// --exclude-rules replaces the configured exclude_rules instead of
	// extending it, so the effective list is read back from sqlfluff's own
	// merged config (render is the cheapest command that prints it) and
	// carried forward alongside the unsafe fixers.
	render := exec.CommandContext(ctx, "sqlfluff", "render", "-vv", "--", abs) //nolint:gosec // external formatter by design; args are our own construction, never a shell
	render.Dir = projectRoot
	// A failed render (no dialect, bad config) still yields RF03 below, and
	// the fix call then reports the real error.
	rendered, _ := render.CombinedOutput()
	exclude := sqlfluffExcludeRules(string(rendered))
	// sqlfluff's sql_file_exts stop at .sql, so a copy whose final suffix
	// is .example is named on the command line and then skipped. Stdin is
	// parsed as SQL; --stdin-filename keeps config lookup on the real file.
	if sqlExampleCopy(abs) {
		return sqlfluffFixExample(ctx, projectRoot, abs, exclude)
	}
	// No --force: it is the default as of sqlfluff 4, and passing it prints
	// a deprecation warning that would itself become diagnostic noise.
	ok, diag, raw := runExternalOutput(ctx, projectRoot, "sqlfluff",
		[]string{"fix", "--exclude-rules", exclude, "--", abs})
	return sqlfluffOutcome(projectRoot, ok, diag, raw)
}

func sqlExampleCopy(abs string) bool {
	return strings.HasSuffix(strings.ToLower(filepath.Base(abs)), ".sql.example")
}

func sqlfluffFixExample(ctx context.Context, projectRoot, abs, exclude string) Result {
	src, err := os.ReadFile(abs) //nolint:gosec // abs is the file this formatter was invoked to format, by design
	if err != nil {
		return Result{Err: fmt.Errorf("read: %w", err)}
	}
	cmd := exec.CommandContext(ctx, "sqlfluff", "fix", "--exclude-rules", exclude, "--stdin-filename", abs, "-") //nolint:gosec // external formatter by design; args are our own construction, never a shell
	cmd.Dir = projectRoot
	cmd.Stdin = bytes.NewReader(src)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err == nil {
		return writeIfChanged(abs, src, stdout.Bytes(), 0o644)
	}
	if ctx.Err() != nil {
		return Result{Diagnostic: context.Cause(ctx).Error()}
	}
	raw := stderr.String()
	diag := err.Error()
	if raw != "" {
		diag = truncate(stderr.Bytes(), 10, 500)
	}
	// A non-zero exit with a rewritten stdout is the unfixable-violation
	// case: the in-place path already treats that as success, and stdin
	// has not written the file yet.
	outcome := sqlfluffOutcome(projectRoot, false, diag, raw)
	if outcome.Diagnostic != "" || stdout.Len() == 0 {
		return outcome
	}
	return writeIfChanged(abs, src, stdout.Bytes(), 0o644)
}

func sqlfluffOutcome(projectRoot string, ok bool, diag, raw string) Result {
	if ok {
		return Result{}
	}
	if sqlfluffNoDialect(raw) {
		return Result{Diagnostic: fmt.Sprintf(
			"sqlfluff has no dialect; set [sqlfluff] dialect in %s",
			filepath.Join(projectRoot, ".sqlfluff"),
		)}
	}
	// No output at all is not classifiable, and sqlfluff always says
	// something when it actually runs -- so an empty non-zero exit means it
	// died rather than linted. Report those rather than swallowing them.
	if raw == "" || sqlfluffFailedToParse(raw) {
		return Result{Diagnostic: diag}
	}
	// Ran, rewrote what it could, left some violations behind: success.
	return Result{}
}

// sqlfluffUnsafeFixRules are never applied by fix because their rewrites can
// change what a query means. RF03 qualifies an unqualified column with the
// only table it sees in a subquery, but in a correlated subquery whose outer
// table sqlfluff does not count (a CREATE POLICY ... USING (EXISTS ...)) that
// column belongs to the outer table, so the rewrite retargets it.
const sqlfluffUnsafeFixRules = "RF03"

// sqlfluffExcludeRules returns the configured exclude_rules from `sqlfluff
// render -vv` output plus sqlfluffUnsafeFixRules. The config dump precedes
// the rendered SQL, so only the first exclude_rules key is read; a
// multi-line value prints its continuation lines unindented.
func sqlfluffExcludeRules(renderOutput string) string {
	var parts []string
	inValue := false
	for line := range strings.Lines(renderOutput) {
		line = strings.TrimRight(line, " \r\n")
		if inValue {
			if line == "" || strings.HasPrefix(line, " ") {
				break
			}
			parts = append(parts, strings.TrimSpace(line))
			continue
		}
		if value, found := strings.CutPrefix(strings.TrimSpace(line), "exclude_rules:"); found && strings.HasPrefix(line, " ") {
			parts = append(parts, strings.TrimSpace(value))
			inValue = true
		}
	}
	configured := strings.Trim(strings.Join(parts, ""), ", ")
	if configured == "" {
		return sqlfluffUnsafeFixRules
	}
	return configured + "," + sqlfluffUnsafeFixRules
}

func sqlfluffNoDialect(output string) bool {
	return strings.Contains(strings.ToLower(output), "no dialect")
}

func sqlfluffFailedToParse(output string) bool {
	for _, marker := range sqlfluffParseFailureMarkers {
		if strings.Contains(output, marker) {
			return true
		}
	}
	return false
}
