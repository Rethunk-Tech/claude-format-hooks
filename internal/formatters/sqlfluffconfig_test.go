package formatters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestResolveSQLFluffConfigMaterializesANSIDefault(t *testing.T) {
	home := t.TempDir()

	path := resolveSQLFluffConfig(home)

	qt.Assert(t, qt.Equals(path, filepath.Join(home, userSQLFluffConfigFile)))
	written, err := os.ReadFile(path) //nolint:gosec // path is the temp dir this test just wrote
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(written, sqlfluffDefaults))
	qt.Check(t, qt.IsTrue(strings.Contains(string(written), "dialect = ansi")))
}

func TestResolveSQLFluffConfigNeverOverwritesAnExistingFile(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, userSQLFluffConfigFile)
	edited := []byte("[sqlfluff]\ndialect = postgres\n")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, edited, 0o600))) //nolint:gosec // test fixture

	got := resolveSQLFluffConfig(home)

	qt.Assert(t, qt.Equals(got, path))
	after, err := os.ReadFile(path) //nolint:gosec // path is the temp dir this test just wrote
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
