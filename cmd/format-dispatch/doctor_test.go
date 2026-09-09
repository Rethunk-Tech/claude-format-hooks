package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-quicktest/qt"
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
