package convert_test

import (
	"fmt"
	"testing"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
	"go.starlark.net/starlark"
)

// Regression tests for deterministic key order: Go's map iteration order is
// randomized, and it used to leak into Starlark through every place a
// wrapped Go map was materialized (keys/items/values/iteration/popitem and
// MakeDict), so the same script over the same data produced different
// orders on every run. Keys must come out in a deterministic order: sorted
// by type rank (bool < int < uint < float < string < other), then by value
// within the same rank.

// TestGoMapDeterministicKeyOrder verifies that scripts observe wrapped Go
// map keys in sorted order through keys/items/values and iteration.
func TestGoMapDeterministicKeyOrder(t *testing.T) {
	m := map[string]int{}
	for i := 0; i < 50; i++ {
		m[fmt.Sprintf("key%02d", i)] = i
	}
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"m":      m,
	}
	code := []byte(`
ks = m.keys()
assert.Eq(sorted(ks), ks)
assert.Eq(len(ks), 50)
assert.Eq([k for k in m], ks)
assert.Eq([i[0] for i in m.items()], ks)
assert.Eq([m[k] for k in ks], [i[1] for i in m.items()])
assert.Eq(m.values(), [m[k] for k in ks])
`)
	if _, err := starlight.Eval(code, globals, nil); err != nil {
		t.Fatal(err)
	}
}

// TestGoMapMixedKeyTypeOrder verifies the documented type-rank order for
// interface-keyed maps holding keys of mixed Go types.
func TestGoMapMixedKeyTypeOrder(t *testing.T) {
	m := map[interface{}]interface{}{
		"b":          1,
		"a":          2,
		int64(7):     3,
		int32(5):     4,
		uint16(9):    5,
		false:        6,
		true:         7,
		float64(2.5): 8,
	}
	g := convert.NewGoMap(m)
	var got []string
	for _, k := range g.Keys() {
		got = append(got, k.String())
	}
	// bool < int < uint < float < string; within a rank, by value
	// (interface-typed keys come out wrapped, hence the plain formatting)
	want := []string{"false", "true", "5", "7", "9", "2.5", "a", "b"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected key order %v, got %v", want, got)
		}
	}
}

// TestMakeDictDeterministicOrder verifies MakeDict materializes Go map
// entries into the Starlark dict in sorted key order (Starlark dicts
// preserve insertion order, so this is script-visible).
func TestMakeDictDeterministicOrder(t *testing.T) {
	m := map[string]int{}
	var want []starlark.Value
	for i := 0; i < 26; i++ {
		k := fmt.Sprintf("k%c", 'a'+i)
		m[k] = i
		want = append(want, starlark.String(k))
	}
	v, err := convert.MakeDict(m)
	if err != nil {
		t.Fatal(err)
	}
	d := v.(*starlark.Dict)
	keys := d.Keys()
	if len(keys) != len(want) {
		t.Fatalf("expected %d keys, got %d", len(want), len(keys))
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("expected key order %v, got %v", want, keys)
		}
	}
}

// TestGoMapPopitemDeterministic verifies popitem pops entries in sorted key
// order instead of a random one.
func TestGoMapPopitemDeterministic(t *testing.T) {
	m := map[string]int{"c": 3, "a": 1, "b": 2}
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"m":      m,
	}
	code := []byte(`
assert.Eq(m.popitem(), ("a", 1))
assert.Eq(m.popitem(), ("b", 2))
assert.Eq(m.popitem(), ("c", 3))
assert.Eq(len(m), 0)
`)
	if _, err := starlight.Eval(code, globals, nil); err != nil {
		t.Fatal(err)
	}
}

// TestFromSetToSlice verifies the new ordered set materialization keeps the
// set's insertion order, unlike FromSet which returns an unordered Go map.
func TestFromSetToSlice(t *testing.T) {
	s := starlark.NewSet(3)
	for _, v := range []string{"c", "a", "b"} {
		if err := s.Insert(starlark.String(v)); err != nil {
			t.Fatal(err)
		}
	}
	got := convert.FromSetToSlice(s)
	want := []interface{}{"c", "a", "b"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}
