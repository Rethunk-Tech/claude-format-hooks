package formatters

import (
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

// markdownRuns formats one Markdown file with stub markdownlint-cli2 and
// biome on PATH and returns the order they ran in.
func markdownRuns(t *testing.T, cfg config.Config, biomeConfig bool) string {
	t.Helper()
	isolateDiskCache(t)
	log := filepath.Join(t.TempDir(), "ran")
	t.Setenv("RAN_LOG", log)
	writeFakeTools(t, map[string]string{
		"markdownlint-cli2": `echo markdownlint >> "$RAN_LOG"`,
		"biome":             `echo biome >> "$RAN_LOG"`,
	})
	project := sourceFile
	if biomeConfig {
		project = biomeProject
	}
	dir, path := project(t, "README.md", "# Title\n")
	res := NewMarkdownRouter(cfg).Format(t.Context(), dir, path)
	qt.Assert(t, qt.IsNil(res.Err))
	return string(readSource(t, log))
}

func TestMarkdownRouterRunsBiomeLastInABiomeProject(t *testing.T) {
	qt.Check(t, qt.Equals(markdownRuns(t, config.Default(), true), "markdownlint\nbiome\n"))
}

func TestMarkdownRouterLeavesBiomeOutsideABiomeProject(t *testing.T) {
	qt.Check(t, qt.Equals(markdownRuns(t, config.Default(), false), "markdownlint\n"))
}

func TestMarkdownRouterHonoursABiomeDisable(t *testing.T) {
	cfg := config.Default()
	cfg.DisabledFormatters = []string{"biome"}
	qt.Check(t, qt.Equals(markdownRuns(t, cfg, true), "markdownlint\n"))
}
