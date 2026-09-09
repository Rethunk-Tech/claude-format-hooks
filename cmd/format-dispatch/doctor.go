package main

import (
	"fmt"
	"io"
	"maps"
	"os/exec"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/dispatch"
	"github.com/Rethunk-Tech/claude-format-hooks/internal/formatters"
)

// runDoctor reports, for the resolved config, every formatter in the
// registry, the extensions it owns, and whether the external tool it needs
// is actually reachable. A missing tool is a silent skip on the hook path
// by design -- the hook must never block an edit over a formatter that
// isn't installed -- which leaves an operator no way to tell "this file
// type is unsupported" from "the tool for it is missing" from "I disabled
// it". This is that way.
//
// It exits 0 regardless of what it finds: an incomplete toolchain is a
// normal state, not a failure. --check is the flag that gates CI.
func runDoctor(out, errOut io.Writer) int {
	registry, _ := buildRegistry(errOut)

	type entry struct {
		tools []string
		exts  []string
	}
	byName := map[string]*entry{}
	for ext, f := range registry.All() {
		e := byName[f.Name()]
		if e == nil {
			e = &entry{}
			if p, ok := f.(formatters.Prober); ok {
				e.tools = p.Tools()
			}
			byName[f.Name()] = e
		}
		e.exts = append(e.exts, ext)
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "FORMATTER\tTOOL\tEXTENSIONS")
	for _, name := range slices.Sorted(maps.Keys(byName)) {
		e := byName[name]
		slices.Sort(e.exts)
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", name, toolStatus(e.tools), strings.Join(e.exts, " "))
	}
	_ = w.Flush()

	if disabled := disabledExtensions(registry); len(disabled) > 0 {
		_, _ = fmt.Fprintf(out, "\ndisabled by config: %s\n", strings.Join(disabled, " "))
	}
	return 0
}

// toolStatus resolves a formatter's candidate binaries the way it will at
// format time: the first one found wins, and none found means every file
// of that type is silently skipped. It calls exec.LookPath rather than the
// formatters package's caching wrapper on purpose -- a doctor that reports
// a 30-second-old cached miss is reporting the cache, not the system.
func toolStatus(tools []string) string {
	if len(tools) == 0 {
		return "native"
	}
	for _, t := range tools {
		if _, err := exec.LookPath(t); err == nil {
			return "ok: " + t
		}
	}
	return "MISSING: " + strings.Join(tools, ", ")
}

// disabledExtensions is the difference between what some formatter
// registers and what this operator's config left enabled.
func disabledExtensions(registry *dispatch.Registry) []string {
	var off []string
	for _, ext := range dispatch.AllKnownExtensions() {
		if !registry.Supported(ext) {
			off = append(off, ext)
		}
	}
	return off
}
