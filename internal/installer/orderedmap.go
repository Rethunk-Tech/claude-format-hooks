package installer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

// orderedMap is a JSON object that preserves the source's key order across
// an Unmarshal/mutate/Marshal round-trip. encoding/json's map[string]T
// re-marshals with keys sorted alphabetically, which would silently
// reorder every key in a hand-curated file like settings.json — the exact
// failure mode json.Indent (internal/formatters/json.go) was chosen over
// Unmarshal+Marshal to avoid for the files this tool formats. A key set
// for the first time is appended at the end; an existing key is updated
// in place, keeping its original position.
type orderedMap struct {
	keys []string
	vals map[string]json.RawMessage
}

func newOrderedMap() *orderedMap {
	return &orderedMap{vals: map[string]json.RawMessage{}}
}

func (m *orderedMap) Get(key string) (json.RawMessage, bool) {
	v, ok := m.vals[key]
	return v, ok
}

func (m *orderedMap) Set(key string, val json.RawMessage) {
	if m.vals == nil {
		m.vals = map[string]json.RawMessage{}
	}
	if _, ok := m.vals[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.vals[key] = val
}

func (m *orderedMap) Delete(key string) {
	if _, ok := m.vals[key]; !ok {
		return
	}
	delete(m.vals, key)
	m.keys = slices.DeleteFunc(m.keys, func(existing string) bool {
		return existing == key
	})
}

// UnmarshalJSON records each top-level key the first time it's seen, then
// decodes its value as a raw, untouched JSON blob — so any key this
// package never inspects survives byte-for-byte.
func (m *orderedMap) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("expected a JSON object, got %v", tok)
	}

	keys := make([]string, 0)
	vals := make(map[string]json.RawMessage)
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("expected a string object key, got %v", keyTok)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return fmt.Errorf("decode value for key %q: %w", key, err)
		}
		if _, exists := vals[key]; !exists {
			keys = append(keys, key)
		}
		vals[key] = raw
	}
	if _, err := dec.Token(); err != nil { // closing '}'
		return err
	}

	m.keys, m.vals = keys, vals
	return nil
}

func (m orderedMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyJSON, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buf.Write(keyJSON)
		buf.WriteByte(':')
		if val := m.vals[key]; val != nil {
			buf.Write(val)
		} else {
			buf.WriteString("null")
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
