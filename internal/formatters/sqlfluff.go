package formatters

import (
	"context"
	"fmt"
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

// NewSQLFluff returns the sqlfluffFormatter for .sql.
func NewSQLFluff() Formatter { return sqlfluffFormatter{} }

func (sqlfluffFormatter) Name() string { return "sqlfluff" }

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

	// No --force: it is the default as of sqlfluff 4, and passing it prints
	// a deprecation warning that would itself become diagnostic noise.
	ok, diag, raw := runExternalOutput(ctx, projectRoot, "sqlfluff", []string{"fix", "--", abs})
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
