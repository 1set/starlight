package convert_test

import (
	"go.starlark.net/starlark"
	"strings"
	"testing"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
)

// Regression tests for checked conversions: the three conversion entry
// points trusted reflect.ConvertibleTo blindly, which silently corrupted
// values on the way into Go functions and collections:
//
//	fn(65)   -> func(string) received "A"  (Go codepoint conversion)
//	fn(1000) -> func(int8)   received -24  (integer wrap-around)
//	fn(3.9)  -> func(int)    received 3    (float truncation)
//	fn(None) -> func(int)    received 0    (None silently zeroed)
//
// All four must be regular errors; lossless conversions must keep working.

func evalErr(t *testing.T, code string, globals map[string]interface{}, want string) {
	t.Helper()
	_, err := starlight.Eval([]byte(code), globals, nil)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("%s: expected error containing %q, got %v", code, want, err)
	}
}

func evalOK(t *testing.T, code string, globals map[string]interface{}) {
	t.Helper()
	if _, err := starlight.Eval([]byte(code), globals, nil); err != nil {
		t.Fatalf("%s: unexpected error %v", code, err)
	}
}

// TestCheckedFuncArgs verifies the silent corruptions through function
// arguments are now errors, and lossless calls still work.
func TestCheckedFuncArgs(t *testing.T) {
	var gotS string
	var gotI8 int8
	var gotI int
	var gotU uint
	var gotF32 float32
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"fnS":    func(s string) { gotS = s },
		"fnI8":   func(i int8) { gotI8 = i },
		"fnI":    func(i int) { gotI = i },
		"fnU":    func(u uint) { gotU = u },
		"fnF32":  func(f float32) { gotF32 = f },
	}

	// silent corruption -> errors
	evalErr(t, `fnS(65)`, globals, "cannot be converted")
	evalErr(t, `fnI8(1000)`, globals, "out of range")
	evalErr(t, `fnI(3.9)`, globals, "would be truncated")
	evalErr(t, `fnU(-1)`, globals, "out of range")
	evalErr(t, `fnI(None)`, globals, "non-nullable")
	evalErr(t, `fnF32(3.5e108)`, globals, "out of range")
	if gotS != "" || gotI8 != 0 || gotI != 0 || gotU != 0 || gotF32 != 0 {
		t.Fatalf("corrupted values leaked through: %q %d %d %d %g", gotS, gotI8, gotI, gotU, gotF32)
	}

	// lossless conversions keep working
	evalOK(t, `fnI8(100)`, globals)
	if gotI8 != 100 {
		t.Fatalf("expected 100, got %d", gotI8)
	}
	evalOK(t, `fnI(3.0)`, globals) // whole float is lossless
	if gotI != 3 {
		t.Fatalf("expected 3, got %d", gotI)
	}
	evalOK(t, `fnU(42)`, globals)
	if gotU != 42 {
		t.Fatalf("expected 42, got %d", gotU)
	}
	evalOK(t, `fnF32(1.5)`, globals)
	if gotF32 != 1.5 {
		t.Fatalf("expected 1.5, got %g", gotF32)
	}
	evalOK(t, `fnS("hi")`, globals)
	if gotS != "hi" {
		t.Fatalf("expected hi, got %q", gotS)
	}
}

// TestCheckedUintAndEdgeSources covers the remaining numeric source
// classes: huge Starlark ints arrive as uint64 (too big for int64), floats
// can be NaN/negative, and huge uints must not fit smaller uints either.
func TestCheckedUintAndEdgeSources(t *testing.T) {
	var gotI8 int8
	var gotU8 uint8
	var gotU uint
	var gotI int
	var gotS string
	var gotF32 float32
	globals := map[string]interface{}{
		"fnI8":  func(i int8) { gotI8 = i },
		"fnU8":  func(u uint8) { gotU8 = u },
		"fnU":   func(u uint) { gotU = u },
		"fnI":   func(i int) { gotI = i },
		"fnS":   func(s string) { gotS = s },
		"fnF32": func(f float32) { gotF32 = f },
	}

	// 1e19 > MaxInt64: arrives as uint64
	evalErr(t, `fnI8(10000000000000000000)`, globals, "out of range")
	evalErr(t, `fnI(10000000000000000000)`, globals, "out of range")
	evalErr(t, `fnU8(10000000000000000000)`, globals, "out of range")
	evalErr(t, `fnS(10000000000000000000)`, globals, "cannot be converted")
	// float sources into integer targets
	evalErr(t, `fnU(3.9)`, globals, "would be truncated")
	evalErr(t, `fnU(-1.0)`, globals, "out of range")
	evalErr(t, `fnI(float("nan"))`, globals, "would be truncated")
	evalErr(t, `fnI(float("inf"))`, globals, "would be truncated")
	evalErr(t, `fnI8(1e30)`, globals, "out of range") // whole float, but far too large
	if gotI8 != 0 || gotU8 != 0 || gotU != 0 || gotI != 0 || gotS != "" || gotF32 != 0 {
		t.Fatalf("corrupted values leaked through: %d %d %d %d %q %g", gotI8, gotU8, gotU, gotI, gotS, gotF32)
	}

	// huge uint64 fits the wide targets
	evalOK(t, `fnU(10000000000000000000)`, globals)
	if gotU != 10000000000000000000 {
		t.Fatalf("expected 1e19, got %d", gotU)
	}
	// whole floats fit unsigned targets
	evalOK(t, `fnU8(200.0)`, globals)
	if gotU8 != 200 {
		t.Fatalf("expected 200, got %d", gotU8)
	}
}

// TestCheckedNoneArgs verifies None stays valid for nullable parameter
// types and is rejected for non-nullable ones (the two entry points used
// to disagree: one zeroed, one errored).
func TestCheckedNoneArgs(t *testing.T) {
	var gotP *int
	var calledP bool
	var gotSl []int
	var calledSl bool
	globals := map[string]interface{}{
		"fnP":  func(p *int) { gotP, calledP = p, true },
		"fnSl": func(s []int) { gotSl, calledSl = s, true },
	}
	evalOK(t, `fnP(None)`, globals)
	if !calledP || gotP != nil {
		t.Fatalf("expected nil pointer call, got %v %v", calledP, gotP)
	}
	evalOK(t, `fnSl(None)`, globals)
	if !calledSl || gotSl != nil {
		t.Fatalf("expected nil slice call, got %v %v", calledSl, gotSl)
	}
}

// TestCheckedCollectionWrites verifies the same checks guard writes into
// wrapped Go collections (append/setkey go through tryConv).
func TestCheckedCollectionWrites(t *testing.T) {
	s8 := []int8{1}
	m8 := map[string]int8{}
	globals := map[string]interface{}{
		"s8": s8,
		"m8": m8,
	}
	evalErr(t, `s8.append(1000)`, globals, "out of range")
	evalErr(t, `m8["a"] = 1000`, globals, "out of range")
	evalErr(t, `m8["a"] = 3.9`, globals, "would be truncated")
	if len(m8) != 0 {
		t.Fatalf("expected map unchanged, got %v", m8)
	}
	evalOK(t, `m8["a"] = 100`, globals)
	if m8["a"] != 100 {
		t.Fatalf("expected 100, got %v", m8)
	}
}

// TestCheckedVariadic verifies variadic arguments get the same checks.
func TestCheckedVariadic(t *testing.T) {
	var got []int8
	globals := map[string]interface{}{
		"fnV": func(vs ...int8) { got = vs },
	}
	evalErr(t, `fnV(1, 1000)`, globals, "out of range")
	if got != nil {
		t.Fatalf("corrupted variadic leaked through: %v", got)
	}
	evalOK(t, `fnV(1, 2, 3)`, globals)
	if len(got) != 3 || got[2] != 3 {
		t.Fatalf("expected [1 2 3], got %v", got)
	}
}

// TestCheckedConvertMapSlice verifies the deep map/slice argument
// conversion path (convertElemValue) applies the same checks.
func TestCheckedConvertMapSlice(t *testing.T) {
	var gotM map[string]int8
	var gotS []int8
	globals := map[string]interface{}{
		"fnM": func(m map[string]int8) { gotM = m },
		"fnS": func(s []int8) { gotS = s },
	}
	evalErr(t, `fnM({"a": 1000})`, globals, "out of range")
	evalErr(t, `fnS([1, 1000])`, globals, "out of range")
	if gotM != nil || gotS != nil {
		t.Fatalf("corrupted values leaked through: %v %v", gotM, gotS)
	}
	evalOK(t, `fnM({"a": 100})`, globals)
	if gotM["a"] != 100 {
		t.Fatalf("expected 100, got %v", gotM)
	}
	evalOK(t, `fnS([1, 2])`, globals)
	if len(gotS) != 2 {
		t.Fatalf("expected [1 2], got %v", gotS)
	}
}

// TestTryConvIntactForSafeConversions pins behaviors that must not change:
// identical types, named types, and string/bytes conversions.
func TestTryConvIntactForSafeConversions(t *testing.T) {
	type myString string
	var gotMy myString
	var gotB []byte
	globals := map[string]interface{}{
		"fnMy": func(s myString) { gotMy = s },
		"fnB":  func(b []byte) { gotB = b },
	}
	evalOK(t, `fnMy("hello")`, globals)
	if gotMy != "hello" {
		t.Fatalf("expected hello, got %q", gotMy)
	}
	evalOK(t, `fnB("bytes")`, globals)
	if string(gotB) != "bytes" {
		t.Fatalf("expected bytes, got %q", gotB)
	}
	_ = convert.FromValue // keep convert imported for symmetry with siblings
}

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
