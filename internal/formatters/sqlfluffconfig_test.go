package formatters

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestResolveSQLFluffConfigMaterializesANSIDefault(t *testing.T) {
	home := t.TempDir()

	path := resolveSQLFluffConfig(home)

	qt.Assert(t, qt.Equals(path, filepath.Join(home, userSQLFluffConfigFile)))
	written, err := os.ReadFile(path)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(written, sqlfluffDefaults))
	qt.Check(t, qt.IsTrue(strings.Contains(string(written), "dialect = ansi")))
}

func TestResolveSQLFluffConfigNeverOverwritesAnExistingFile(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, userSQLFluffConfigFile)
	edited := []byte("[sqlfluff]\ndialect = postgres\n")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, edited, 0o600)))

	got := resolveSQLFluffConfig(home)

	qt.Assert(t, qt.Equals(got, path))
	after, err := os.ReadFile(path)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(after, edited), qt.Commentf("an existing user config must survive"))
}

func TestResolveSQLFluffConfigIsIdempotent(t *testing.T) {
	home := t.TempDir()

	first := resolveSQLFluffConfig(home)
	second := resolveSQLFluffConfig(home)

	qt.Check(t, qt.Equals(second, first))
}

func TestSQLFluffNoDialectDiagnostic(t *testing.T) {
	qt.Check(t, qt.IsTrue(sqlfluffNoDialect("User Error: No dialect was specified")))
	qt.Check(t, qt.IsFalse(sqlfluffNoDialect("All Finished!")))
}

func TestSQLFluffFormatMaterializesUserConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script tools are POSIX-shell only")
	}
	resetSQLFluffConfigForTest()
	isolateDiskCache(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFakeTool(t, "sqlfluff", "exit 0")
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.sql")
	qt.Assert(t, qt.IsNil(os.WriteFile(abs, []byte("select 1;\n"), 0o644)))

	res := NewSQLFluff().Format(t.Context(), dir, abs)

	assertFormatted(t, res)
	written, err := os.ReadFile(filepath.Join(home, userSQLFluffConfigFile))
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(written, sqlfluffDefaults))
}

func TestSQLFluffFormatPreservesExistingUserConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script tools are POSIX-shell only")
	}
	resetSQLFluffConfigForTest()
	isolateDiskCache(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	existing := []byte("[sqlfluff]\ndialect = postgres\n")
	configPath := filepath.Join(home, userSQLFluffConfigFile)
	qt.Assert(t, qt.IsNil(os.WriteFile(configPath, existing, 0o600)))
	writeFakeTool(t, "sqlfluff", "exit 0")
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.sql")
	qt.Assert(t, qt.IsNil(os.WriteFile(abs, []byte("select 1;\n"), 0o644)))

	res := NewSQLFluff().Format(t.Context(), dir, abs)

	assertFormatted(t, res)
	written, err := os.ReadFile(configPath)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(written, existing))
}

func resetSQLFluffConfigForTest() {
	sqlfluffConfigOnce = sync.Once{}
	sqlfluffConfigPath = ""
}
