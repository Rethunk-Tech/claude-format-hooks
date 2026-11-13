package formatters

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestBiomeExampleCopyIncludesMarkdown(t *testing.T) {
	qt.Check(t, qt.IsTrue(biomeExampleCopy("README.md.example")))
	qt.Check(t, qt.Equals(exampleStdinFilePath("README.md.example"), "README.md"))
}

func TestBiomeUsesFormatSubcommand(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.ts")
	argvPath := filepath.Join(t.TempDir(), "argv")
	t.Setenv("BIOME_ARGV", argvPath)
	writeFakeTool(t, "biome", `case "$1" in
format) printf '%s\n' "$@" > "$BIOME_ARGV"; exit 0 ;;
check) echo "check must not run" >&2; exit 1 ;;
*) exit 1 ;;
esac`)

	res := NewBiome().Format(t.Context(), dir, abs)
	assertFormatted(t, res)

	assertArgv(t, argvPath, "format\n--write\n--no-errors-on-unmatched\n--\n"+abs+"\n")
}

func TestBiomeNamesJSONCExampleOnStdin(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	abs := filepath.Join(dir, "config.jsonc.example")
	qt.Assert(t, qt.IsNil(os.WriteFile(abs, []byte("{ \"a\":1, // c\n}\n"), 0o600)))
	argvPath := filepath.Join(t.TempDir(), "argv")
	t.Setenv("BIOME_ARGV", argvPath)
	writeFakeTool(t, "biome", `printf '%s\n' "$@" > "$BIOME_ARGV"
printf '{\n\t"a": 1 // c\n}\n'`)

	res := NewBiome().Format(t.Context(), dir, abs)
	assertFormatted(t, res)
	named := abs[:len(abs)-len(".example")]
	assertArgv(t, argvPath, "format\n--write\n--no-errors-on-unmatched\n--stdin-file-path\n"+named+"\n")
	qt.Check(t, qt.Equals(string(readSource(t, abs)), "{\n\t\"a\": 1 // c\n}\n"))
}

func TestBiomeNamesTSExampleOnStdin(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	abs := filepath.Join(dir, "app.ts.example")
	qt.Assert(t, qt.IsNil(os.WriteFile(abs, []byte("export const x=1\n"), 0o600)))
	argvPath := filepath.Join(t.TempDir(), "argv")
	t.Setenv("BIOME_ARGV", argvPath)
	writeFakeTool(t, "biome", `printf '%s\n' "$@" > "$BIOME_ARGV"
printf 'export const x = 1\n'`)

	res := NewBiome().Format(t.Context(), dir, abs)
	assertFormatted(t, res)
	named := abs[:len(abs)-len(".example")]
	assertArgv(t, argvPath, "format\n--write\n--no-errors-on-unmatched\n--stdin-file-path\n"+named+"\n")
	qt.Check(t, qt.Equals(string(readSource(t, abs)), "export const x = 1\n"))
}

func TestBiomeBunxUsesFormatSubcommand(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.ts")
	argvPath := filepath.Join(t.TempDir(), "argv")
	t.Setenv("BIOME_ARGV", argvPath)
	writeFakeTool(t, "bunx", `for arg do
	if [ "$arg" = check ]; then exit 1; fi
done
printf '%s\n' "$@" > "$BIOME_ARGV"
exit 0`)

	res := NewBiome().Format(t.Context(), dir, abs)
	assertFormatted(t, res)

	assertArgv(t, argvPath, "@biomejs/biome\nformat\n--write\n--no-errors-on-unmatched\n--\n"+abs+"\n")
}

// A project root spelled through a symlink must still bound the walk, or a
// biome.json in a parent directory of the real path configures the wrong repo.
func TestFindUpwardStopsAtSymlinkedRoot(t *testing.T) {
	base := t.TempDir()
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(base, "biome.json"), []byte("{}"), 0o600)))
	proj := filepath.Join(base, "proj", "pkg")
	qt.Assert(t, qt.IsNil(os.MkdirAll(proj, 0o750)))
	link := filepath.Join(t.TempDir(), "link")
	qt.Assert(t, qt.IsNil(os.Symlink(filepath.Join(base, "proj"), link)))

	qt.Check(t, qt.Equals(findUpward(proj, link, biomeConfigNames...), ""))
}

// The found directory keeps the caller's spelling: resolving it would turn a
// Windows 8.3 short name into the long name, which callers compare against.
func TestFindUpwardKeepsTheCallersSpelling(t *testing.T) {
	real := t.TempDir()
	qt.Assert(t, qt.IsNil(os.MkdirAll(filepath.Join(real, "a", "b"), 0o750)))
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(real, "a", "biome.json"), []byte("{}"), 0o600)))
	link := filepath.Join(t.TempDir(), "link")
	qt.Assert(t, qt.IsNil(os.Symlink(real, link)))

	qt.Check(t, qt.Equals(findUpward(filepath.Join(link, "a", "b"), link, biomeConfigNames...), filepath.Join(link, "a")))
}
