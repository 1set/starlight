package convert_test

import (
	"testing"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
	"go.starlark.net/starlark"
)

// TestToValueTypedNilStarlarkPointer: a typed-nil pointer that satisfies the
// starlark.Value interface (e.g. (*starlark.List)(nil)) passed the assertion
// in ToValue and was returned as-is, panicking later when the interpreter
// dereferenced it. It must become None, like other nil-ish inputs.
func TestToValueTypedNilStarlarkPointer(t *testing.T) {
	var l *starlark.List
	var d *starlark.Dict
	for _, v := range []interface{}{l, d} {
		got, err := convert.ToValue(v)
		if err != nil {
			t.Fatalf("ToValue(%T) errored: %v", v, err)
		}
		if got != starlark.None {
			t.Fatalf("ToValue(%T nil) = %v, want None", v, got)
		}
	}
}

type boxNilErr struct{}

func (e *boxNilErr) Error() string { return "should-not-raise" }

// TestMakeOutBoxedNilError: a func declaring (T, error) that returns a
// typed-nil pointer boxed in the error interface (a common Go idiom) must
// not raise — the boxed nil is "no error".
func TestMakeOutBoxedNilError(t *testing.T) {
	globals := map[string]interface{}{
		"fn": func() (int, error) {
			var e *boxNilErr // typed nil
			return 42, e
		},
	}
	res, err := starlight.Eval([]byte(`x = fn()`), globals, nil)
	if err != nil {
		t.Fatalf("boxed typed-nil error must not raise, got %v", err)
	}
	if res["x"] != int64(42) {
		t.Fatalf("expected 42, got %v", res["x"])
	}
	// a real error still raises
	globals["fn2"] = func() (int, error) { return 0, &boxNilErr{} }
	if _, err := starlight.Eval([]byte(`x = fn2()`), globals, nil); err == nil {
		t.Fatal("a non-nil error should still raise")
	}
}
