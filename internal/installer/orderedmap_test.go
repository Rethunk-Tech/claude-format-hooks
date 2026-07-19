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

func TestOrderedMapEmptyObjectRoundTrips(t *testing.T) {
	m := newOrderedMap()
	qt.Assert(t, qt.IsNil(json.Unmarshal([]byte(`{}`), m)))

	out, err := json.Marshal(m)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(out), `{}`))
}
