package formatters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestResolveMarkdownlintConfigMaterializesDefaults(t *testing.T) {
	home := t.TempDir()

	path := resolveMarkdownlintConfig(home)

	qt.Assert(t, qt.Equals(path, filepath.Join(home, ".claude", userMarkdownlintConfigFile)))
	written, err := os.ReadFile(path) //nolint:gosec // path is the temp dir this test just wrote
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(written, markdownlintDefaults), qt.Commentf("materialized file must be the embedded defaults verbatim"))
}

func TestResolveMarkdownlintConfigNeverOverwritesAnEditedFile(t *testing.T) {
	// The whole point of materializing into ~/.claude rather than a cache
	// directory is that the user can tune it; silently restoring the
	// defaults on the next .md write would make that pointless.
	home := t.TempDir()
	path := filepath.Join(home, ".claude", userMarkdownlintConfigFile)
	qt.Assert(t, qt.IsNil(os.MkdirAll(filepath.Dir(path), 0o755)))
	edited := []byte(`{ "config": { "MD013": true } }`)
	qt.Assert(t, qt.IsNil(os.WriteFile(path, edited, 0o644))) //nolint:gosec // test fixture

	got := resolveMarkdownlintConfig(home)

	qt.Assert(t, qt.Equals(got, path))
	after, err := os.ReadFile(path) //nolint:gosec // path is the temp dir this test just wrote
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(after, edited), qt.Commentf("an existing config is the user's and must survive"))
}

func TestResolveMarkdownlintConfigIsIdempotent(t *testing.T) {
	home := t.TempDir()

	first := resolveMarkdownlintConfig(home)
	second := resolveMarkdownlintConfig(home)

	qt.Check(t, qt.Equals(second, first))
}

func TestResolveMarkdownlintConfigReturnsEmptyWhenUnwritable(t *testing.T) {
	// A home directory that can't hold a .claude directory must degrade to
	// "no --config" rather than failing the write that triggered the hook.
	home := t.TempDir()
	blocker := filepath.Join(home, ".claude")
	qt.Assert(t, qt.IsNil(os.WriteFile(blocker, []byte("not a directory"), 0o644))) //nolint:gosec // test fixture

	qt.Check(t, qt.Equals(resolveMarkdownlintConfig(home), ""))
}

func TestWriteIfMissingKeepsTheExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "cfg.jsonc")
	qt.Assert(t, qt.IsNil(writeIfMissing(path, []byte("first"))))

	qt.Assert(t, qt.IsNil(writeIfMissing(path, []byte("second"))))

	got, err := os.ReadFile(path) //nolint:gosec // path is the temp dir this test just wrote
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(got), "first"), qt.Commentf("writeIfMissing must not clobber"))
}

func TestWriteIfMissingLeavesNoTempFilesBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.jsonc")

	qt.Assert(t, qt.IsNil(writeIfMissing(path, []byte("x"))))
	qt.Assert(t, qt.IsNil(writeIfMissing(path, []byte("y"))))

	entries, err := os.ReadDir(dir)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(len(entries), 1), qt.Commentf("the temp file must be cleaned up on both the write and the no-op path"))
}

func TestUserMarkdownlintConfigFileNameSelectsTheOptionsSchema(t *testing.T) {
	// markdownlint-cli2 decides how to parse a --config file from its name:
	// `.markdownlint.<ext>` means a bare rules object, `.markdownlint-cli2.<ext>`
	// means the options object (rules nested under "config"). The embedded
	// defaults are the latter, so the filename has to say so.
	//
	// A mismatch does not error -- markdownlint-cli2 silently discards the
	// file and applies stock rules, so the hook still exits 0 and still
	// formats, just against the wrong configuration. Nothing at runtime
	// would reveal it, which is why it is pinned here.
	qt.Assert(t, qt.IsTrue(strings.Contains(userMarkdownlintConfigFile, ".markdownlint-cli2.")),
		qt.Commentf("filename must carry the markdownlint-cli2 infix or the config is silently ignored"))

	var parsed map[string]any
	qt.Assert(t, qt.IsNil(json.Unmarshal(stripJSONCComments(markdownlintDefaults), &parsed)))
	_, hasConfigKey := parsed["config"]
	qt.Check(t, qt.IsTrue(hasConfigKey),
		qt.Commentf("defaults use the options shape, which is what the filename above promises"))
}

// stripJSONCComments removes whole-line // comments so encoding/json can
// parse what markdownlint-cli2 reads as JSONC. It deliberately does not
// handle trailing or block comments -- the embedded defaults only use
// whole-line ones, and a stripper that silently mangles anything more would
// make these tests pass for the wrong reason.
func stripJSONCComments(raw []byte) []byte {
	var out strings.Builder
	for line := range strings.SplitSeq(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	return []byte(out.String())
}

func TestEmbeddedMarkdownlintDefaultsAreValid(t *testing.T) {
	// markdownlint-cli2 reads this as JSONC. Strip the line comments and
	// confirm what remains parses -- a malformed embedded default would
	// make every .md write report a config error, and the failure would
	// only ever show up at runtime.
	var parsed struct {
		Config  map[string]any `json:"config"`
		Ignores []string       `json:"ignores"`
	}
	qt.Assert(t, qt.IsNil(json.Unmarshal(stripJSONCComments(markdownlintDefaults), &parsed)))
	qt.Check(t, qt.Equals(parsed.Config["default"], true), qt.Commentf("defaults must start from the full rule set"))
	qt.Check(t, qt.Equals(parsed.Config["MD013"], false), qt.Commentf("line-length is the rule that makes unconfigured projects noisy"))
	qt.Check(t, qt.IsTrue(len(parsed.Ignores) > 0))
}
