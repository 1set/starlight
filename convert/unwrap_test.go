package convert_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
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

// TestGoInterfaceWrapperBasics pins the wrapper's value semantics directly:
// printable, truthy, unhashable, and freezable as a no-op.
func TestGoInterfaceWrapperBasics(t *testing.T) {
	m := map[string]error{"e": wrappedError{msg: "boom"}}
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"m":      m,
	}
	code := []byte(`
e = m["e"]
assert.Eq(str(e), "boom")
assert.Eq(bool(e), True)
d = {}
ok = "no error"
`)
	if _, err := starlight.Eval(code, globals, nil); err != nil {
		t.Fatal(err)
	}
	// unhashable: using the wrapper as a dict key must error, not panic
	_, err := starlight.Eval([]byte(`d = {m["e"]: 1}`), globals, nil)
	if err == nil {
		t.Fatal("expected unhashable error for wrapper as dict key")
	}
	// Freeze is a documented no-op (no write path) and must not affect use
	v, err := convert.ToValue(loudInt(3))
	if err != nil {
		t.Fatal(err)
	}
	v.Freeze()
	if v.Truth() != true {
		t.Fatalf("expected truthy wrapper after freeze")
	}
	if _, err := v.(*convert.GoInterface).Hash(); err == nil {
		t.Fatal("expected hash error for interface wrapper")
	}
	if s := v.String(); s != "3" {
		t.Fatalf("expected 3, got %q", s)
	}
}

// Regression tests for dynamic values the unwrapping cannot convert: after
// empty interfaces started unwrapping to their dynamic value, an element
// whose dynamic type toValue does not support (e.g. a chan) produced a
// conversion error inside methods that cannot return errors — items(),
// values(), GoSlice indexing and iteration all panicked and killed the
// host. The static collection pre-check cannot help: interface{} element
// types are checked at runtime, value by value. Such values must fall back
// to the opaque interface wrapper (the pre-unwrap behavior): inert but
// printable, and never a panic.

// TestDynamicUnsupportedMapValue drives every materialization path of a
// map[string]interface{} holding a chan.
func TestDynamicUnsupportedMapValue(t *testing.T) {
	ch := make(chan int)
	codes := []string{
		`x = m.items()`,
		`x = m.values()`,
		`x = m.keys()`,
		`x = [k for k in m]`,
		`x = m["bad"]`,
		`x = str(m)`,
		`x = len(m)`,
		`x = m.get("bad")`,
		`x = m.pop("bad")`,
	}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			globals := map[string]interface{}{
				"m": map[string]interface{}{"bad": ch, "good": 1},
			}
			if _, err := starlight.Eval([]byte(code), globals, nil); err != nil {
				t.Fatalf("%s: expected graceful success, got %v", code, err)
			}
		})
	}
	// the unsupported value comes through as the opaque wrapper
	globals := map[string]interface{}{
		"m": map[string]interface{}{"bad": ch},
	}
	res, err := starlight.Eval([]byte(`t = type(m["bad"])`), globals, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := res["t"].(string); !ok || !strings.HasPrefix(s, "starlight_") {
		t.Fatalf("expected opaque wrapper type, got %v", res["t"])
	}
}

// TestDynamicUnsupportedSliceElem drives the GoSlice paths (Index and the
// iterator cannot return errors).
func TestDynamicUnsupportedSliceElem(t *testing.T) {
	for _, code := range []string{
		`x = l[0]`,
		`x = [e for e in l]`,
		`x = str(l)`,
		`x = len(l)`,
		`x = l.find(1)`,
	} {
		t.Run(code, func(t *testing.T) {
			globals := map[string]interface{}{
				"l": []interface{}{make(chan int), 1},
			}
			if _, err := starlight.Eval([]byte(code), globals, nil); err != nil {
				t.Fatalf("%s: expected graceful success, got %v", code, err)
			}
		})
	}
}

// TestDynamicUnsupportedNested covers an interface holding a collection
// whose static element type is unsupported (rejected by the pre-check when
// unwrapped) — it must also degrade to the wrapper, not an error or panic.
func TestDynamicUnsupportedNested(t *testing.T) {
	globals := map[string]interface{}{
		"m": map[string]interface{}{"inner": map[string]chan int{"c": make(chan int)}},
	}
	res, err := starlight.Eval([]byte(`x = m.items()
t = type(m["inner"])`), globals, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := res["t"].(string); !ok || !strings.HasPrefix(s, "starlight_") {
		t.Fatalf("expected opaque wrapper for nested unsupported collection, got %v", res["t"])
	}
}

// TestStaticUnsupportedStillErrors pins the boundary of the degradation:
// the wrapper fallback applies only to values hidden behind interface{}
// (unknowable at wrap time). Collections whose static key/element type is
// unsupported keep failing the conversion up front.
func TestStaticUnsupportedStillErrors(t *testing.T) {
	if _, err := convert.MakeDict(map[string]complex128{"b": 3 + 4i}); err == nil {
		t.Fatal("expected error for statically unsupported value type")
	}
	if _, err := convert.MakeDict(map[complex64]string{3i: "b"}); err == nil {
		t.Fatal("expected error for statically unsupported key type")
	}
	if _, err := convert.ToValue(map[string]chan int{}); err == nil {
		t.Fatal("expected error from the static element pre-check")
	}
}

// TestDynamicSupportedStillUnwraps pins that the fallback does not undo the
// unwrapping for supported dynamic values.
func TestDynamicSupportedStillUnwraps(t *testing.T) {
	globals := map[string]interface{}{
		"m": map[string]interface{}{"a": 1},
	}
	res, err := starlight.Eval([]byte(`t = type(m["a"])
v = m["a"] + 1`), globals, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res["t"] != "int" || res["v"] != int64(2) {
		t.Fatalf("expected unwrapped int arithmetic, got %v", res)
	}
}
