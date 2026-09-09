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
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
	"github.com/Rethunk-Tech/claude-format-hooks/internal/dispatch"
)

// checkTimeout bounds a whole --check run. This is CI-shaped work over many
// files, not the single-file hook path, so formatterTimeout's few seconds
// would be far too tight.
const checkTimeout = 15 * time.Minute

// runCheck reports which of the given paths a formatter would change,
// without changing them. Directories are walked; unsupported extensions,
// skipped directories, and file types whose tool is not installed are
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

	rootCandidates := checkRootCandidates(args)
	files, err := collectCheckTargets(args, checkSkipConfig(rootCandidates))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "format-dispatch --check: %v\n", err)
		return 2
	}
	var tally checkTally
	// Built on the first supported file, not up front: a run over files no
	// formatter handles must stay silent, even about a malformed config.
	var registry *dispatch.Registry
	var cfg config.Config
	var jobs []checkJob
	for _, abs := range files {
		ext := resolveDispatchExt(abs)
		if !dispatch.KnownExtension(ext) {
			tally.unsupported++
			continue
		}
		if registry == nil {
			registry, cfg = buildRegistry(errOut)
		}
		if cfg.IsDisabled(ext) || !registry.Supported(ext) {
			tally.disabled++
			continue
		}
		jobs = append(jobs, checkJob{abs: abs, ext: ext})
	}

	outcomes := runCheckJobs(ctx, jobs, registry, cfg, rootCandidates, errOut)

	// Tallied in job order, not completion order, so two runs over one tree
	// produce identical output.
	var wouldChange []string
	for i, o := range outcomes {
		switch {
		case o.err != nil:
			_, _ = fmt.Fprintf(errOut, "format-dispatch --check: %s: %v\n", jobs[i].abs, o.err)
			return 2
		case o.disabled:
			tally.disabled++
		case o.skipped:
			tally.noTool(o.formatter)
		default:
			tally.checked++
			if o.changed {
				wouldChange = append(wouldChange, jobs[i].abs)
			}
		}
	}

	for _, path := range wouldChange {
		_, _ = fmt.Fprintf(out, "would reformat: %s\n", path)
	}
	_, _ = fmt.Fprintf(out, "format-dispatch --check: %s\n", tally.summary(len(wouldChange)))
	if len(wouldChange) == 0 {
		return 0
	}
	return 1
}

// checkJob is one file that survived classification: a known, enabled
// extension whose formatter still has to be run to answer the question.
type checkJob struct {
	abs string
	ext string
}

// checkOutcome is what running one job produced. Every field is filled by
// exactly one goroutine and read only after the pool drains.
type checkOutcome struct {
	changed   bool
	skipped   bool
	disabled  bool
	formatter string
	err       error
}

// runCheckJobs formats and compares every job, NumCPU at a time. The work
// is one subprocess per file with the CPU otherwise idle -- measured at
// 64ms per file serially, around 0.8 of the machine's cores -- so the pool
// is what keeps a large tree inside checkTimeout.
//
// Results are written to a preallocated slice by index rather than
// collected from a channel: the caller tallies in job order, so output does
// not depend on which worker finished first.
func runCheckJobs(
	ctx context.Context,
	jobs []checkJob,
	registry *dispatch.Registry,
	cfg config.Config,
	rootCandidates []string,
	errOut io.Writer,
) []checkOutcome {
	outcomes := make([]checkOutcome, len(jobs))
	if len(jobs) == 0 {
		return outcomes
	}

	workers := min(runtime.NumCPU(), len(jobs))
	// errOut is shared; a malformed project config found by two workers at
	// once would otherwise interleave mid-line.
	var errMu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)

	for i, job := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			outcomes[i] = runCheckJob(ctx, job, registry, cfg, rootCandidates, errOut, &errMu)
		}()
	}
	wg.Wait()
	return outcomes
}

// runCheckJob is the per-file body: resolve this file's project root and
// its project-level opt-outs, then format a copy and compare.
func runCheckJob(
	ctx context.Context,
	job checkJob,
	registry *dispatch.Registry,
	cfg config.Config,
	rootCandidates []string,
	errOut io.Writer,
	errMu *sync.Mutex,
) checkOutcome {
	projectRoot := checkProjectRoot(rootCandidates, job.abs)
	registryForFile := registry
	if disabled, projectCfg, err := projectDisables(projectRoot, job.ext, registry.Name(job.ext)); err != nil {
		errMu.Lock()
		_, _ = fmt.Fprintf(errOut, "format-dispatch --check: project config: %v (ignoring)\n", err)
		errMu.Unlock()
	} else if disabled {
		return checkOutcome{disabled: true}
	} else {
		registryForFile = registryWithProjectConfig(registry, cfg, job.ext, projectCfg)
	}

	fileCtx, cancel := context.WithTimeoutCause(ctx, formatterTimeout, errFormatterTimeout)
	defer cancel()

	changed, skipped, err := wouldReformat(fileCtx, registryForFile, projectRoot, job.abs, job.ext)
	return checkOutcome{
		changed:   changed,
		skipped:   skipped,
		formatter: registryForFile.Name(job.ext),
		err:       err,
	}
}

// checkTally counts what --check actually did, which is not the same as the
// number of files it walked. Reporting the walked count as "already
// formatted" claims files passed a check that never ran on them: an
// unsupported type, an opted-out one, and one whose formatter isn't
// installed all reach the end untouched and byte-identical. On a CI runner
// missing half the toolchain that turns a green formatting gate into a
// statement about nothing.
type checkTally struct {
	checked     int
	unsupported int
	disabled    int
	skippedTool int
	// tools are the distinct formatter names that declined for want of a
	// binary, so the summary can say which ones to install.
	tools []string
}

func (t *checkTally) noTool(formatter string) {
	t.skippedTool++
	if !slices.Contains(t.tools, formatter) {
		t.tools = append(t.tools, formatter)
	}
}

// summary reads as one line: what was checked, what needs work, and what
// was passed over and why.
func (t *checkTally) summary(needFormatting int) string {
	head := fmt.Sprintf("%d file(s) checked, %d need formatting", t.checked, needFormatting)

	var skipped []string
	if t.unsupported > 0 {
		skipped = append(skipped, fmt.Sprintf("%d unsupported", t.unsupported))
	}
	if t.disabled > 0 {
		skipped = append(skipped, fmt.Sprintf("%d disabled", t.disabled))
	}
	if t.skippedTool > 0 {
		sort.Strings(t.tools)
		skipped = append(skipped, fmt.Sprintf("%d no tool (%s)", t.skippedTool, strings.Join(t.tools, ", ")))
	}
	if len(skipped) == 0 {
		return head
	}
	return head + "; skipped: " + strings.Join(skipped, ", ")
}

// checkProjectRoot preserves the hook's project-root choice when --check is
// run without CLAUDE_PROJECT_DIR: the path argument, rather than each nested
// file discovered beneath it, defines the config and dispatch boundary.
func checkProjectRoot(candidates []string, abs string) string {
	if root := projectRootEnv(); root != "" {
		return root
	}

	best := ""
	for _, root := range candidates {
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

// checkRootCandidates resolves each path argument to the directory that
// would define a config boundary. Loop-invariant across the run, so it is
// computed once rather than re-Stat-ing every argument for every file.
func checkRootCandidates(paths []string) []string {
	candidates := make([]string, 0, len(paths))
	for _, path := range paths {
		root, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		if info, err := os.Stat(root); err == nil && !info.IsDir() {
			root = filepath.Dir(root)
		}
		candidates = append(candidates, root)
	}
	return candidates
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
func wouldReformat(ctx context.Context, registry *dispatch.Registry, projectRoot, abs, ext string) (changed, skipped bool, err error) {
	original, err := os.ReadFile(abs) //nolint:gosec // abs is a path the caller asked to check, by design
	if err != nil {
		return false, false, err
	}

	scratch, cleanup, err := copyBeside(abs, original)
	if err != nil {
		return false, false, err
	}
	defer cleanup()

	// A formatter that skips (tool not installed) leaves the copy untouched,
	// which would compare equal and read as "already formatted". It is
	// reported as a skip instead: an absent tool must not fail a build for
	// files it could never have formatted, but it must not claim to have
	// checked them either.
	// Dispatch from the check's project root so config discovery matches the
	// hook, even when the file being checked is nested below that root.
	res := registry.Dispatch(ctx, projectRoot, scratch, ext)
	if err := context.Cause(ctx); err != nil {
		return false, false, err
	}
	if res.Skipped {
		return false, true, nil
	}

	formatted, err := os.ReadFile(scratch) //nolint:gosec // scratch is the temp copy this function just created
	if err != nil {
		return false, false, err
	}
	return !bytes.Equal(original, formatted), false, nil
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
	return path, func() { _ = os.Remove(path) }, nil
}

// inSkipped reports whether abs sits under a skipped directory.
// dispatch.InSkippedDir matches path SEGMENTS, so abs must first be made
// relative to a root -- handing it an absolute path would let an unrelated
// ancestor named "build" or "vendor" silently exclude the whole run. The
// project root wins, then the working directory, then fallbackRoot for when
// os.Getwd failed. A path that cannot be made relative at all (a different
// Windows volume, say) is judged on its own base name.
func inSkipped(abs, projectRoot, workingRoot, fallbackRoot string, skip skipConfig) bool {
	relRoot := workingRoot
	if projectRoot != "" {
		relRoot = projectRoot
	} else if relRoot == "" {
		relRoot = fallbackRoot
	}
	rel, err := filepath.Rel(relRoot, abs)
	if err != nil {
		rel = filepath.Base(abs)
	}
	return dispatch.InSkippedDir(rel, skip.dirs) ||
		dispatch.InSkippedFile(filepath.Base(abs), skip.files)
}

// checkSkipDirs is the extra skip list --check honors while collecting
// targets. Collection runs before the lazy registry build, so this reads
// the user and per-root project configs directly and ignores a malformed
// one: a run over a tree no formatter handles must stay silent, even about
// a broken config.
func checkSkipConfig(rootCandidates []string) skipConfig {
	var skip skipConfig
	add := func(cfg config.Config) {
		skip.dirs = append(skip.dirs, cfg.SkipDirs...)
		skip.files = append(skip.files, cfg.SkipFiles...)
	}
	if cfg, err := config.Load(configPath()); err == nil {
		add(cfg)
	}
	for _, root := range rootCandidates {
		if cfg, err := config.Load(filepath.Join(root, projectConfigFile)); err == nil {
			add(cfg)
		}
	}
	return skip
}

// skipConfig is the config-supplied half of the skip rules, carried
// together because every site that consults one consults the other.
type skipConfig struct {
	dirs  []string
	files []string
}

// collectCheckTargets expands the given paths into a sorted, deduplicated
// file list, walking directories and applying the same skipped-directory
// exclusion the hook uses. Sorting keeps output stable so a CI diff of two
// runs is meaningful.
func collectCheckTargets(paths []string, skip skipConfig) ([]string, error) {
	seen := map[string]bool{}
	projectRoot := projectRootEnv()
	workingRoot, err := os.Getwd()
	if err != nil {
		workingRoot = ""
	}

	// InSkippedDir matches path SEGMENTS, so it must be given a path
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
		if inSkipped(abs, projectRoot, workingRoot, root, skip) {
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
		if inSkipped(absDir, projectRoot, workingRoot, filepath.Dir(absDir), skip) {
			continue
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error { //nolint:gosec // walking an operator-supplied directory is the entire contract
			if err != nil {
				return err
			}
			rel, relErr := filepath.Rel(p, path)
			if relErr != nil {
				rel = path
			}
			if d.IsDir() {
				// Prune rather than filter per-file: skipping a non-source
				// tree at its root avoids walking node_modules at all.
				if rel != "." && dispatch.InSkippedDir(rel, skip.dirs) {
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
