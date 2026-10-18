package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestInstallShRejectsDryRunExtraWithoutBuilding(t *testing.T) {
	stdout, stderr, code := runInstallSh(t, installShEnv(t, t.TempDir()), "--dry-run", "extra")
	qt.Check(t, qt.Equals(code, 2))
	qt.Check(t, qt.StringContains(stderr, "usage:"))
	qt.Check(t, qt.Equals(stdout, ""))
	qt.Check(t, qt.Not(qt.StringContains(stderr+stdout, "Building")))
}

func TestInstallShUninstallMissingBinaryIsSuccess(t *testing.T) {
	binDir := t.TempDir()
	stdout, stderr, code := runInstallSh(t, installShEnv(t, binDir), "--uninstall")
	qt.Check(t, qt.Equals(code, 0))
	qt.Check(t, qt.Equals(stderr, ""))
	qt.Check(t, qt.StringContains(stdout, "Nothing to unwire"))
	qt.Check(t, qt.Not(qt.StringContains(stdout, "Building")))
}

func TestInstallShUninstallRejectsExtraArg(t *testing.T) {
	_, stderr, code := runInstallSh(t, installShEnv(t, t.TempDir()), "--uninstall", "--bogus")
	qt.Check(t, qt.Equals(code, 2))
	qt.Check(t, qt.StringContains(stderr, "accepts only an optional --dry-run"))
}

func TestInstallShUninstallDelegatesToExistingBinary(t *testing.T) {
	binDir := t.TempDir()
	record := filepath.Join(t.TempDir(), "argv")
	writeFakeHookBinary(t, filepath.Join(binDir, "format-dispatch"), record)

	stdout, stderr, code := runInstallSh(t, installShEnv(t, binDir), "--uninstall")
	qt.Check(t, qt.Equals(code, 0))
	qt.Check(t, qt.Equals(stderr, ""))
	qt.Check(t, qt.Not(qt.StringContains(stdout, "Building")))
	qt.Check(t, qt.Equals(strings.TrimSpace(readFile(t, record)), "--uninstall"))
}

func TestInstallShUninstallDryRunDelegatesWithoutWritingBuild(t *testing.T) {
	binDir := t.TempDir()
	record := filepath.Join(t.TempDir(), "argv")
	writeFakeHookBinary(t, filepath.Join(binDir, "format-dispatch"), record)

	_, stderr, code := runInstallSh(t, installShEnv(t, binDir), "--uninstall", "--dry-run")
	qt.Check(t, qt.Equals(code, 0))
	qt.Check(t, qt.Equals(stderr, ""))
	qt.Check(t, qt.Equals(strings.TrimSpace(readFile(t, record)), "--uninstall --dry-run"))
}

func runInstallSh(t *testing.T, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is the bash entry")
	}
	cmd := exec.CommandContext(t.Context(), "bash", append([]string{installShPath(t)}, args...)...) //nolint:gosec // test invokes the repo install.sh with fixed args
	cmd.Env = env
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err == nil {
		return outBuf.String(), errBuf.String(), 0
	}
	var exit *exec.ExitError
	qt.Assert(t, qt.IsTrue(errors.As(err, &exit)), qt.Commentf("%v", err))
	return outBuf.String(), errBuf.String(), exit.ExitCode()
}

func installShPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	qt.Assert(t, qt.IsTrue(ok))
	return filepath.Join(filepath.Dir(file), "..", "..", "install.sh")
}

func installShEnv(t *testing.T, binDir string) []string {
	t.Helper()
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + t.TempDir(),
		"CLAUDE_HOOKS_BIN_DIR=" + binDir,
		"CLAUDE_SETTINGS_FILE=" + filepath.Join(t.TempDir(), "settings.json"),
		"CURSOR_HOOKS_FILE=" + filepath.Join(t.TempDir(), "hooks.json"),
	}
}

func writeFakeHookBinary(t *testing.T, path, record string) {
	t.Helper()
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > '" + record + "'\n"
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(script), 0o755))) //nolint:gosec // test hook fixture must be executable
}
