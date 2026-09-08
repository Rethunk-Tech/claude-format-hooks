package installer

import (
	"context"
	"fmt"
	"io"
	"os/exec"
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

// provisionTimeout bounds the whole provisioning step. Installing four
// packages over a cold network is slow but bounded; a hung registry must
// not wedge an install that has already done its real work.
const provisionTimeout = 5 * time.Minute

// ProvisionTools installs the bunx-dispatched formatters globally so no
// format-time fetch is ever needed.
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

	return nil
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
