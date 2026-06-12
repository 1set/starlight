package convert_test

import (
	"testing"

	"github.com/1set/starlight/convert"
	"go.starlark.net/starlark"
)

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
