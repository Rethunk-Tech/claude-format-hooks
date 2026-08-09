package formatters

import (
	"os"
	"path/filepath"
	"sync"
)

const userSQLFluffConfigFile = ".sqlfluff"

var sqlfluffDefaults = []byte(`[sqlfluff]
# Use the portable ANSI dialect unless a project config overrides it.
dialect = ansi
`)

var sqlfluffConfigOnce sync.Once
var sqlfluffConfigPath string

// userSQLFluffConfig materializes a user-level default once per process.
// SQLFluff loads ~/.sqlfluff before project config, so project settings still
// override this dialect through the tool's native configuration merge.
func userSQLFluffConfig() string {
	sqlfluffConfigOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		sqlfluffConfigPath = resolveSQLFluffConfig(home)
	})
	return sqlfluffConfigPath
}

func resolveSQLFluffConfig(home string) string {
	path := filepath.Join(home, userSQLFluffConfigFile)

	// An existing file may contain the user's dialect and must never be
	// overwritten by the hook.
	if _, err := os.Stat(path); err == nil {
		return path
	}

	if err := writeIfMissing(path, sqlfluffDefaults); err != nil {
		return ""
	}
	return path
}
