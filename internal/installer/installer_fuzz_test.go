package installer

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func FuzzWireUnwire(f *testing.F) {
	for _, raw := range [][]byte{
		[]byte(`{}`),
		[]byte(`{"hooks":{}}`),
		[]byte(`{"hooks":{"PostToolUse":[]}}`),
		[]byte(`{"hooks":{"PreToolUse":[]}}`),
		[]byte(`{"hooks":{"PostToolUse":[{"matcher":"Write","hooks":[{"type":"command","command":"echo"}]}]}}`),
		[]byte(`{"permissions":{"allow":["Bash"]},"hooks":{}}`),
		[]byte(`[]`),
		[]byte(`null`),
		[]byte(`true`),
		[]byte(`""`),
		[]byte(`{`),
		[]byte(``),
		[]byte(`not json`),
	} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "settings.json")
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		bin := "/format-dispatch-fuzz-bin"
		for bytes.Contains(raw, []byte(bin)) {
			bin += "x"
		}

		_, wired, err := Wire(path, bin)
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		empty := len(bytes.TrimSpace(raw)) == 0
		if !json.Valid(raw) && !empty {
			if err == nil {
				t.Fatal("invalid JSON: Wire succeeded")
			}
			if !bytes.Equal(got, raw) {
				t.Fatal("invalid JSON: Wire wrote the settings file")
			}
			return
		}
		if err != nil {
			if !bytes.Equal(got, raw) {
				t.Fatal("Wire error wrote the settings file")
			}
			return
		}
		if err := os.WriteFile(path, wired, 0o644); err != nil {
			t.Fatal(err)
		}
		_, unwired, err := Unwire(path, bin)
		if err != nil {
			t.Fatal(err)
		}
		src := raw
		if empty {
			src = []byte("{}")
		}
		want, ok1 := hookEntries(src)
		gotHooks, ok2 := hookEntries(unwired)
		if !ok1 || !ok2 {
			t.Fatal("valid JSON lost object shape after Unwire(Wire(x))")
		}
		if !reflect.DeepEqual(normalizeHooks(gotHooks), normalizeHooks(want)) {
			t.Fatalf("Unwire(Wire(x)) changed hook entries\nbefore: %#v\nafter:  %#v", want, gotHooks)
		}
	})
}

func hookEntries(data []byte) (any, bool) {
	var top map[string]any
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, false
	}
	return top["hooks"], true
}

func normalizeHooks(h any) any {
	m, ok := h.(map[string]any)
	if !ok || m == nil {
		if h == nil {
			return map[string]any{}
		}
		return h
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if a, ok := v.([]any); ok && len(a) == 0 {
			continue
		}
		out[k] = v
	}
	return out
}
