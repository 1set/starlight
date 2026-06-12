package convert_test

import (
	"strings"
	"testing"

	"github.com/1set/starlight"
)

// TestCheckedConvertUintptr: checkedConvert had no reflect.Uintptr target
// case, so a numeric Starlark value converted to a uintptr parameter
// skipped the negative/overflow/truncation guards and wrapped silently.
func TestCheckedConvertUintptr(t *testing.T) {
	var got uintptr
	globals := map[string]interface{}{
		"fn": func(u uintptr) { got = u },
	}
	// negative -> error (a uintptr cannot be negative)
	if _, err := starlight.Eval([]byte(`fn(-1)`), globals, nil); err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected out-of-range error for fn(-1), got %v (got=%d)", err, got)
	}
	// fractional -> truncation error
	if _, err := starlight.Eval([]byte(`fn(3.9)`), globals, nil); err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("expected truncation error for fn(3.9), got %v", err)
	}
	// in-range value still works
	got = 0
	if _, err := starlight.Eval([]byte(`fn(42)`), globals, nil); err != nil {
		t.Fatalf("fn(42) should work, got %v", err)
	}
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}
