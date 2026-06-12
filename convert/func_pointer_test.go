package convert_test

import (
	"strings"
	"testing"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
)

// Regression tests for pointer-to-func: toValue's pre-switch deref left
// `*func` as a pointer while setting kind=Func, so the Func case called
// makeStarFn on a pointer and panicked. A non-nil *func should deref to a
// callable; a nil *func and *func-element collections should error cleanly,
// never panic.

func TestPointerToFuncCallable(t *testing.T) {
	fn := func(x int) int { return x * 2 }
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"f":      &fn, // *func
	}
	if _, err := starlight.Eval([]byte(`assert.Eq(f(21), 42)`), globals, nil); err != nil {
		t.Fatalf("*func should be callable, got %v", err)
	}
}

func TestNilPointerToFuncErrors(t *testing.T) {
	var fn *func(int) int
	_, err := convert.ToValue(fn)
	if err == nil {
		t.Fatal("nil *func should error, not panic")
	}
}

func TestFuncPointerSliceRejected(t *testing.T) {
	fn := func() {}
	// a slice of *func: rejected at conversion time, not a panic on access
	if _, err := convert.ToValue([]*func(){&fn}); err == nil {
		t.Fatal("[]*func should be rejected at conversion time")
	}
	// plain []func still works (each element is callable)
	v, err := convert.ToValue([]func(){fn})
	if err != nil {
		t.Fatalf("[]func should convert, got %v", err)
	}
	if v == nil {
		t.Fatal("expected a wrapped slice")
	}
	_ = strings.TrimSpace
}
