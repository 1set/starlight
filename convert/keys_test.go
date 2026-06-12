package convert_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
)

// Map-key conversion and deterministic ordering.
//
// Sections:
//   1. Unhashable / non-value-comparable keys (errors, not panics)
//   2. Tuple / bytes / big.Int keys -> comparable-by-value Go forms
//   3. TryFromDict / TryFromSet error variants
//   4. Deterministic key order (sortedMapKeys: type rank, then value)
//   5. Pointer and pointer-bearing keys sort address-free

// Regression tests for unhashable dict/set keys: a Starlark value whose Go
// form is not comparable (e.g. dict, list, set, or a tuple converted to
// []interface{}) used to escape as a runtime panic ("hash of unhashable
// type") and kill the host process. These tests pin the graceful behavior:
// errors for keys with no comparable Go form, and comparable equivalents
// (fixed-size arrays) for tuples and bytes.

// TestGoMapUnhashableKeyErrors verifies that scripts indexing a wrapped Go
// map with an unhashable key get a Starlark error instead of a host panic.
func TestGoMapUnhashableKeyErrors(t *testing.T) {
	tests := []fail{
		{`m[{1: 2}] = "x"`, "not hashable"},
		{`m[[1, 2]] = "x"`, "not hashable"},
		{`m[set([1])] = "x"`, "not hashable"},
		{`v = m[{1: 2}]`, "not hashable"},
		{`m.pop([1, 2])`, "not hashable"},
		{`m.get({1: 2})`, "not hashable"},
		{`m.setdefault([1], "x")`, "not hashable"},
	}
	for _, f := range tests {
		t.Run(f.code, func(t *testing.T) {
			m := map[interface{}]interface{}{"a": "b"}
			globals := map[string]interface{}{"m": m}
			_, err := starlight.Eval([]byte(f.code), globals, nil)
			if err == nil || !strings.Contains(err.Error(), f.err) {
				t.Fatalf(`expected error containing %q, got %v`, f.err, err)
			}
			if len(m) != 1 {
				t.Errorf("expected map to be unchanged, got %v", m)
			}
		})
	}
}

// TestGoMapTupleKey verifies that tuple keys work on wrapped Go maps with
// interface{} keys: they are stored as comparable fixed-size arrays, so
// writing and reading back with an equal tuple round-trips.
func TestGoMapTupleKey(t *testing.T) {
	m := map[interface{}]interface{}{}
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"m":      m,
	}
	code := []byte(`
m[(1, "a")] = "x"
assert.Eq(m[(1, "a")], "x")
assert.Eq(m.get((1, "a")), "x")
assert.Eq(len(m), 1)
v = m.pop((1, "a"))
assert.Eq(v, "x")
assert.Eq(len(m), 0)
`)
	if _, err := starlight.Eval(code, globals, nil); err != nil {
		t.Fatal(err)
	}
	// write again and inspect the Go-side key form
	code = []byte(`m[(1, "a")] = "y"`)
	if _, err := starlight.Eval(code, globals, nil); err != nil {
		t.Fatal(err)
	}
	wantKey := [2]interface{}{int64(1), "a"}
	if got, ok := m[wantKey]; !ok || got != "y" {
		t.Fatalf("expected m[%#v] == %q, map is %#v", wantKey, "y", m)
	}
}

// TestFromDictTupleKey verifies FromDict converts tuple keys to comparable
// fixed-size arrays instead of panicking on []interface{} map keys.
func TestFromDictTupleKey(t *testing.T) {
	d := starlark.NewDict(2)
	if err := d.SetKey(starlark.Tuple{starlark.String("A"), starlark.MakeInt(1)}, starlark.Bool(true)); err != nil {
		t.Fatal(err)
	}
	if err := d.SetKey(starlark.Tuple{starlark.String("B"), starlark.MakeInt(2)}, starlark.Bool(false)); err != nil {
		t.Fatal(err)
	}
	got := convert.FromDict(d)
	want := map[interface{}]interface{}{
		[2]interface{}{"A", int64(1)}: true,
		[2]interface{}{"B", int64(2)}: false,
	}
	if len(got) != len(want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
	for k, v := range want {
		if gv, ok := got[k]; !ok || gv != v {
			t.Fatalf("expected key %#v -> %#v in %#v", k, v, got)
		}
	}
}

// TestFromDictBytesKey verifies bytes keys become fixed-size byte arrays,
// which are comparable and stay distinct from equal string keys.
func TestFromDictBytesKey(t *testing.T) {
	d := starlark.NewDict(2)
	if err := d.SetKey(starlark.Bytes("ab"), starlark.MakeInt(1)); err != nil {
		t.Fatal(err)
	}
	if err := d.SetKey(starlark.String("ab"), starlark.MakeInt(2)); err != nil {
		t.Fatal(err)
	}
	got := convert.FromDict(d)
	if len(got) != 2 {
		t.Fatalf("expected bytes and string keys to stay distinct, got %#v", got)
	}
	if v, ok := got[[2]byte{'a', 'b'}]; !ok || v != int64(1) {
		t.Fatalf("expected bytes key [2]byte{'a','b'} -> 1, got %#v", got)
	}
	if v, ok := got["ab"]; !ok || v != int64(2) {
		t.Fatalf(`expected string key "ab" -> 2, got %#v`, got)
	}
}

// TestGoMapBigIntKey verifies a Starlark int too large for int64/uint64
// (which FromValue yields as a *big.Int) works as a Go map key. *big.Int is
// comparable in Go but only by pointer identity, so without canonicalizing
// the key, the same large int written and read back produced two different
// pointers — the value was stored but could never be retrieved.
func TestGoMapBigIntKey(t *testing.T) {
	m := map[interface{}]interface{}{}
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"m":      m,
	}
	code := []byte(`
k = 1 << 70
m[k] = "a"
assert.Eq(m[k], "a")
assert.Eq(k in m, True)
assert.Eq(len(m), 1)
# a second equal big int finds the same entry, not a new one
m[1 << 70] = "b"
assert.Eq(m[k], "b")
assert.Eq(len(m), 1)
`)
	if _, err := starlight.Eval(code, globals, nil); err != nil {
		t.Fatal(err)
	}
}

// TestFromDictBigIntKey verifies FromDict gives equal large-int keys a
// single, value-stable Go key (distinct from a different large int).
func TestFromDictBigIntKey(t *testing.T) {
	d := starlark.NewDict(2)
	big1 := starlark.MakeInt64(1).Lsh(70)
	big2 := starlark.MakeInt64(1).Lsh(80)
	if err := d.SetKey(big1, starlark.String("a")); err != nil {
		t.Fatal(err)
	}
	if err := d.SetKey(big2, starlark.String("b")); err != nil {
		t.Fatal(err)
	}
	got := convert.FromDict(d)
	if len(got) != 2 {
		t.Fatalf("expected two distinct big-int keys, got %#v", got)
	}
	// the two equal-value keys must collapse to one comparable Go key
	k1, err := convert.TryFromDict(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(k1) != 2 {
		t.Fatalf("expected two keys via TryFromDict, got %#v", k1)
	}
}

// TestFromSetTupleElem verifies FromSet converts tuple elements to
// comparable arrays instead of panicking.
func TestFromSetTupleElem(t *testing.T) {
	s := starlark.NewSet(2)
	if err := s.Insert(starlark.Tuple{starlark.MakeInt(1), starlark.String("A")}); err != nil {
		t.Fatal(err)
	}
	if err := s.Insert(starlark.Tuple{starlark.MakeInt(2), starlark.String("B")}); err != nil {
		t.Fatal(err)
	}
	got := convert.FromSet(s)
	want := map[interface{}]bool{
		[2]interface{}{int64(1), "A"}: true,
		[2]interface{}{int64(2), "B"}: true,
	}
	if len(got) != len(want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
	for k := range want {
		if !got[k] {
			t.Fatalf("expected member %#v in %#v", k, got)
		}
	}
}

// customHashableValue is a Starlark value that claims to be hashable on the
// Starlark side but converts to a non-comparable Go value (itself, holding a
// slice), to exercise the error paths of TryFromDict / TryFromSet.
type customHashableValue struct {
	s []string
}

func (customHashableValue) String() string        { return "customHashableValue" }
func (customHashableValue) Type() string          { return "customHashableValue" }
func (customHashableValue) Freeze()               {}
func (customHashableValue) Truth() starlark.Bool  { return true }
func (customHashableValue) Hash() (uint32, error) { return 42, nil }
func (v customHashableValue) CompareSameType(op syntax.Token, y starlark.Value, depth int) (bool, error) {
	eq := reflect.DeepEqual(v, y)
	switch op {
	case syntax.EQL:
		return eq, nil
	case syntax.NEQ:
		return !eq, nil
	}
	return false, fmt.Errorf("unsupported comparison %s", op)
}

// TestTryFromDict verifies the error-returning variant: clean dicts convert
// with a nil error, and keys with no comparable Go form yield an error.
func TestTryFromDict(t *testing.T) {
	d := starlark.NewDict(2)
	if err := d.SetKey(starlark.String("a"), starlark.MakeInt(1)); err != nil {
		t.Fatal(err)
	}
	if err := d.SetKey(starlark.Tuple{starlark.MakeInt(1)}, starlark.MakeInt(2)); err != nil {
		t.Fatal(err)
	}
	got, err := convert.TryFromDict(d)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(got) != 2 || got["a"] != int64(1) || got[[1]interface{}{int64(1)}] != int64(2) {
		t.Fatalf("unexpected conversion result: %#v", got)
	}

	bad := starlark.NewDict(1)
	if err := bad.SetKey(customHashableValue{s: []string{"x"}}, starlark.MakeInt(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := convert.TryFromDict(bad); err == nil || !strings.Contains(err.Error(), "hashable") {
		t.Fatalf("expected unhashable key error, got %v", err)
	}
}

// TestTryFromSet mirrors TestTryFromDict for sets.
func TestTryFromSet(t *testing.T) {
	s := starlark.NewSet(1)
	if err := s.Insert(starlark.Tuple{starlark.MakeInt(1)}); err != nil {
		t.Fatal(err)
	}
	got, err := convert.TryFromSet(s)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !got[[1]interface{}{int64(1)}] {
		t.Fatalf("unexpected conversion result: %#v", got)
	}

	bad := starlark.NewSet(1)
	if err := bad.Insert(customHashableValue{s: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := convert.TryFromSet(bad); err == nil || !strings.Contains(err.Error(), "hashable") {
		t.Fatalf("expected unhashable element error, got %v", err)
	}
}

// TestFromDictUnhashableFallback verifies the legacy FromDict keeps entries
// for keys with no comparable Go form by falling back to their printed form
// (instead of panicking or dropping data silently).
func TestFromDictUnhashableFallback(t *testing.T) {
	d := starlark.NewDict(1)
	if err := d.SetKey(customHashableValue{s: []string{"x"}}, starlark.MakeInt(1)); err != nil {
		t.Fatal(err)
	}
	got := convert.FromDict(d)
	if v, ok := got["customHashableValue"]; !ok || v != int64(1) {
		t.Fatalf("expected printed-form fallback key, got %#v", got)
	}
}

// Regression tests for identity-comparable map keys: a Starlark value whose
// Go form is comparable by POINTER IDENTITY rather than by value
// (*starlarkstruct.Struct, *starlarkstruct.Module, custom pointer-backed
// values) used to pass the reflect.Comparable() gate in hashableGoValue and
// silently key a wrapped Go map by identity — so equal values became
// distinct, unretrievable keys (the same class as the *big.Int bug). They
// must now error cleanly, like other unhashable-by-value keys.

func TestStructKeyErrorsNotIdentityKeyed(t *testing.T) {
	g := convert.NewGoMap(map[interface{}]interface{}{})
	st := starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"a": starlark.MakeInt(1),
	})
	err := g.SetKey(st, starlark.String("x"))
	if err == nil || !strings.Contains(err.Error(), "hashable") && !strings.Contains(err.Error(), "identity") {
		t.Fatalf("expected a clean key error for a struct value, got %v", err)
	}
}

// pointerCustom is a custom starlark.Value backed by a Go pointer (so
// FromValue returns it as-is and it compares by identity).
type pointerCustom struct{ n int }

func (p *pointerCustom) String() string        { return "pointerCustom" }
func (p *pointerCustom) Type() string          { return "pointerCustom" }
func (p *pointerCustom) Freeze()               {}
func (p *pointerCustom) Truth() starlark.Bool  { return true }
func (p *pointerCustom) Hash() (uint32, error) { return uint32(p.n), nil }

func TestPointerBackedCustomKeyErrors(t *testing.T) {
	g := convert.NewGoMap(map[interface{}]interface{}{})
	err := g.SetKey(&pointerCustom{n: 1}, starlark.String("x"))
	if err == nil {
		t.Fatal("expected a clean key error for a pointer-backed custom value, got nil")
	}
}

// big.Int keys must STILL work (canonicalized), and normal keys unaffected —
// regression guards for the generalization.
func TestNormalKeysStillWorkAfterIdentityGuard(t *testing.T) {
	g := convert.NewGoMap(map[interface{}]interface{}{})
	for _, k := range []starlark.Value{
		starlark.String("s"),
		starlark.MakeInt(7),
		starlark.MakeInt64(1).Lsh(70), // big.Int -> bigIntKey
		starlark.Tuple{starlark.MakeInt(1), starlark.String("a")},
		starlark.Bytes("b"),
	} {
		if err := g.SetKey(k, starlark.String("v")); err != nil {
			t.Fatalf("key %v should be usable, got %v", k, err)
		}
		if v, _, err := g.Get(k); err != nil || v != starlark.String("v") {
			t.Fatalf("key %v round-trip failed: v=%v err=%v", k, v, err)
		}
	}
}

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
