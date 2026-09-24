package formatters

import (
	"os"
	"os/exec"
	"path/filepath"
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

	got, err := os.ReadFile(abs) //nolint:gosec // test path is created under t.TempDir
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
	got, err := os.ReadFile(argsLog) //nolint:gosec // test path is created under t.TempDir
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.StringContains(string(got), "fix --exclude-rules PG01,LT05,RF03 --"))
}

func TestSQLFluffExcludeRulesWithoutConfiguredList(t *testing.T) {
	qt.Check(t, qt.Equals(sqlfluffExcludeRules(""), "RF03"))
	qt.Check(t, qt.Equals(sqlfluffExcludeRules("    exclude_rules:                          \n"), "RF03"))
}
