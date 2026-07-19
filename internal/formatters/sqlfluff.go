package formatters

import "context"

// sqlfluffFormatter shells out to the system `sqlfluff` binary (Python
// tool, not npm-published — no bunx path, and no Go equivalent exists).
type sqlfluffFormatter struct{}

// NewSQLFluff returns the sqlfluffFormatter for .sql.
func NewSQLFluff() Formatter { return sqlfluffFormatter{} }

func (sqlfluffFormatter) Name() string { return "sqlfluff" }

func (sqlfluffFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	if _, err := lookPath("sqlfluff"); err != nil {
		return Result{Skipped: true}
	}
	ok, diag := runExternal(ctx, projectRoot, "sqlfluff", []string{"fix", "--force", "--", abs})
	if !ok {
		return Result{Diagnostic: diag}
	}
	return Result{}
}
