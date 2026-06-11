package convert_test

import (
	"errors"
	"testing"

	"github.com/1set/starlight"
)

// Regression tests for empty-interface unwrapping: JSON-shaped data
// (map[string]interface{}, []interface{}) used to convert its elements into
// opaque interface wrappers, so m["a"] == 1 was False, m["a"] + 1 failed
// with an unknown binary op, and type() reported starlight_interface<int>.

// TestJSONShapedDataUsable verifies elements of empty-interface collections
// behave as native Starlark values.
func TestJSONShapedDataUsable(t *testing.T) {
	m := map[string]interface{}{
		"a": 1,
		"s": "x",
		"f": 2.5,
		"b": true,
		"n": nil,
		"l": []interface{}{1, "two", 3.0},
		"d": map[string]interface{}{"inner": 42},
	}
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"m":      m,
	}
	code := []byte(`
assert.Eq(m["a"] == 1, True)
assert.Eq(m["a"] + 1, 2)
assert.Eq(type(m["a"]), "int")
assert.Eq(m["s"] + "!", "x!")
assert.Eq(m["f"] * 2, 5.0)
assert.Eq(m["b"], True)
assert.Eq(m["n"], None)
assert.Eq(m["l"][1], "two")
assert.Eq(m["d"]["inner"] + 0, 42)
total = m["a"] + m["l"][0]
assert.Eq(total, 2)
`)
	if _, err := starlight.Eval(code, globals, nil); err != nil {
		t.Fatal(err)
	}
}

type wrappedError struct{ msg string }

func (w wrappedError) Error() string { return w.msg }

// TestNonEmptyInterfaceKeepsWrapper verifies interfaces with methods keep
// the GoInterface wrapper, which is what exposes those methods.
func TestNonEmptyInterfaceKeepsWrapper(t *testing.T) {
	m := map[string]error{"e": wrappedError{msg: "boom"}}
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"m":      m,
	}
	code := []byte(`
e = m["e"]
assert.Eq(type(e).startswith("starlight_"), True)
assert.Eq(e.Error(), "boom")
`)
	if _, err := starlight.Eval(code, globals, nil); err != nil {
		t.Fatal(err)
	}
	_ = errors.New // keep errors imported alongside the error-shaped fixture
}
