package formatters

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"
)

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
	qt.Check(t, qt.IsNil(res.Err))
	qt.Check(t, qt.Equals(res.Diagnostic, ""))
	qt.Check(t, qt.IsFalse(res.Skipped))

	argv, err := os.ReadFile(argvPath)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(argv), "format\n--write\n--no-errors-on-unmatched\n--\n"+abs+"\n"))
}
