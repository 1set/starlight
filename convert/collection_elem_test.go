package convert_test

import (
	"strings"
	"testing"

	"github.com/1set/starlight"
	"go.starlark.net/starlark"
)

// Regression tests for typed-collection argument conversion.

// TestConvertSliceConcreteElemNoPanic: a wrapped Go slice with concrete
// element kind (e.g. []int8), passed to a Go func whose parameter element
// type it is neither assignable nor convertible to, used to reach
// convertSlice's elem.Elem() on a non-pointer/non-interface value and panic
// internally (recovered into a confusing "reflect:" error). It must produce
// a clean type-mismatch error instead.
func TestConvertSliceConcreteElemNoPanic(t *testing.T) {
	globals := map[string]interface{}{
		"fn": func(s []chan int) int { return len(s) },
		"s":  []int8{1, 2, 3},
	}
	_, err := starlight.Eval([]byte(`x = fn(s)`), globals, nil)
	if err == nil {
		t.Fatal("expected a type-mismatch error")
	}
	if strings.Contains(err.Error(), "reflect:") || strings.Contains(err.Error(), "Elem on") {
		t.Fatalf("internal reflect panic leaked into the error: %v", err)
	}
}

// TestConvertElemStarlarkValueChecked: a Go map holding a raw starlark.Value
// element, passed to a func with a narrow integer element type, went through
// the unchecked convertNumericTypes and silently wrapped an out-of-range
// value. It must now error like every other checked conversion.
func TestConvertElemStarlarkValueChecked(t *testing.T) {
	var got int8
	globals := map[string]interface{}{
		"fn": func(m map[string]int8) { got = m["k"] },
		"m":  map[string]interface{}{"k": starlark.MakeInt(1000)}, // 1000 overflows int8
	}
	_, err := starlight.Eval([]byte(`fn(m)`), globals, nil)
	if err == nil {
		t.Fatalf("expected out-of-range error; instead got silent value %d", got)
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected out-of-range error, got %v", err)
	}
	// a None value into a non-convertible element type (bool) reaches the
	// starlark.Value branch's nil-guard cleanly (not a panic)
	gb := map[string]interface{}{
		"fn": func(m map[string]bool) {},
		"m":  map[string]interface{}{"k": starlark.None},
	}
	if _, err := starlight.Eval([]byte(`fn(m)`), gb, nil); err == nil {
		t.Fatal("expected error for None into a bool element")
	}

	// an in-range value still works
	got = 0
	globals["m"] = map[string]interface{}{"k": starlark.MakeInt(100)}
	if _, err := starlight.Eval([]byte(`fn(m)`), globals, nil); err != nil {
		t.Fatalf("in-range value should convert, got %v", err)
	}
	if got != 100 {
		t.Fatalf("expected 100, got %d", got)
	}
}
