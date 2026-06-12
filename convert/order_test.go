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
	// (interface-typed keys unwrap to Starlark values, hence the Starlark
	// formatting: capitalized bools, quoted strings)
	want := []string{"False", "True", "5", "7", "9", "2.5", `"a"`, `"b"`}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected key order %v, got %v", want, got)
		}
	}
}

// TestGoMapKeyTieBreak verifies that keys of different concrete types with
// equal rank and value (int8(5) vs int64(5)) still come out in a
// deterministic order — ties break on the concrete type name.
func TestGoMapKeyTieBreak(t *testing.T) {
	var want []string
	for run := 0; run < 20; run++ {
		m := map[interface{}]interface{}{
			int8(5):  "a",
			int64(5): "b",
			int32(5): "c",
		}
		var got []string
		for _, k := range convert.NewGoMap(m).Keys() {
			got = append(got, fmt.Sprintf("%T", convert.FromValue(k)))
		}
		if run == 0 {
			want = got
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("run %d: key order changed: %v vs %v", run, got, want)
			}
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

// valueOrder materializes a wrapped Go map's VALUES in the deterministic
// order sortedMapKeys imposes over its keys. Values are address-free, so a
// stable value sequence proves a stable key order without depending on how
// the keys themselves stringify.
func valueOrder(m interface{}) []string {
	g := convert.NewGoMap(m)
	var out []string
	for _, item := range g.Items() {
		out = append(out, string(item[1].(starlark.String)))
	}
	return out
}

// TestPointerKeyDeterministic: a map[*int]V key is a pointer; decorateKey
// classified it as rank-6 and sorted by fmt.Sprint of the ADDRESS, so the
// order varied run to run. After unwrapping the pointer it sorts by pointee
// value — stable across constructions (Go map iteration is randomized).
func TestPointerKeyDeterministic(t *testing.T) {
	mk := func() map[*int]string {
		a, b, c := 3, 1, 2
		return map[*int]string{&a: "a", &b: "b", &c: "c"}
	}
	var want []string
	for run := 0; run < 30; run++ {
		got := valueOrder(mk())
		if run == 0 {
			want = got
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("run %d: pointer-key order changed: %v vs %v", run, got, want)
			}
		}
	}
}

// TestPointerBearingStructKeyDeterministic: a map[struct{P *int}]V key is a
// struct containing a pointer; the rank-6 fmt.Sprint leaked the field's
// address into the sort key. An address-free render keeps the order stable.
func TestPointerBearingStructKeyDeterministic(t *testing.T) {
	type key struct {
		N int
		P *int
	}
	mk := func() map[key]string {
		x, y, z := 10, 20, 30
		return map[key]string{{N: 3, P: &x}: "a", {N: 1, P: &y}: "b", {N: 2, P: &z}: "c"}
	}
	var want []string
	for run := 0; run < 30; run++ {
		got := valueOrder(mk())
		if run == 0 {
			want = got
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("run %d: struct-ptr-key order changed: %v vs %v", run, got, want)
			}
		}
	}
}
