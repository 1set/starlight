package convert_test

import (
	"strings"
	"testing"

	"github.com/1set/starlight"
)

// Regression tests for None elements in typed collection arguments: a None
// inside a list passed to a Go function taking a typed slice hit
// reflect.Value.Type on a zero Value inside convertSlice — an internal
// reflect panic surfaced through the recover as a confusing error instead
// of a clean checked-conversion error. None must follow the same policy as
// scalar arguments: allowed for nullable element types, a clear error for
// non-nullable ones.

// TestNoneSliceElem covers both sides of the policy for slice arguments.
func TestNoneSliceElem(t *testing.T) {
	var gotInts []int
	var gotPtrs []*int
	globals := map[string]interface{}{
		"fnInts": func(s []int) { gotInts = s },
		"fnPtrs": func(s []*int) { gotPtrs = s },
	}

	_, err := starlight.Eval([]byte(`fnInts([1, None])`), globals, nil)
	if err == nil || !strings.Contains(err.Error(), "non-nullable") {
		t.Fatalf("expected non-nullable element error, got %v", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "reflect") || strings.Contains(err.Error(), "panic") {
		t.Fatalf("internal reflect panic leaked into the error: %v", err)
	}
	if gotInts != nil {
		t.Fatalf("corrupted slice leaked through: %v", gotInts)
	}

	if _, err := starlight.Eval([]byte(`fnPtrs([None, None])`), globals, nil); err != nil {
		t.Fatalf("None must be valid for nullable element types, got %v", err)
	}
	if len(gotPtrs) != 2 || gotPtrs[0] != nil || gotPtrs[1] != nil {
		t.Fatalf("expected [nil nil], got %v", gotPtrs)
	}
}

// TestNoneMapElem mirrors the policy for typed map arguments.
func TestNoneMapElem(t *testing.T) {
	var gotInts map[string]int
	var gotPtrs map[string]*int
	globals := map[string]interface{}{
		"fnInts": func(m map[string]int) { gotInts = m },
		"fnPtrs": func(m map[string]*int) { gotPtrs = m },
	}

	_, err := starlight.Eval([]byte(`fnInts({"a": None})`), globals, nil)
	if err == nil || !strings.Contains(err.Error(), "non-nullable") {
		t.Fatalf("expected non-nullable element error, got %v", err)
	}
	if gotInts != nil {
		t.Fatalf("corrupted map leaked through: %v", gotInts)
	}

	if _, err := starlight.Eval([]byte(`fnPtrs({"a": None})`), globals, nil); err != nil {
		t.Fatalf("None must be valid for nullable element types, got %v", err)
	}
	if len(gotPtrs) != 1 || gotPtrs["a"] != nil {
		t.Fatalf("expected {a: nil}, got %v", gotPtrs)
	}
}
