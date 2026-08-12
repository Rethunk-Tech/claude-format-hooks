package formatters

import "context"

// terraformFormatter shells out to the system terraform or OpenTofu binary's
// `fmt` subcommand for .tf, .tfvars, .tftest.hcl, .tfmock.hcl, and
// .tfquery.hcl — Terraform/HCL has no Go equivalent invokable in-process
// without vendoring HashiCorp's own hclwrite package, and `terraform fmt` or
// `tofu fmt` is the single canonical formatter for the language: unlike e.g.
// Kotlin, there's no competing tool to weigh.
type terraformFormatter struct{}

// NewTerraform returns the terraformFormatter for Terraform configuration,
// variable, test, mock, and query files.
func NewTerraform() Formatter { return terraformFormatter{} }

func (terraformFormatter) Name() string { return "terraform" }

func (terraformFormatter) Format(ctx context.Context, projectRoot, abs string) Result {
	bin := "terraform"
	if _, err := lookPath(bin); err != nil {
		bin = "tofu"
		if _, err := lookPath(bin); err != nil {
			return Result{Skipped: true}
		}
	}
	ok, diag := runExternal(ctx, projectRoot, bin, []string{"fmt", abs})
	if !ok {
		return Result{Diagnostic: diag}
	}
	return Result{}
}
