package formatters

import (
	"context"
	"os"
	"os/exec"
)

// sqlfluffFormatter shells out to the system `sqlfluff` binary (Python
// tool, not npm-published — no bunx path, and no Go equivalent exists).
type sqlfluffFormatter struct{}

func NewSQLFluff() Formatter { return sqlfluffFormatter{} }

func (sqlfluffFormatter) Name() string { return "sqlfluff" }

func (sqlfluffFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	if _, err := exec.LookPath("sqlfluff"); err != nil {
		return Result{Skipped: true}
	}
	before, _ := os.ReadFile(abs)
	ok, diag := runExternal(ctx, projectRoot, "sqlfluff", []string{"fix", "--force", "--", abs})
	if !ok {
		return Result{Diagnostic: diag}
	}
	after, _ := os.ReadFile(abs)
	return Result{Changed: string(before) != string(after)}
}
