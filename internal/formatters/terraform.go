package formatters

import "context"

// terraformFormatter shells out to the system terraform binary's `fmt`
// subcommand for .tf — Terraform/HCL has no Go equivalent invokable
// in-process without vendoring HashiCorp's own hclwrite package, and
// `terraform fmt` is the single canonical formatter for the language:
// unlike e.g. Kotlin, there's no competing tool to weigh.
type terraformFormatter struct{}

// NewTerraform returns the terraformFormatter for .tf.
func NewTerraform() Formatter { return terraformFormatter{} }

func (terraformFormatter) Name() string { return "terraform" }

func (terraformFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	if _, err := lookPath("terraform"); err != nil {
		return Result{Skipped: true}
	}
	ok, diag := runExternal(ctx, projectRoot, "terraform", []string{"fmt", abs})
	if !ok {
		return Result{Diagnostic: diag}
	}
	return Result{}
}
