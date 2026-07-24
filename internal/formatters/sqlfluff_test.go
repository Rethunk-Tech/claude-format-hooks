package formatters

import (
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
