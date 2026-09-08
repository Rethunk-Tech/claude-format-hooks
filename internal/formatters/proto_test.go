package formatters

import (
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestProtoFormatterSkipsWhenBufMissing(t *testing.T) {
	isolateDiskCache(t)
	clearPath(t)
	abs := filepath.Join(t.TempDir(), "f.proto")
	res := NewProto().Format(t.Context(), t.TempDir(), abs)
	qt.Check(t, qt.IsTrue(res.Skipped))
}

func TestProtoFormatterReportsFailure(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.proto")

	writeFakeTool(t, "buf", "echo 'syntax error'; exit 1")
	res := NewProto().Format(t.Context(), dir, abs)
	assertDiagnostic(t, res)
}

func TestProtoFormatterSucceedsSilently(t *testing.T) {
	isolateDiskCache(t)
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.proto")

	writeFakeTool(t, "buf", "exit 0")
	res := NewProto().Format(t.Context(), dir, abs)
	qt.Check(t, qt.IsNil(res.Err))
	qt.Check(t, qt.Equals(res.Diagnostic, ""))
}
