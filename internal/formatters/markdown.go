package formatters

import (
	"context"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

// markdownRouter fixes Markdown with markdownlint-cli2 and, in a Biome
// project, formats it with Biome last. Biome pads table columns that
// markdownlint's compact table style collapses, and such a project's own
// check is Biome's, so Biome must have the final word or every edit leaves
// the file failing that check.
type markdownRouter struct {
	lint          bunxFormatter
	biome         biomeFormatter
	biomeDisabled bool
}

// NewMarkdownRouter returns the formatter for .md, .mdx, .mdc and their
// .example copies.
func NewMarkdownRouter(cfg config.Config) markdownRouter {
	return markdownRouter{
		lint:          NewMarkdown(),
		biome:         NewBiome(),
		biomeDisabled: cfg.IsFormatterDisabled("biome"),
	}
}

func (r markdownRouter) Name() string { return r.lint.Name() }

func (r markdownRouter) Tools() []string { return r.lint.Tools() }

func (r markdownRouter) Format(ctx context.Context, projectRoot, abs string) Result {
	res := r.lint.Format(ctx, projectRoot, abs)
	if res.Err != nil || !biomeUsable(projectRoot, abs, r.biomeDisabled) {
		return res
	}
	return r.biome.Format(ctx, projectRoot, abs)
}
