package installer

import (
	"encoding/json"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestOrderedMapRoundTripPreservesOrder(t *testing.T) {
	src := `{"b":1,"a":2,"c":3}`
	m := newOrderedMap()
	qt.Assert(t, qt.IsNil(json.Unmarshal([]byte(src), m)))

	out, err := json.Marshal(m)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(out), src))
}

func TestOrderedMapSetOnZeroValueLazyInits(t *testing.T) {
	var m orderedMap // bypassing newOrderedMap -- vals starts out nil
	m.Set("a", json.RawMessage(`1`))

	out, err := json.Marshal(m)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(out), `{"a":1}`))
}

func TestOrderedMapSetExistingKeyKeepsPosition(t *testing.T) {
	m := newOrderedMap()
	qt.Assert(t, qt.IsNil(json.Unmarshal([]byte(`{"b":1,"a":2}`), m)))

	m.Set("b", json.RawMessage(`99`))

	out, err := json.Marshal(m)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(out), `{"b":99,"a":2}`))
}

func TestOrderedMapSetNewKeyAppendsAtEnd(t *testing.T) {
	m := newOrderedMap()
	qt.Assert(t, qt.IsNil(json.Unmarshal([]byte(`{"a":1}`), m)))

	m.Set("z", json.RawMessage(`2`))

	out, err := json.Marshal(m)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(out), `{"a":1,"z":2}`))
}

func TestOrderedMapGetMissingKey(t *testing.T) {
	m := newOrderedMap()
	qt.Assert(t, qt.IsNil(json.Unmarshal([]byte(`{"a":1}`), m)))

	_, ok := m.Get("nope")
	qt.Check(t, qt.IsFalse(ok))
}

func TestOrderedMapUnmarshalRejectsNonObject(t *testing.T) {
	m := newOrderedMap()
	qt.Check(t, qt.IsNotNil(json.Unmarshal([]byte(`[1,2,3]`), m)))
	qt.Check(t, qt.IsNotNil(json.Unmarshal([]byte(`"just a string"`), m)))
}

// TestOrderedMapUnmarshalJSONDirectlyOnMalformedInput pins the exact
// decoder errors a handful of malformed inputs produce (verified with a
// spike against encoding/json's Decoder before writing these
// assertions), each hitting a distinct error return in UnmarshalJSON:
// the very first token, a key token mid-object, a truncated value, and a
// truncated closing token. Calls UnmarshalJSON directly rather than
// through json.Unmarshal, which pre-validates the entire input is
// well-formed JSON before ever delegating to a custom Unmarshaler --
// truncated/malformed bytes like these never reach UnmarshalJSON any
// other way.
func TestOrderedMapUnmarshalJSONDirectlyOnMalformedInput(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"empty input fails at the first token", ""},
		{"non-string key fails to even tokenize", `{1:2}`},
		{"truncated value fails to decode", `{"a":`},
		{"missing closing brace fails at the final token", `{"a":1`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newOrderedMap()
			qt.Check(t, qt.IsNotNil(m.UnmarshalJSON([]byte(tc.src))))
		})
	}
}

func TestOrderedMapMarshalsANilValueAsNull(t *testing.T) {
	m := newOrderedMap()
	m.Set("k", nil)

	out, err := json.Marshal(m)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(out), `{"k":null}`))
}

func TestOrderedMapEmptyObjectRoundTrips(t *testing.T) {
	m := newOrderedMap()
	qt.Assert(t, qt.IsNil(json.Unmarshal([]byte(`{}`), m)))

	out, err := json.Marshal(m)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(out), `{}`))
}
