package formatters

import (
	"context"
	"path/filepath"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

// graphqlRouter sends .graphql and .gql to biome when the project has a biome
// config and a usable launcher, and to prettier otherwise. Its Name remains
// "prettier" even when Format delegates to biome, so callers applying project
// formatter opt-outs must account for the delegated formatter name; disabling
// biome is therefore enforced here rather than by registry name filtering.
type graphqlRouter struct {
	biome         Formatter
	prettier      Formatter
	biomeDisabled bool
}

// NewGraphQLRouter returns the GraphQL formatter: biome when the project has a
// biome config and biome is enabled, prettier otherwise. Its Name remains
// "prettier" so disabledFormatters: ["prettier"] removes GraphQL from the
// registry while disabledFormatters: ["biome"] leaves the prettier fallback.
func NewGraphQLRouter(cfg config.Config) Formatter {
	return graphqlRouter{
		biome:         NewBiome(),
		prettier:      NewPrettier(),
		biomeDisabled: cfg.IsFormatterDisabled("biome"),
	}
}

// Name returns "prettier", the fallback formatter and registry identity.
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
