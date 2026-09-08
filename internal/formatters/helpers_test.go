package formatters

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"
)

// sourceFile writes src as name inside a fresh temp directory, returning
// both the directory (the project root a formatter is given) and the file.
func sourceFile(t *testing.T, name, src string) (dir, path string) {
	t.Helper()
	dir = t.TempDir()
	path = filepath.Join(dir, name)
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(src), 0o600))) //nolint:gosec // test fixture
	return dir, path
}

func readSource(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // path is a temp file the test wrote
	qt.Assert(t, qt.IsNil(err))
	return raw
}

// assertIdempotent formats twice and returns the settled output, checking a
// second pass changes nothing.
func assertIdempotent(t *testing.T, f Formatter, dir, path string) []byte {
	t.Helper()
	ctx := t.Context()

	f.Format(ctx, dir, path)
	first := readSource(t, path)

	f.Format(ctx, dir, path)
	second := readSource(t, path)

	qt.Check(t, qt.DeepEquals(first, second), qt.Commentf("not idempotent"))
	return first
}

// assertDirectoryIsAReadError checks a formatter reports a directory as a
// read failure rather than the "not my syntax" Skipped path.
func assertDirectoryIsAReadError(t *testing.T, f Formatter) {
	t.Helper()
	dir := t.TempDir()
	res := f.Format(t.Context(), dir, dir)
	qt.Check(t, qt.IsNotNil(res.Err))
	qt.Check(t, qt.IsFalse(res.Skipped))
}

// assertDirEntryCount checks how much a failed write left behind, which is
// how these tests prove the temporary file was cleaned up.
func assertDirEntryCount(t *testing.T, dir string, want int, why string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.HasLen(entries, want), qt.Commentf("%s", why))
}

// assertStillSymlink checks writeFormatted wrote through a symlink rather
// than replacing the link node itself.
func assertStillSymlink(t *testing.T, link string) {
	t.Helper()
	info, err := os.Lstat(link)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(info.Mode()&os.ModeSymlink, os.ModeSymlink),
		qt.Commentf("the symlink node must remain intact"))
}

// assertFormatted checks a formatter reported plain success: no error, no
// diagnostic, and not skipped.
func assertFormatted(t *testing.T, res Result) {
	t.Helper()
	qt.Check(t, qt.IsNil(res.Err))
	qt.Check(t, qt.Equals(res.Diagnostic, ""))
	qt.Check(t, qt.IsFalse(res.Skipped))
}

// assertDiagnostic checks a formatter surfaced a failure to the operator.
func assertDiagnostic(t *testing.T, res Result) {
	t.Helper()
	qt.Check(t, qt.Not(qt.Equals(res.Diagnostic, "")))
}

// assertArgv checks the argument vector a fake tool recorded.
func assertArgv(t *testing.T, argvPath, want string) {
	t.Helper()
	qt.Check(t, qt.Equals(string(readSource(t, argvPath)), want))
}

// biomeProject is sourceFile plus a biome.json, so the directory reads as a
// Biome project to the routers.
func biomeProject(t *testing.T, name, src string) (dir, path string) {
	t.Helper()
	dir, path = sourceFile(t, name, src)
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, "biome.json"), []byte("{}\n"), 0o600)))
	return dir, path
}
