//go:build unix

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-quicktest/qt"
)

// Opening a FIFO for reading blocks until something writes to it, so a hook
// that reads one hangs until Claude Code kills it at the hook timeout.
func TestRunReturnsOnAFIFO(t *testing.T) {
	for _, name := range []string{"pipe.json", "script"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			abs := filepath.Join(root, name)
			qt.Assert(t, qt.IsNil(syscall.Mkfifo(abs, 0o600)))
			t.Setenv("CLAUDE_PROJECT_DIR", root)

			done := make(chan struct{})
			go func() {
				run(strings.NewReader(payload(abs)))
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("run blocked opening a FIFO")
			}
		})
	}
}

func TestCheckReturnsOnAFIFOInTheTree(t *testing.T) {
	dir := t.TempDir()
	writeCheckFile(t, dir, "ok.json", formattedJSON)
	qt.Assert(t, qt.IsNil(syscall.Mkfifo(filepath.Join(dir, "pipe.json"), 0o600)))

	done := make(chan int)
	go func() {
		var out, errOut bytes.Buffer
		done <- runCheck([]string{dir}, &out, &errOut)
	}()
	select {
	case code := <-done:
		qt.Check(t, qt.Equals(code, 0))
	case <-time.After(5 * time.Second):
		t.Fatal("--check blocked opening a FIFO")
	}
}
