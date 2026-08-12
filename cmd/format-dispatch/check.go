package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
	"github.com/Rethunk-Tech/claude-format-hooks/internal/dispatch"
)

// checkTimeout bounds a whole --check run. This is CI-shaped work over many
// files, not the single-file hook path, so formatterTimeout's few seconds
// would be far too tight.
const checkTimeout = 15 * time.Minute

var checkWalkDir = filepath.WalkDir

// runCheck reports which of the given paths a formatter would change,
// without changing them. Directories are walked; unsupported extensions,
// vendored directories, and file types whose tool is not installed are
// skipped.
//
// This is the inverse of the hook contract: the hook always exits 0 because
// it runs after a tool call that already succeeded, whereas --check exists
// precisely to fail a build. It never shares that path.
//
// What it verifies is "would the formatter rewrite this file" -- the same
// question the hook answers on every write -- rather than "does this file
// satisfy every lint rule." Those differ: a formatter can legitimately
// leave violations it declines to fix, and failing CI on them would gate
// pushes on something no local write would ever repair.
func runCheck(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(errOut, "format-dispatch --check: no paths given\n\n%s", usage)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	files, err := collectCheckTargets(args)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "format-dispatch --check: %v\n", err)
		return 2
	}
	var wouldChange []string
	var registry *dispatch.Registry
	registryByProjectRoot := make(map[string]*dispatch.Registry)
	var cfg config.Config
	var cfgLoaded bool
	for _, abs := range files {
		ext := dispatch.ResolveExtension(abs)
		if ext == "" {
			if shebangExt, ok := shellShebangExt(abs); ok {
				ext = shebangExt
			}
		}
		if !dispatch.KnownExtension(ext) {
			continue
		}
		if !cfgLoaded {
			var loadErr error
			cfg, loadErr = config.Load(configPath())
			if loadErr != nil {
				_, _ = fmt.Fprintf(errOut, "format-dispatch: config: %v (using defaults)\n", loadErr)
				cfg = config.Default()
			}
			cfgLoaded = true
		}
		if cfg.IsDisabled(ext) {
			continue
		}
		if registry == nil {
			registry = dispatch.NewRegistry(cfg)
		}
		if !registry.Supported(ext) {
			continue
		}
		projectRoot := checkProjectRoot(args, abs)
		registryForFile := registry
		if disabled, projectCfg, err := projectDisables(projectRoot, ext, registry.Name(ext)); err != nil {
			_, _ = fmt.Fprintf(errOut, "format-dispatch --check: project config: %v (ignoring)\n", err)
		} else if disabled {
			continue
		} else {
			registryForFile = registryForCheck(registry, cfg, ext, projectCfg, projectRoot, registryByProjectRoot)
		}
		fileCtx, fileCancel := context.WithTimeoutCause(ctx, formatterTimeout, errFormatterTimeout)
		changed, err := wouldReformat(fileCtx, registryForFile, projectRoot, abs, ext)
		fileCancel()
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "format-dispatch --check: %s: %v\n", abs, err)
			return 2
		}
		if changed {
			wouldChange = append(wouldChange, abs)
		}
	}

	if len(wouldChange) == 0 {
		_, _ = fmt.Fprintf(out, "format-dispatch --check: %d file(s) already formatted\n", len(files))
		return 0
	}
	for _, path := range wouldChange {
		_, _ = fmt.Fprintf(out, "would reformat: %s\n", path)
	}
	_, _ = fmt.Fprintf(out, "format-dispatch --check: %d file(s) need formatting\n", len(wouldChange))
	return 1
}

// registryForCheck returns the per-file registry for --check, caching the
// project-biome-disabled router (.json, .graphql, or .gql) NewRegistry rebuild
// by projectRoot so a large tree does not rebuild once per file. The cache
// guard must use projectRebuildsJSONRegistry — the same predicate
// registryWithProjectConfig uses — so a future rebuild trigger cannot leave
// --check serving a stale map.
func registryForCheck(registry *dispatch.Registry, userCfg config.Config, ext string, projectCfg config.Config, projectRoot string, cache map[string]*dispatch.Registry) *dispatch.Registry {
	if !projectRebuildsJSONRegistry(ext, projectCfg) {
		return registryWithProjectConfig(registry, userCfg, ext, projectCfg)
	}
	if cached, ok := cache[projectRoot]; ok {
		return cached
	}
	registryForFile := registryWithProjectConfig(registry, userCfg, ext, projectCfg)
	cache[projectRoot] = registryForFile
	return registryForFile
}

// checkProjectRoot preserves the hook's project-root choice when --check is
// run without CLAUDE_PROJECT_DIR: the path argument, rather than each nested
// file discovered beneath it, defines the config and dispatch boundary.
func checkProjectRoot(paths []string, abs string) string {
	if root := os.Getenv("CLAUDE_PROJECT_DIR"); root != "" {
		if absolute, err := filepath.Abs(root); err == nil {
			return absolute
		}
		return root
	}

	best := ""
	for _, path := range paths {
		root, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		info, err := os.Stat(root)
		if err == nil && !info.IsDir() {
			root = filepath.Dir(root)
		}
		if within(abs, root) && len(root) > len(best) {
			best = root
		}
	}
	if best != "" {
		return best
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return filepath.Dir(abs)
}

// wouldReformat answers the question by actually formatting a copy and
// comparing bytes, rather than by asking each tool for a check/dry-run
// flag. Those flags differ per tool, several report lint findings a fixer
// would not have touched, and two of the formatters are native with no CLI
// at all -- so a uniform copy-and-compare is both simpler and exactly
// consistent with what the hook does on a real write.
//
// The copy is made in the file's OWN directory: every one of these tools
// resolves its config by walking up from the file (biome.json, .sqlfluff, a
// project .markdownlint-cli2.jsonc), so formatting a copy in a temp
// directory elsewhere would silently apply the wrong rules.
func wouldReformat(ctx context.Context, registry *dispatch.Registry, projectRoot, abs, ext string) (bool, error) {
	original, err := os.ReadFile(abs) //nolint:gosec // abs is a path the caller asked to check, by design
	if err != nil {
		return false, err
	}

	scratch, cleanup, err := copyBeside(abs, original)
	if err != nil {
		return false, err
	}
	defer cleanup()

	// A formatter that skips (tool not installed) or fails leaves the copy
	// untouched, which compares equal -- an absent tool must not fail a
	// build for files it could never have formatted.
	// Dispatch from the check's project root so config discovery matches the
	// hook, even when the file being checked is nested below that root.
	registry.Dispatch(ctx, projectRoot, scratch, ext)
	if err := context.Cause(ctx); err != nil {
		return false, err
	}

	formatted, err := os.ReadFile(scratch) //nolint:gosec // scratch is the temp copy this function just created
	if err != nil {
		return false, err
	}
	return !bytes.Equal(original, formatted), nil
}

// copyBeside writes content to a uniquely-named sibling of abs, preserving
// abs's extension so the formatter parses it as the same language. The name
// is dotted so it stays out of ordinary globs, and randomized so concurrent
// --check runs over one tree cannot collide.
func copyBeside(abs string, content []byte) (path string, cleanup func(), err error) {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", nil, err
	}
	dir, base := filepath.Split(abs)
	path = filepath.Join(dir, ".fmtcheck-"+hex.EncodeToString(suffix[:])+"-"+base)

	if err := os.WriteFile(path, content, 0o600); err != nil { //nolint:gosec // a sibling of the file the operator asked to check, by design
		return "", nil, err
	}
	return path, func() { _ = os.Remove(path) }, nil //nolint:gosec // removes only the temp copy created immediately above
}

// collectCheckTargets expands the given paths into a sorted, deduplicated
// file list, walking directories and applying the same vendored-directory
// exclusion the hook uses. Sorting keeps output stable so a CI diff of two
// runs is meaningful.
func collectCheckTargets(paths []string) ([]string, error) {
	seen := map[string]bool{}
	projectRoot := os.Getenv("CLAUDE_PROJECT_DIR")
	if projectRoot != "" {
		if abs, err := filepath.Abs(projectRoot); err == nil {
			projectRoot = abs
		}
	}
	vendorRoot, err := os.Getwd()
	if err != nil {
		vendorRoot = ""
	}

	// InVendoredDir matches path SEGMENTS, so it must be given a path
	// relative to the project or working directory -- handing it an absolute
	// path would let an unrelated ancestor directory named "build" or
	// "vendor" silently exclude the whole run.
	add := func(root, p string) {
		// copyBeside uses this prefix; ignoring it here keeps leftovers from
		// an interrupted check out of the next target set.
		if strings.HasPrefix(filepath.Base(p), ".fmtcheck-") {
			return
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return
		}
		if projectRoot != "" && !within(abs, projectRoot) {
			return
		}
		relRoot := vendorRoot
		if projectRoot != "" {
			relRoot = projectRoot
		} else if relRoot == "" {
			relRoot = root
		}
		rel, err := filepath.Rel(relRoot, abs)
		if err != nil {
			rel = filepath.Base(abs)
		}
		if dispatch.InVendoredDir(rel) {
			return
		}
		seen[abs] = true
	}

	for _, p := range paths {
		info, err := os.Stat(p) //nolint:gosec // p is a path the operator passed to --check; inspecting it is the entire contract
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			add(filepath.Dir(p), p)
			continue
		}
		absDir, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if projectRoot != "" && !within(absDir, projectRoot) {
			continue
		}
		relRoot := vendorRoot
		if projectRoot != "" {
			relRoot = projectRoot
		}
		rel, relErr := filepath.Rel(relRoot, absDir)
		if relErr == nil && dispatch.InVendoredDir(rel) {
			continue
		}
		err = checkWalkDir(p, func(path string, d fs.DirEntry, err error) error { //nolint:gosec // walking an operator-supplied directory is the entire contract
			if err != nil {
				return err
			}
			rel, relErr := filepath.Rel(p, path)
			if relErr != nil {
				rel = path
			}
			if d.IsDir() {
				// Prune rather than filter per-file: skipping a vendored
				// tree at its root avoids walking node_modules at all.
				if rel != "." && dispatch.InVendoredDir(rel) {
					return fs.SkipDir
				}
				return nil
			}
			add(p, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}
