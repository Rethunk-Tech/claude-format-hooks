package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
	"github.com/Rethunk-Tech/claude-format-hooks/internal/dispatch"
	"github.com/Rethunk-Tech/claude-format-hooks/internal/formatters"
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
	rest, help := stripHelp(args)
	paths, write, asJSON, err := parseCheckArgs(rest)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "format-dispatch --check: %v\n\n%s", err, usage)
		return 2
	}
	if help {
		_, _ = fmt.Fprint(out, usage)
		return 0
	}
	if len(paths) == 0 {
		_, _ = fmt.Fprintf(errOut, "format-dispatch --check: no paths given\n\n%s", usage)
		return 2
	}
	args = paths

	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	// A check changes nothing, but it formats each file in place to find out.
	// An interrupt between that write and the restore would leave the tree
	// reformatted, so the pending originals are put back before exiting.
	stopRestoring := restoreOnSignal()
	defer stopRestoring()

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

	outcomes := runCheckJobs(ctx, jobs, registry, cfg, rootCandidates, errOut, write)

	// Tallied in job order, not completion order, so two runs over one tree
	// produce identical output.
	wouldChange, failed := tally.record(jobs, outcomes, errOut)

	if asJSON {
		if wouldChange == nil {
			wouldChange = []string{}
		}
		if err := json.NewEncoder(out).Encode(struct {
			Changed []string `json:"changed"`
		}{Changed: wouldChange}); err != nil {
			_, _ = fmt.Fprintf(errOut, "format-dispatch --check: %v\n", err)
			return 2
		}
	} else {
		verb := "would reformat"
		if write {
			verb = "reformatted"
		}
		for _, path := range wouldChange {
			_, _ = fmt.Fprintf(out, "%s: %s\n", verb, path)
		}
		_, _ = fmt.Fprintf(out, "format-dispatch --check: %s\n", tally.summary(len(wouldChange), write))
	}
	// A file the run could not check is the strongest failure: it means the
	// gate did not answer its question, so it outranks "needs formatting".
	if failed {
		return 2
	}
	// --write fixed what it found, so there is nothing left to fail over.
	// Without it the whole point is a non-zero exit CI can gate on.
	if write || len(wouldChange) == 0 {
		return 0
	}
	return 1
}

// parseCheckArgs splits --write out of the path list. It is accepted
// anywhere among the paths rather than only first, because "--check .
// --write" is what people type. Any other dash-prefix is a typo, not a
// path: --install already refuses those instead of ignoring them.
func parseCheckArgs(args []string) (paths []string, write, asJSON bool, err error) {
	for _, a := range args {
		switch a {
		case "--write":
			write = true
			continue
		case "--json":
			asJSON = true
			continue
		}
		if strings.HasPrefix(a, "-") {
			return nil, false, false, fmt.Errorf("unrecognized argument %q", a)
		}
		paths = append(paths, a)
	}
	return paths, write, asJSON, nil
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
	write bool,
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
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			outcomes[i] = runCheckJob(ctx, job, registry, cfg, rootCandidates, errOut, &errMu, write)
		})
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
	write bool,
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

	changed, skipped, err := wouldReformat(fileCtx, registryForFile, projectRoot, job.abs, job.ext, write)
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

// record folds one run's outcomes into the tally and returns the files that
// would change, in job order. failed reports any file the run could not check.
//
// Every failing file is reported rather than only the first: returning at the
// first error discarded the rest of the run's results, so the operator needed
// a second check just to see them.
func (t *checkTally) record(jobs []checkJob, outcomes []checkOutcome, errOut io.Writer) (wouldChange []string, failed bool) {
	for i, o := range outcomes {
		switch {
		case o.err != nil:
			_, _ = fmt.Fprintf(errOut, "format-dispatch --check: %s: %v\n", jobs[i].abs, o.err)
			failed = true
		case o.disabled:
			t.disabled++
		case o.skipped:
			t.noTool(o.formatter)
		default:
			t.checked++
			if o.changed {
				wouldChange = append(wouldChange, jobs[i].abs)
			}
		}
	}
	return wouldChange, failed
}

// summary reads as one line: what was checked, what needs work, and what
// was passed over and why.
func (t *checkTally) summary(changed int, write bool) string {
	state := "need formatting"
	if write {
		state = "reformatted"
	}
	head := fmt.Sprintf("%d file(s) checked, %d %s", t.checked, changed, state)

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

// wouldReformat answers the question by actually formatting the file and
// comparing bytes, rather than by asking each tool for a check/dry-run
// flag. Those flags differ per tool, several report lint findings a fixer
// would not have touched, and two of the formatters are native with no CLI
// at all -- so a uniform format-and-compare is both simpler and exactly
// consistent with what the hook does on a real write.
//
// The file formatted is the real one, and the saved copy is only the undo
// record. prettier, biome and taplo all decide which file a path is -- from
// .prettierignore, its own include rules, path-scoped overrides -- so a
// scratch name beside it is judged by different rules than the hook applies,
// in both directions: an ignored file came back "would reformat" and a
// dotfile-ignored one came back clean. Formatting the real path leaves a
// check changing nothing on its own, which is why it is restored unless
// --write asked for the change.
//
// The copy is written in the file's OWN directory, which also proves that
// directory is writable. Where it is not -- an archive mount, a 0555
// checkout -- the formatter is asked to format the same bytes on stdin under
// the real path instead, which keeps path-scoped config resolution and needs
// no write of its own.
func wouldReformat(ctx context.Context, registry *dispatch.Registry, projectRoot, abs, ext string, write bool) (changed, skipped bool, err error) {
	original, err := os.ReadFile(filepath.Clean(abs))
	if err != nil {
		return false, false, err
	}

	// The copy is the undo record and the writability probe in one: if it
	// cannot be written beside the file, nothing there can be, and the file
	// has to be judged on stdin instead.
	_, cleanup, copyErr := copyBeside(abs, original)
	if copyErr != nil {
		return wouldReformatStdin(ctx, registry, projectRoot, abs, ext, original, write)
	}
	defer cleanup()

	// A check changes nothing, so every way out of here -- a formatter
	// failure, a panic, an interrupt -- has to put the file back.
	if !write {
		pendingRestores.add(abs, original)
		defer pendingRestores.remove(abs)
		defer func() {
			if restoreErr := restoreOriginal(abs, original); restoreErr != nil && err == nil {
				changed = false
				err = fmt.Errorf("restore: %w", restoreErr)
			}
		}()
	}

	// Dispatch from the check's project root so config discovery matches the
	// hook, even when the file being checked is nested below that root.
	res := registry.Dispatch(ctx, projectRoot, abs, ext)
	if err := context.Cause(ctx); err != nil {
		return false, false, err
	}
	if res.Err != nil {
		return false, false, res.Err
	}
	if res.Diagnostic != "" {
		return false, false, fmt.Errorf("%s", res.Diagnostic)
	}
	// A formatter that skips (tool not installed) leaves the file untouched,
	// which would compare equal and read as "already formatted". It is
	// reported as a skip instead: an absent tool must not fail a build for
	// files it could never have formatted, but it must not claim to have
	// checked them either.
	if res.Skipped {
		return false, true, nil
	}

	formatted, err := os.ReadFile(filepath.Clean(abs))
	if err != nil {
		return false, false, err
	}
	// --write keeps what the formatter produced; otherwise the deferred
	// restore above is already putting the original bytes back.
	return !bytes.Equal(original, formatted), false, nil
}

// restoreSet is the files --check is formatting in place right now, by
// absolute path, with the bytes to put back. A check must never leave a
// reformatted file behind, and the process can be killed between the
// formatter's write and the caller's restore, so the undo record has to be
// visible outside the goroutine doing the work.
type restoreSet struct {
	mu    sync.Mutex
	files map[string][]byte
}

var pendingRestores = restoreSet{files: map[string][]byte{}}

func (s *restoreSet) add(abs string, original []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[abs] = original
}

func (s *restoreSet) remove(abs string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.files, abs)
}

// restoreAll puts every in-flight file back, for the signal handler.
func (s *restoreSet) restoreAll() {
	s.mu.Lock()
	pending := maps.Clone(s.files)
	s.mu.Unlock()
	for abs, original := range pending {
		_ = restoreOriginal(abs, original)
	}
}

// restoreOriginal writes original back over abs unless abs already holds it:
// rewriting an untouched file replaces its inode and mtime for nothing. The
// bytes just read are the write's source guard, so an edit landing between
// that read and the rename is kept.
func restoreOriginal(abs string, original []byte) error {
	current, err := os.ReadFile(filepath.Clean(abs))
	if err != nil {
		return formatters.WriteFormatted(abs, nil, original, 0o644)
	}
	if bytes.Equal(current, original) {
		return nil
	}
	return formatters.WriteFormatted(abs, current, original, 0o644)
}

// restoreOnSignal puts every in-flight file back when the operator interrupts
// a check, then exits non-zero: the run did not answer its question, and it
// must not leave the tree changed. The returned function stops watching.
func restoreOnSignal() (stop func()) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case <-sigs:
			pendingRestores.restoreAll()
			os.Exit(2)
		case <-done:
		}
	}()
	return func() {
		signal.Stop(sigs)
		close(done)
	}
}

// wouldReformatStdin is wouldReformat for a file whose directory cannot hold a
// scratch copy. It formats the bytes in memory through the formatter's stdin
// mode, under the real path so config lookup and path-scoped rules resolve the
// way the hook's do. A formatter with no stdin mode cannot answer the question
// at all, and says so rather than reporting the file clean.
func wouldReformatStdin(ctx context.Context, registry *dispatch.Registry, projectRoot, abs, ext string, original []byte, write bool) (changed, skipped bool, err error) {
	formatted, res, ok := registry.FormatStdin(ctx, projectRoot, abs, ext, original)
	if !ok {
		return false, false, fmt.Errorf("no writable directory beside it and %s has no stdin mode", registry.Name(ext))
	}
	if err := context.Cause(ctx); err != nil {
		return false, false, err
	}
	if res.Err != nil {
		return false, false, res.Err
	}
	if res.Diagnostic != "" {
		return false, false, fmt.Errorf("%s", res.Diagnostic)
	}
	if res.Skipped {
		return false, true, nil
	}
	if bytes.Equal(original, formatted) {
		return false, false, nil
	}
	if write {
		if err := replaceContents(abs, formatted); err != nil {
			return true, false, err
		}
	}
	return true, false, nil
}

// replaceContents overwrites abs with out through the same atomic,
// mode-preserving, symlink-following write the hook's formatters use, so
// --write cannot drift from them.
func replaceContents(abs string, out []byte) error {
	return formatters.WriteFormatted(abs, nil, out, 0o644)
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
	workingRoot, err := os.Getwd()
	if err != nil {
		workingRoot = ""
	}
	t := &targetSet{seen: map[string]bool{}, projectRoot: projectRootEnv(), workingRoot: workingRoot, skip: skip}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			t.add(filepath.Dir(p), p)
			continue
		}
		if !t.dirKept(p) {
			continue
		}
		if err := t.addDir(p); err != nil {
			return nil, err
		}
	}
	out := make([]string, 0, len(t.seen))
	for p := range t.seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

// targetSet accumulates the files collectCheckTargets keeps.
type targetSet struct {
	seen        map[string]bool
	projectRoot string
	workingRoot string
	skip        skipConfig
}

// add keeps p unless it is a leftover, outside the project, or in a skipped directory.
//
// InSkippedDir matches path SEGMENTS, so inSkipped must be given a path
// relative to the project or working directory -- handing it an absolute
// path would let an unrelated ancestor directory named "build" or
// "vendor" silently exclude the whole run.
func (t *targetSet) add(root, p string) {
	// copyBeside uses this prefix; ignoring it here keeps leftovers from
	// an interrupted check out of the next target set.
	if strings.HasPrefix(filepath.Base(p), ".fmtcheck-") {
		return
	}
	abs, err := filepath.Abs(p)
	if err != nil || !regularFile(abs) {
		return
	}
	bound := t.projectRoot
	if bound == "" {
		if root != "" {
			if absRoot, rootErr := filepath.Abs(root); rootErr == nil {
				bound = absRoot
			}
		}
		if bound == "" {
			bound = t.workingRoot
		}
	}
	if bound != "" && !within(abs, bound) {
		return
	}
	if inSkipped(abs, t.projectRoot, t.workingRoot, root, t.skip) {
		return
	}
	t.seen[abs] = true
}

// dirKept reports whether directory p is inside the project and not itself skipped.
func (t *targetSet) dirKept(p string) bool {
	absDir, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	if t.projectRoot != "" && !within(absDir, t.projectRoot) {
		return false
	}
	return !inSkipped(absDir, t.projectRoot, t.workingRoot, filepath.Dir(absDir), t.skip)
}

// addDir walks directory p, pruning skipped directories, and adds every file under it.
func (t *targetSet) addDir(p string) error {
	return filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
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
			if rel != "." && dispatch.InSkippedDir(rel, t.skip.dirs) {
				return fs.SkipDir
			}
			return nil
		}
		t.add(p, path)
		return nil
	})
}
