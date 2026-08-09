package formatters

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestNotebookFormatterPrefersRuffOverBlack(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	ruffPath := filepath.Join(dir, "ruff")
	blackPath := filepath.Join(dir, "black")
	qt.Assert(t, qt.IsNil(os.WriteFile(ruffPath, []byte("#!/bin/sh\nexit 0\n"), 0o755)))  //nolint:gosec // test fixture
	qt.Assert(t, qt.IsNil(os.WriteFile(blackPath, []byte("#!/bin/sh\nexit 1\n"), 0o755))) //nolint:gosec // test fixture
	t.Setenv("PATH", dir)

	fileDir := t.TempDir()
	res := NewNotebook().Format(t.Context(), fileDir, filepath.Join(fileDir, "f.ipynb"))

	qt.Check(t, qt.IsNil(res.Err))
	qt.Check(t, qt.Equals(res.Diagnostic, ""))
	qt.Check(t, qt.IsFalse(res.Skipped))
}

func TestNotebookFormatterFallsBackToBlack(t *testing.T) {
	isolateDiskCache(t)
	writeFakeTool(t, "black", "exit 0")
	dir := t.TempDir()

	res := NewNotebook().Format(t.Context(), dir, filepath.Join(dir, "f.ipynb"))

	qt.Check(t, qt.IsNil(res.Err))
	qt.Check(t, qt.Equals(res.Diagnostic, ""))
	qt.Check(t, qt.IsFalse(res.Skipped))
}

func TestNotebookFormatterSkipsWhenBothMissing(t *testing.T) {
	isolateDiskCache(t)
	clearPath(t)
	dir := t.TempDir()

	res := NewNotebook().Format(t.Context(), dir, filepath.Join(dir, "f.ipynb"))

	qt.Check(t, qt.IsTrue(res.Skipped))
	qt.Check(t, qt.Equals(res.Diagnostic, ""))
}

func TestNotebookFormatterSkipsWhenBlackNotebookSupportIsMissing(t *testing.T) {
	isolateDiskCache(t)
	writeFakeTool(t, "black", "echo 'No module named jupyter'; exit 1")
	dir := t.TempDir()

	res := NewNotebook().Format(t.Context(), dir, filepath.Join(dir, "f.ipynb"))

	qt.Check(t, qt.IsTrue(res.Skipped))
	qt.Check(t, qt.Equals(res.Diagnostic, ""))
}
