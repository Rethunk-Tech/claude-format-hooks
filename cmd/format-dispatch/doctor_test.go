package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-quicktest/qt"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/installer"
)

// fakePath puts an executable named name on a PATH containing only a fresh
// temp dir, so --doctor's lookups see exactly what the test declares.
func fakePath(t *testing.T, names ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		script := filepath.Join(dir, name)
		qt.Assert(t, qt.IsNil(os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755)))
	}
	t.Setenv("PATH", dir)
}

func TestDoctorReportsToolPresence(t *testing.T) {
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", filepath.Join(t.TempDir(), "absent.json"))
	fakePath(t, "rustfmt")

	var out, errOut strings.Builder
	qt.Assert(t, qt.Equals(runDoctor(&out, &errOut), 0))
	got := out.String()

	qt.Check(t, qt.StringContains(got, "FORMATTER"))
	// A formatter with no external tool must not be reported as missing.
	qt.Check(t, qt.StringContains(got, "gofmt"))
	qt.Check(t, qt.StringContains(got, "native"))
	// Present and absent tools are distinguishable, which is the point.
	qt.Check(t, qt.StringContains(got, "ok: rustfmt"))
	qt.Check(t, qt.StringContains(got, "MISSING: buf"))
	// Every extension a formatter owns is listed against it.
	qt.Check(t, qt.StringContains(got, ".rs"))
	qt.Check(t, qt.StringContains(got, ".lua"))
}

// A config-disabled extension vanishes from the registry entirely, which
// looks identical to "unsupported" unless --doctor says otherwise --
// exactly the confusion it exists to resolve.
func TestDoctorReportsDisabledExtensions(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.json")
	qt.Assert(t, qt.IsNil(os.WriteFile(cfg, []byte(`{"disabled":[".rs"]}`), 0o600)))
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", cfg)
	fakePath(t)

	var out, errOut strings.Builder
	qt.Assert(t, qt.Equals(runDoctor(&out, &errOut), 0))
	got := out.String()

	qt.Check(t, qt.StringContains(got, "disabled by config: .rs"))
	qt.Check(t, qt.Not(qt.StringContains(got, "rustfmt")))
}

// A complete toolchain is useless if the binary was never wired into the
// harness, and from the outside the two failures look identical: files
// simply stay unformatted. --doctor has to separate them.
func TestDoctorReportsHookWiring(t *testing.T) {
	binDir := t.TempDir()
	settings := filepath.Join(t.TempDir(), "settings.json")
	cursor := filepath.Join(t.TempDir(), "hooks.json")
	t.Setenv("CLAUDE_HOOKS_BIN_DIR", binDir)
	t.Setenv("CLAUDE_SETTINGS_FILE", settings)
	t.Setenv("CURSOR_HOOKS_FILE", cursor)
	t.Setenv("CLAUDE_FORMAT_HOOKS_CONFIG", filepath.Join(t.TempDir(), "absent.json"))
	fakePath(t)

	t.Run("absent files are not wired", func(t *testing.T) {
		var out, errOut strings.Builder
		qt.Assert(t, qt.Equals(runDoctor(&out, &errOut), 0))
		qt.Check(t, qt.StringContains(out.String(), "hook wiring"))
		qt.Check(t, qt.StringContains(out.String(), "NOT WIRED"))
	})

	t.Run("wired after install", func(t *testing.T) {
		opts, err := installer.DefaultOptions()
		qt.Assert(t, qt.IsNil(err))
		qt.Assert(t, qt.IsNil(os.MkdirAll(filepath.Dir(opts.BinPath), 0o750)))
		qt.Assert(t, qt.IsNil(os.WriteFile(opts.BinPath, []byte("#!/bin/sh\n"), 0o755)))
		qt.Assert(t, qt.IsNil(installer.Install(opts, false, io.Discard)))

		var out, errOut strings.Builder
		qt.Assert(t, qt.Equals(runDoctor(&out, &errOut), 0))
		got := out.String()
		// Column padding is tabwriter's business, so match the line, not
		// the spacing between path and state.
		qt.Check(t, qt.IsTrue(wiringLineSays(got, settings, "wired")))
		qt.Check(t, qt.IsTrue(wiringLineSays(got, cursor, "wired")))
		qt.Check(t, qt.Not(qt.StringContains(got, "NOT WIRED")))
	})

	t.Run("malformed file is unreadable, not unwired", func(t *testing.T) {
		qt.Assert(t, qt.IsNil(os.WriteFile(settings, []byte("{not json"), 0o600)))
		var out, errOut strings.Builder
		qt.Assert(t, qt.Equals(runDoctor(&out, &errOut), 0))
		qt.Check(t, qt.StringContains(out.String(), "UNREADABLE"),
			qt.Commentf("--install would fail on this, not fix it"))
	})
}

// wiringLineSays reports whether the --doctor line naming path ends in
// state, independent of how tabwriter padded the columns.
func wiringLineSays(output, path, state string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, path) {
			return strings.HasSuffix(strings.TrimSpace(line), state)
		}
	}
	return false
}
