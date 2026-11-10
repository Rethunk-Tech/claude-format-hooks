package formatters

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-quicktest/qt"
)

// These fixtures are trimmed from real `sqlfluff fix` output. Both cases
// exit 1, which is why the classification has to read the output at all --
// see sqlfluff.go.
const (
	sqlfluffUnfixableOutput = `==== finding fixable violations ====
== [m.sql] ==
L:   2 | P:  10 | CP01 | Keywords must be consistently upper case.
                       | [capitalisation.keywords]
== [m.sql] FIXED
4 fixable linting violations found
  [1 unfixable linting violations found]
All Finished!`

	sqlfluffUnparsableOutput = `==== finding fixable violations ====
  [1 templating/parsing errors found]
  == [broken.sql] ==
  L:   1 | P:   1 |  PRS | Line 1, Position 1: Found unparsable section: 'this is
                       | not valid sql at all @@@ ;'
==== no fixable linting violations found ====
All Finished!`
)

func TestSQLFluffTreatsUnfixableViolationsAsSuccess(t *testing.T) {
	// The file was rewritten correctly; the leftovers are things sqlfluff
	// declined to auto-fix. Surfacing that as a hook failure put a
	// diagnostic on every affected .sql write.
	qt.Check(t, qt.IsFalse(sqlfluffFailedToParse(sqlfluffUnfixableOutput)))
}

func TestSQLFluffTreatsUnparsableInputAsFailure(t *testing.T) {
	// Nothing was fixed and the SQL is genuinely broken -- the one non-zero
	// exit that is worth the operator's attention.
	qt.Check(t, qt.IsTrue(sqlfluffFailedToParse(sqlfluffUnparsableOutput)))
}

// The config dump costs a process startup (~275ms measured) on every .sql
// write, and the stdout path bypassed sqlfluff's own large-file limit -- a
// 34KB .sql.example spent 71s in a 4s budget and reported a timeout forever.
func TestSQLFluffCachesItsSettingsAndSkipsOversizedStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script tools are POSIX-shell only")
	}
	resetSQLFluffConfigForTest()
	isolateDiskCache(t)
	t.Setenv("HOME", t.TempDir())

	argvPath := filepath.Join(t.TempDir(), "argv")
	t.Setenv("SQLFLUFF_ARGV", argvPath)
	writeFakeTool(t, "sqlfluff", `printf '%s\n' "$@" >> "$SQLFLUFF_ARGV"
if [ "$1" = render ]; then
  printf '  exclude_rules: LT01\n  large_file_skip_byte_limit: 16\n'
fi`)

	dir := t.TempDir()
	small := filepath.Join(dir, "small.sql.example")
	qt.Assert(t, qt.IsNil(os.WriteFile(small, []byte("select 1;\n"), 0o600)))
	res := NewSQLFluff().Format(t.Context(), dir, small)
	assertFormatted(t, res)

	// A second file in the same project must reuse the cached dump rather
	// than paying for another render.
	other := filepath.Join(dir, "other.sql")
	qt.Assert(t, qt.IsNil(os.WriteFile(other, []byte("select 1;\n"), 0o600)))
	assertFormatted(t, NewSQLFluff().Format(t.Context(), dir, other))
	qt.Check(t, qt.Equals(strings.Count(string(readSource(t, argvPath)), "render\n"), 1),
		qt.Commentf("the exclude_rules dump must be cached per project root, not re-read per file"))
	qt.Check(t, qt.StringContains(string(readSource(t, argvPath)), "--exclude-rules\nLT01,RF03"),
		qt.Commentf("the cached dump still carries the configured rules"))

	// A file past sqlfluff's own large_file_skip_byte_limit is one sqlfluff
	// would decline to lint; the stdin path has to decline it too instead of
	// blowing the formatter budget and reporting a timeout on every write.
	big := filepath.Join(dir, "big.sql.example")
	bigSrc := strings.Repeat("select 1;\n", 4)
	qt.Assert(t, qt.IsNil(os.WriteFile(big, []byte(bigSrc), 0o600)))
	before := readSource(t, argvPath)
	res = NewSQLFluff().Format(t.Context(), dir, big)
	qt.Check(t, qt.IsTrue(res.Skipped), qt.Commentf("an oversized file is a skip, not a timeout"))
	qt.Check(t, qt.Equals(res.Diagnostic, ""))
	qt.Check(t, qt.DeepEquals(readSource(t, argvPath), before), qt.Commentf("no tool run for a file sqlfluff would skip"))
	qt.Check(t, qt.Equals(string(readSource(t, big)), bigSrc))
}

func TestSQLFluffLargeFileSkipByteLimit(t *testing.T) {
	tests := []struct {
		name string
		dump string
		want int64
	}{
		{name: "configured value", dump: "  large_file_skip_byte_limit: 40000\n", want: 40000},
		{name: "absent key uses sqlfluff's default", dump: "  exclude_rules: LT01\n", want: sqlfluffDefaultLargeFileSkipByteLimit},
		{name: "top-level text is not config", dump: "large_file_skip_byte_limit: 1\n", want: sqlfluffDefaultLargeFileSkipByteLimit},
		{name: "non-numeric value uses the default", dump: "  large_file_skip_byte_limit: none\n", want: sqlfluffDefaultLargeFileSkipByteLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qt.Check(t, qt.Equals(sqlfluffLargeFileSkipByteLimit(tt.dump), tt.want))
		})
	}
}

func TestSQLFluffParseFailureMatchesEitherMarkerAlone(t *testing.T) {
	// Two independent markers are matched so a wording change in one does
	// not silently reclassify parse errors as success. Each must therefore
	// be sufficient on its own, or the redundancy buys nothing.
	for _, marker := range sqlfluffParseFailureMarkers {
		qt.Check(t, qt.IsTrue(sqlfluffFailedToParse("noise\n"+marker+"\nmore noise")),
			qt.Commentf("marker %q must be sufficient by itself", marker))
	}
}

func TestSQLFluffCleanOutputIsNotAParseFailure(t *testing.T) {
	qt.Check(t, qt.IsFalse(sqlfluffFailedToParse("All Finished!")))
	qt.Check(t, qt.IsFalse(sqlfluffFailedToParse("")))
}

// The unqualified credential_id belongs to the policy's table, not to the
// subquery's. RF03 sees one table in the subquery and qualifies it as
// c.credential_id, which silently changes the policy's meaning.
const sqlfluffCorrelatedPolicy = `create policy credential_access on credential_grant
for select
using (
    exists (
        select 1
        from credential as c
        where c.owner_id = auth.uid()
          and credential_id = credential_grant.credential_id
    )
);
`

func TestSQLFluffFixKeepsCorrelatedReferencesUnqualified(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the real sqlfluff")
	}
	if _, err := exec.LookPath("sqlfluff"); err != nil {
		t.Skip("sqlfluff not installed")
	}
	isolateDiskCache(t)
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, ".sqlfluff"), []byte("[sqlfluff]\ndialect = postgres\n"), 0o600)))
	abs := filepath.Join(dir, "policy.sql")
	qt.Assert(t, qt.IsNil(os.WriteFile(abs, []byte(sqlfluffCorrelatedPolicy), 0o600)))

	res := NewSQLFluff().Format(t.Context(), dir, abs)
	qt.Assert(t, qt.Equals(res.Diagnostic, ""))

	got, err := os.ReadFile(filepath.Clean(abs))
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.StringContains(string(got), "and credential_id = credential_grant.credential_id"))
	qt.Check(t, qt.IsFalse(strings.Contains(string(got), "c.credential_id")))
}

func TestSQLFluffFixKeepsConfiguredExclusions(t *testing.T) {
	// --exclude-rules replaces the configured list rather than adding to
	// it, so the configured rules must be carried into the fix call.
	isolateDiskCache(t)
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	argsLog := filepath.Join(dir, "args")
	writeFakeTool(t, "sqlfluff", `if [ "$1" = render ]; then
printf '    encoding:           autodetect          \n    exclude_rules:      PG01,\nLT05          \n    fix_even_unparsable:False\n    exclude_rules:      from the rendered file\n'
exit 0
fi
echo "$@" > `+argsLog)

	res := NewSQLFluff().Format(t.Context(), dir, filepath.Join(dir, "f.sql"))
	qt.Assert(t, qt.Equals(res.Diagnostic, ""))
	got, err := os.ReadFile(filepath.Clean(argsLog))
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.StringContains(string(got), "fix --exclude-rules PG01,LT05,RF03 --"))
}

func TestSQLFluffExampleUsesStdin(t *testing.T) {
	isolateDiskCache(t)
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	argsLog := filepath.Join(dir, "args")
	abs := filepath.Join(dir, "query.sql.example")
	qt.Assert(t, qt.IsNil(os.WriteFile(abs, []byte("SELECT  1\n"), 0o600)))
	writeFakeTool(t, "sqlfluff", `if [ "$1" = render ]; then
printf '    exclude_rules: LT01\n'
exit 0
fi
printf '%s\n' "$@" > "`+argsLog+`"
printf 'SELECT 1\n'
`)

	res := NewSQLFluff().Format(t.Context(), dir, abs)
	assertFormatted(t, res)
	qt.Check(t, qt.Equals(string(readSource(t, abs)), "SELECT 1\n"))
	got := string(readSource(t, argsLog))
	qt.Check(t, qt.StringContains(got, "--stdin-filename"))
	qt.Check(t, qt.StringContains(got, abs))
	qt.Check(t, qt.StringContains(got, "LT01,RF03"))
}

func TestSQLFluffExampleKeepsUnfixableRewrite(t *testing.T) {
	isolateDiskCache(t)
	t.Setenv("HOME", t.TempDir())
	dir, abs := sourceFile(t, "query.sql.example", "SELECT  1\n")
	writeFakeTool(t, "sqlfluff", `if [ "$1" = render ]; then exit 0; fi
echo 'Unfixable violations detected.' >&2
printf 'SELECT 1\n'
exit 1
`)

	res := NewSQLFluff().Format(t.Context(), dir, abs)
	assertFormatted(t, res)
	qt.Check(t, qt.Equals(string(readSource(t, abs)), "SELECT 1\n"))
}

func TestSQLFluffExampleReportsParseFailure(t *testing.T) {
	isolateDiskCache(t)
	t.Setenv("HOME", t.TempDir())
	dir, abs := sourceFile(t, "query.sql.example", "nope\n")
	writeFakeTool(t, "sqlfluff", `if [ "$1" = render ]; then exit 0; fi
echo 'Found unparsable section' >&2
printf 'nope\n'
exit 1
`)

	res := NewSQLFluff().Format(t.Context(), dir, abs)
	assertDiagnostic(t, res)
	qt.Check(t, qt.Equals(string(readSource(t, abs)), "nope\n"))
}

func TestSQLFluffExampleReportsMissingDialect(t *testing.T) {
	isolateDiskCache(t)
	t.Setenv("HOME", t.TempDir())
	dir, abs := sourceFile(t, "query.sql.example", "SELECT 1\n")
	writeFakeTool(t, "sqlfluff", `if [ "$1" = render ]; then exit 0; fi
echo 'User Error: No dialect was specified' >&2
exit 1
`)

	res := NewSQLFluff().Format(t.Context(), dir, abs)
	assertDiagnostic(t, res)
	qt.Check(t, qt.StringContains(res.Diagnostic, ".sqlfluff"))
	qt.Check(t, qt.Equals(string(readSource(t, abs)), "SELECT 1\n"))
}

func TestSQLFluffExcludeRulesWithoutConfiguredList(t *testing.T) {
	qt.Check(t, qt.Equals(sqlfluffExcludeRules(""), "RF03"))
	qt.Check(t, qt.Equals(sqlfluffExcludeRules("    exclude_rules:                          \n"), "RF03"))
}
