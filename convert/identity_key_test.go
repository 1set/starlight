package convert_test

import (
	"strings"
	"testing"

	"github.com/1set/starlight/convert"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

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
