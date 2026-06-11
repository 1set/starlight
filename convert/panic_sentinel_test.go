package convert

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestToValuePanicSentinel verifies the conversion boundary's recover
// produces a typed *PanicError carrying the recovered value and the stack
// where the panic started, instead of a bare message that hides the
// origin. The panic is provoked through reflect: reading an unexported
// field yields a value whose Interface() panics.
func TestToValuePanicSentinel(t *testing.T) {
	type hidden struct {
		secret string //nolint:unused // read via reflect to provoke the panic
	}
	rv := reflect.ValueOf(hidden{secret: "x"}).Field(0)
	if rv.CanInterface() {
		t.Fatal("test setup broken: expected an unexported field value")
	}

	v, err := toValue(rv, "")
	if err == nil {
		t.Fatalf("expected an error, got value %v", v)
	}
	var pe *PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *PanicError, got %T: %v", err, err)
	}
	if pe.Value == nil || len(pe.Stack) == 0 {
		t.Fatalf("expected recovered value and stack, got %+v", pe)
	}
	if !strings.Contains(err.Error(), "panic recovered") {
		t.Fatalf("expected historic message prefix, got %q", err.Error())
	}
	if !strings.Contains(string(pe.Stack), "toValue") {
		t.Fatalf("expected stack to identify the conversion frame, got:\n%s", pe.Stack)
	}
}
