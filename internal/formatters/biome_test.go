package formatters

import (
	"path/filepath"
	"testing"
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
	assertFormatted(t, res)

	assertArgv(t, argvPath, "format\n--write\n--no-errors-on-unmatched\n--\n"+abs+"\n")
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
