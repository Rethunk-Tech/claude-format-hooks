package formatters

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

func TestFormatEnvDashSZshShebang(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "envS.sh")
	src := []byte("#!/usr/bin/env -S zsh -u\n() { echo hi }\n")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}
	res := NewShell(config.Config{}).Format(t.Context(), dir, path)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if res.Skipped {
		t.Fatal(`formatter declined #!/usr/bin/env -S zsh -u`)
	}
	out, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte("() { echo hi; }")) {
		t.Fatalf("formatted = %q, want zsh-formatted anonymous function", out)
	}
}

func TestFormatUppercaseZshSuffix(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "FILE.ZSH")
	src := []byte("() { echo hi }\n")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}
	res := NewShell(config.Config{}).Format(t.Context(), dir, path)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if res.Skipped {
		t.Fatal(`formatter declined FILE.ZSH`)
	}
	out, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte("() { echo hi; }")) {
		t.Fatalf("formatted = %q, want zsh-formatted anonymous function", out)
	}
}
