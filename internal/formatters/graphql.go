package formatters

import (
	"context"
	"path/filepath"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

type graphqlRouter struct {
	biome         Formatter
	prettier      Formatter
	biomeDisabled bool
}

// NewGraphQLRouter returns the GraphQL formatter: biome when the project has a
// biome config and biome is enabled, prettier otherwise.
func NewGraphQLRouter(cfg config.Config) Formatter {
	return graphqlRouter{
		biome:         NewBiome(),
		prettier:      NewPrettier(),
		biomeDisabled: cfg.IsFormatterDisabled("biome"),
	}
}

func (graphqlRouter) Name() string { return "prettier" }

func (r graphqlRouter) Format(ctx context.Context, projectRoot, abs string) Result {
	if !r.biomeDisabled && cachedFindUpward(filepath.Dir(abs), projectRoot, "biome.json", "biome.jsonc") != "" {
		if _, err := lookPath("biome"); err == nil {
			return r.biome.Format(ctx, projectRoot, abs)
		}
		if _, err := lookPath("bunx"); err == nil {
			return r.biome.Format(ctx, projectRoot, abs)
		}
	}
	return r.prettier.Format(ctx, projectRoot, abs)
}
