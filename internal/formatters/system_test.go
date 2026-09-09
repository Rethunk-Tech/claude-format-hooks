package formatters

import (
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"
)

// systemFormatters is every formatter built on systemFormatter, paired with
// the binary each one prefers.
var systemFormatters = []struct {
	f         Formatter
	name      string
	preferred string
}{
	{NewClangFormat(), "clang-format", "clang-format"},
	{NewJava(), "google-java-format", "google-java-format"},
	{NewKotlin(), "ktlint", "ktlint"},
	{NewSwift(), "swift-format", "swift-format"},
	{NewRuby(), "rubocop", "rubocop"},
	{NewPHP(), "php-cs-fixer", "php-cs-fixer"},
	{NewNix(), "nixfmt", "nixfmt"},
	{NewLua(), "stylua", "stylua"},
}

func TestSystemFormatterNamesAndTools(t *testing.T) {
	for _, tc := range systemFormatters {
		qt.Check(t, qt.Equals(tc.f.Name(), tc.name))
		tools := tc.f.(Prober).Tools()
		qt.Assert(t, qt.Not(qt.HasLen(tools, 0)))
		qt.Check(t, qt.Equals(tools[0], tc.preferred),
			qt.Commentf("%s must try its preferred binary first", tc.name))
	}
}

func TestSystemFormattersSkipWhenToolMissing(t *testing.T) {
	isolateDiskCache(t)
	clearPath(t)
	abs := filepath.Join(t.TempDir(), "f.txt")
	for _, tc := range systemFormatters {
		res := tc.f.Format(t.Context(), t.TempDir(), abs)
		qt.Check(t, qt.IsTrue(res.Skipped), qt.Commentf("%s", tc.name))
		qt.Check(t, qt.Equals(res.Diagnostic, ""), qt.Commentf("%s", tc.name))
	}
}

// The whole point of the bins list is that a missing preferred tool falls
// through to the next candidate rather than skipping the file: a machine
// with only clang-format still formats Java, and only alejandra still
// formats Nix.
func TestSystemFormatterFallsThroughToAlternative(t *testing.T) {
	cases := []struct {
		f        Formatter
		fallback string
		wantArg  string
	}{
		{NewJava(), "clang-format", "-i"},
		{NewSwift(), "swiftformat", "--quiet"},
		{NewNix(), "alejandra", ""},
		{NewRuby(), "standardrb", "--fix"},
		{NewPHP(), "pint", "--quiet"},
	}
	for _, tc := range cases {
		t.Run(tc.fallback, func(t *testing.T) {
			isolateDiskCache(t)
			dir, path := sourceFile(t, "src", "x\n")
			argv := filepath.Join(dir, "argv")
			writeFakeTool(t, tc.fallback, `printf '%s' "$*" > `+argv)

			res := tc.f.Format(t.Context(), dir, path)
			assertFormatted(t, res)
			got := string(readSource(t, argv))
			qt.Check(t, qt.IsTrue(len(got) > 0))
			if tc.wantArg != "" {
				qt.Check(t, qt.StringContains(got, tc.wantArg))
			}
			qt.Check(t, qt.StringContains(got, path),
				qt.Commentf("the file must be the last argument"))
		})
	}
}

func TestSystemFormatterReportsToolFailure(t *testing.T) {
	isolateDiskCache(t)
	dir, path := sourceFile(t, "a.lua", "x\n")
	writeFakeTool(t, "stylua", "echo 'syntax error' >&2; exit 1")

	res := NewLua().Format(t.Context(), dir, path)
	assertDiagnostic(t, res)
	qt.Check(t, qt.IsFalse(res.Skipped))
}
