package formatters

import (
	"os"
	"os/exec"
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
	{NewGoMod(), "go mod edit", "go"},
	{NewGoWork(), "go work edit", "go"},
}

func TestSystemFormatterNamesAndTools(t *testing.T) {
	for _, tc := range systemFormatters {
		qt.Check(t, qt.Equals(tc.f.Name(), tc.name))
		prober, ok := tc.f.(Prober)
		if !ok {
			t.Fatalf("%s does not implement Prober", tc.name)
		}
		tools := prober.Tools()
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

func TestGoModAndWorkEditOnlyTheirOwnFile(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	gomod := filepath.Join(dir, "go.mod")
	otherMod := filepath.Join(dir, "other.mod")
	gowork := filepath.Join(dir, "go.work")
	otherWork := filepath.Join(dir, "notes.work")
	for _, path := range []string{gomod, otherMod, gowork, otherWork} {
		qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte("x\n"), 0o600)))
	}
	argv := filepath.Join(dir, "argv")
	writeFakeTool(t, "go", `printf '%s' "$*" > `+argv)

	res := NewGoMod().Format(t.Context(), dir, gomod)
	assertFormatted(t, res)
	assertArgv(t, argv, "mod edit -fmt "+gomod)

	qt.Assert(t, qt.IsNil(os.Remove(argv)))
	res = NewGoMod().Format(t.Context(), dir, otherMod)
	qt.Check(t, qt.IsTrue(res.Skipped))
	_, err := os.Stat(argv)
	qt.Check(t, qt.IsTrue(os.IsNotExist(err)))

	res = NewGoWork().Format(t.Context(), dir, gowork)
	assertFormatted(t, res)
	assertArgv(t, argv, "work edit -fmt "+gowork)

	qt.Assert(t, qt.IsNil(os.Remove(argv)))
	res = NewGoWork().Format(t.Context(), dir, otherWork)
	qt.Check(t, qt.IsTrue(res.Skipped))
	_, err = os.Stat(argv)
	qt.Check(t, qt.IsTrue(os.IsNotExist(err)))

	// --check copies beside the original as .fmtcheck-<8 hex>-<base>.
	// That copy is the file the command must rewrite; a lookalike name is not.
	scratch := filepath.Join(dir, ".fmtcheck-0123abcd-go.mod")
	qt.Assert(t, qt.IsNil(os.WriteFile(scratch, []byte("x\n"), 0o600)))
	res = NewGoMod().Format(t.Context(), dir, scratch)
	assertFormatted(t, res)
	assertArgv(t, argv, "mod edit -fmt "+scratch)

	lookalike := filepath.Join(dir, "notgo.mod")
	qt.Assert(t, qt.IsNil(os.WriteFile(lookalike, []byte("x\n"), 0o600)))
	qt.Assert(t, qt.IsNil(os.Remove(argv)))
	res = NewGoMod().Format(t.Context(), dir, lookalike)
	qt.Check(t, qt.IsTrue(res.Skipped))
	_, err = os.Stat(argv)
	qt.Check(t, qt.IsTrue(os.IsNotExist(err)))
}

func TestGoModAndWorkFormatInPlace(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	isolateDiskCache(t)
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	qt.Assert(t, qt.IsNil(os.Mkdir(sub, 0o750)))

	messyMod := "module example.com/x\n\ngo 1.22\n\nrequire (\nexample.com/b v1.2.3\nexample.com/a v1.0.0\n)\n"
	messyWork := "go 1.22\n\nuse (\n./b\n./a\n)\n"
	parentMod := filepath.Join(root, "go.mod")
	childMod := filepath.Join(sub, "go.mod")
	otherMod := filepath.Join(root, "other.mod")
	work := filepath.Join(root, "go.work")
	otherWork := filepath.Join(root, "notes.work")
	for _, path := range []string{parentMod, childMod, otherMod} {
		qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(messyMod), 0o600)))
	}
	for _, path := range []string{work, otherWork} {
		qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(messyWork), 0o600)))
	}

	res := NewGoMod().Format(t.Context(), root, childMod)
	assertFormatted(t, res)
	qt.Check(t, qt.StringContains(string(readSource(t, childMod)), "\texample.com/a v1.0.0\n"))
	qt.Check(t, qt.Equals(string(readSource(t, parentMod)), messyMod))
	qt.Check(t, qt.Equals(string(readSource(t, otherMod)), messyMod))

	res = NewGoWork().Format(t.Context(), root, work)
	assertFormatted(t, res)
	qt.Check(t, qt.StringContains(string(readSource(t, work)), "\t./a\n"))
	qt.Check(t, qt.Equals(string(readSource(t, otherWork)), messyWork))
	qt.Check(t, qt.Equals(string(readSource(t, parentMod)), messyMod))

	scratch := filepath.Join(root, ".fmtcheck-0123abcd-go.mod")
	qt.Assert(t, qt.IsNil(os.WriteFile(scratch, []byte(messyMod), 0o600)))
	res = NewGoMod().Format(t.Context(), root, scratch)
	assertFormatted(t, res)
	qt.Check(t, qt.StringContains(string(readSource(t, scratch)), "\texample.com/a v1.0.0\n"))
	qt.Check(t, qt.Equals(string(readSource(t, parentMod)), messyMod))
}

func TestSystemFormatterReportsToolFailure(t *testing.T) {
	isolateDiskCache(t)
	dir, path := sourceFile(t, "a.lua", "x\n")
	writeFakeTool(t, "stylua", "echo 'syntax error' >&2; exit 1")

	res := NewLua().Format(t.Context(), dir, path)
	assertDiagnostic(t, res)
	qt.Check(t, qt.IsFalse(res.Skipped))
}
