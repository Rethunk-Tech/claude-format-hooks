package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // test fixture
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func payload(filePath string) string {
	return `{"tool_input":{"file_path":"` + filePath + `"}}`
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestRunReadStdinError(t *testing.T) {
	got := run(errReader{})
	if got != 1 {
		t.Errorf("run() = %d, want 1 on a stdin read failure", got)
	}
}

func TestRunEmptyPayloadIsNoop(t *testing.T) {
	if got := run(strings.NewReader(`{}`)); got != 0 {
		t.Errorf("run() = %d, want 0", got)
	}
}

func TestRunUnsupportedExtensionIsNoop(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "f.xyz")
	writeFile(t, abs, "irrelevant")

	if got := run(strings.NewReader(payload(abs))); got != 0 {
		t.Errorf("run() = %d, want 0", got)
	}
	if got := readFile(t, abs); got != "irrelevant" {
		t.Errorf("file was modified for an unsupported extension: %q", got)
	}
}

func TestRunOutsideProjectRootIsSkipped(t *testing.T) {
	projectRoot := t.TempDir()
	outside := t.TempDir()
	abs := filepath.Join(outside, "f.json")
	writeFile(t, abs, `{"b":1,"a":2}`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	if got := run(strings.NewReader(payload(abs))); got != 0 {
		t.Errorf("run() = %d, want 0", got)
	}
	if got := readFile(t, abs); got != `{"b":1,"a":2}` {
		t.Errorf("file outside CLAUDE_PROJECT_DIR was formatted: %q", got)
	}
}

func TestRunVendoredDirIsSkipped(t *testing.T) {
	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "node_modules", "pkg", "f.json")
	writeFile(t, abs, `{"b":1,"a":2}`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	if got := run(strings.NewReader(payload(abs))); got != 0 {
		t.Errorf("run() = %d, want 0", got)
	}
	if got := readFile(t, abs); got != `{"b":1,"a":2}` {
		t.Errorf("file under node_modules was formatted: %q", got)
	}
}

func TestRunDispatchesToJSONFormatter(t *testing.T) {
	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "f.json")
	writeFile(t, abs, `{"b":1,"a":2}`)

	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)
	if got := run(strings.NewReader(payload(abs))); got != 0 {
		t.Errorf("run() = %d, want 0", got)
	}
	want := "{\n  \"b\": 1,\n  \"a\": 2\n}\n"
	if got := readFile(t, abs); got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestRunPrintsDiagnosticOnFormatterFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script tool is POSIX-shell only")
	}
	toolDir := t.TempDir()
	script := filepath.Join(toolDir, "bunx")
	body := "#!/bin/sh\ni=1\nwhile [ $i -le 20 ]; do echo \"line $i\"; i=$((i+1)); done\nexit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	t.Setenv("PATH", toolDir)

	projectRoot := t.TempDir()
	abs := filepath.Join(projectRoot, "f.md")
	writeFile(t, abs, "# heading")
	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)

	var got int
	stderr := captureStderr(t, func() {
		got = run(strings.NewReader(payload(abs)))
	})
	if got != 0 {
		t.Errorf("run() = %d, want 0 even on formatter failure", got)
	}
	if !strings.Contains(stderr, "fixer failed") || !strings.Contains(stderr, "line 1") {
		t.Errorf("stderr = %q, want it to contain the truncated diagnostic", stderr)
	}
	if strings.Contains(stderr, "line 11") {
		t.Errorf("stderr = %q, want output truncated to 10 lines", stderr)
	}
}
