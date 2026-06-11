package starlight

import (
	"testing"

	"go.starlark.net/resolve"
)

// TestNoGlobalDialectMutation verifies that importing starlight (and,
// transitively, convert) no longer mutates the process-global resolve
// flags: the dialect is passed explicitly to every compile/exec call.
// Before, any import of these packages rewrote the dialect for every other
// Starlark user in the same process.
func TestNoGlobalDialectMutation(t *testing.T) {
	if resolve.AllowSet {
		t.Fatal("resolve.AllowSet was mutated by import")
	}
}

// TestDialectCapabilities verifies the starlight entry points still compile
// the full dialect (set built-in plus the standard nested def / lambda /
// float / bitwise features) without the global flags.
func TestDialectCapabilities(t *testing.T) {
	res, err := Eval([]byte(`
s = set([1, 2, 3])
n = len(s)
f = (lambda x: x * 2)(21)
fl = 1.5 * 2
b = 6 & 3

def outer():
    def inner():
        return 1
    return inner()

d = outer()
`), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]interface{}{
		"n":  int64(3),
		"f":  int64(42),
		"fl": float64(3),
		"b":  int64(2),
		"d":  int64(1),
	} {
		if res[k] != want {
			t.Fatalf("expected %s == %v, got %v (%T)", k, want, res[k], res[k])
		}
	}
}
