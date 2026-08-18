package installer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// bunTools are the formatters dispatched through bunx. They are installed
// globally at install time rather than fetched on first use: the hook's
// per-file budget is a few seconds, and a cold npm-registry fetch does not
// fit inside it. Warming bunx's cache alone is not enough — a cached package
// still costs a resolution step, and the binaries must be on PATH where bunx
// can reach them immediately.
var bunTools = []string{
	"@biomejs/biome",
	"prettier",
	"@taplo/cli",
	"markdownlint-cli2",
}

// bunGlobalOverrides lists optional transitive dependency overrides for the
// tools above. Bun honors an `overrides` key in the global manifest exactly as
// it does in a project's, but only on `bun install` -- `bun add -g` rewrites
// the file and drops it, which is why provisioning adds the packages first
// and applies overrides second.
var bunGlobalOverrides = map[string]string{}

// provisionTimeout bounds the whole provisioning step. Installing four
// packages over a cold network is slow but bounded; a hung registry must
// not wedge an install that has already done its real work.
const provisionTimeout = 5 * time.Minute

// ProvisionTools installs the bunx-dispatched formatters globally so no
// format-time fetch is ever needed, then applies bunGlobalOverrides.
//
// Best-effort by design: it reports what happened to out and returns nil
// even when a step fails. The settings.json wiring is the part of --install
// that must succeed, and a network hiccup here only means the affected tool
// falls back to bunx's own on-demand fetch, exactly as before. Absent bun,
// it is a silent no-op.
func ProvisionTools(out io.Writer) error {
	if _, err := exec.LookPath("bun"); err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), provisionTimeout)
	defer cancel()

	_, _ = fmt.Fprintf(out, "==> Installing formatters globally: %v\n", bunTools)
	args := append([]string{"add", "-g"}, bunTools...)
	if output, err := exec.CommandContext(ctx, "bun", args...).CombinedOutput(); err != nil { //nolint:gosec // args is bunTools, a package-level constant list; never caller input
		_, _ = fmt.Fprintf(out, "    (skipped: %v -- formatters will be fetched on first use)\n", err)
		if len(output) > 0 {
			_, _ = fmt.Fprintf(out, "    %s\n", firstLine(output))
		}
		return nil
	}

	if err := applyBunGlobalOverrides(ctx, out); err != nil {
		_, _ = fmt.Fprintf(out, "    (could not pin transitive dependencies: %v)\n", err)
	}
	return nil
}

// applyBunGlobalOverrides merges bunGlobalOverrides into bun's global
// manifest and reinstalls so they take effect. It merges rather than
// replaces: the manifest is shared with whatever else the operator has
// installed globally, and clobbering their dependencies to pin ours would
// be a far worse bug than the advisory being pinned.
func applyBunGlobalOverrides(ctx context.Context, out io.Writer) error {
	if len(bunGlobalOverrides) == 0 {
		return nil
	}

	dir, err := bunGlobalDir()
	if err != nil {
		return err
	}
	manifest := filepath.Join(dir, "package.json")

	raw, err := os.ReadFile(manifest) //nolint:gosec // bun's own global manifest, at a path bun defines
	if err != nil {
		return err
	}

	// map[string]any round-trips the manifest without a schema for keys we
	// do not know about; bun writes this file itself and does not depend on
	// key order the way a hand-authored config would.
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}

	overrides, _ := doc["overrides"].(map[string]any)
	if overrides == nil {
		overrides = map[string]any{}
	}
	changed := false
	for name, version := range bunGlobalOverrides {
		if existing, ok := overrides[name].(string); ok && existing == version {
			continue
		}
		overrides[name] = version
		changed = true
	}
	if !changed {
		return nil
	}
	doc["overrides"] = overrides

	updated, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifest, append(updated, '\n'), 0o600); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(out, "==> Pinning transitive dependencies: %v\n", bunGlobalOverrides)
	cmd := exec.CommandContext(ctx, "bun", "install")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		if len(output) > 0 {
			return fmt.Errorf("%w: %s", err, firstLine(output))
		}
		return err
	}
	return nil
}

// bunGlobalDir returns bun's global install directory. BUN_INSTALL is bun's
// own override for it, so honoring it keeps a non-default layout working.
func bunGlobalDir() (string, error) {
	if root := os.Getenv("BUN_INSTALL"); root != "" {
		return filepath.Join(root, "install", "global"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".bun", "install", "global"), nil
}

// firstLine keeps a failing command's output to one line: this runs during
// an install whose important output is the settings.json diff, and a full
// npm error dump would bury it.
func firstLine(output []byte) []byte {
	for i, b := range output {
		if b == '\n' {
			return output[:i]
		}
	}
	return output
}
