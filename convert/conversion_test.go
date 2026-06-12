package convert_test

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
	startime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
)

// Value conversion correctness and safety (Go<->Starlark).
//
// Sections:
//   1. Checked numeric conversions (overflow, codepoint, truncation, None)
//   2. Typed collection element conversion (no panic, checked, uintptr)
//   3. Empty-interface unwrapping and the opaque-wrapper fallback
//   4. Type mapping edges: time.Duration, Int ladder, *time.Time
//   5. Typed-nil handling; kwargs; NewStruct misuse

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

// Consolidated small black-box tests from the review round: popitem
// guards, time.Duration symmetry / Int ladder, and the kwargs-message /
// reachable-branch cleanups.

// Regression tests for dict.popitem's mutation guards. Every other GoMap
// mutation entry point (SetKey, Clear, Delete/pop) rejects writes on a
// frozen map and during iteration; popitem called the internal unguarded
// delete, so it silently mutated frozen maps and mutated maps mid-iteration
// where its sibling pop() correctly errors.

// TestPopitemFrozen verifies popitem on a frozen GoMap errors instead of
// mutating, matching pop().
func TestPopitemFrozen(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	g := convert.NewGoMap(m)
	g.Freeze()
	globals := map[string]interface{}{"m": g}

	_, err := starlight.Eval([]byte(`x = m.popitem()`), globals, nil)
	if err == nil || !strings.Contains(err.Error(), "frozen") {
		t.Fatalf("expected frozen error, got %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("expected frozen map unchanged, got %v", m)
	}
}

// TestPopitemDuringIteration verifies popitem during an active iteration of
// the same map errors, matching pop(). Driven at the API level because a
// Starlark for-loop must live in a function.
func TestPopitemDuringIteration(t *testing.T) {
	g := convert.NewGoMap(map[string]int{"a": 1, "b": 2, "c": 3})
	it := g.Iterate()
	defer it.Done()
	var k starlark.Value
	it.Next(&k) // open the iteration (numIt > 0)

	piFn, err := g.Attr("popitem")
	if err != nil || piFn == nil {
		t.Fatal("no popitem attr")
	}
	_, piErr := starlark.Call(&starlark.Thread{}, piFn.(*starlark.Builtin), nil, nil)
	if piErr == nil || !strings.Contains(piErr.Error(), "during iteration") {
		t.Fatalf("expected during-iteration error, got %v", piErr)
	}

	// pop() agrees
	popFn, _ := g.Attr("pop")
	_, popErr := starlark.Call(&starlark.Thread{}, popFn.(*starlark.Builtin), starlark.Tuple{starlark.String("a")}, nil)
	if popErr == nil || !strings.Contains(popErr.Error(), "during iteration") {
		t.Fatalf("pop should also error during iteration, got %v", popErr)
	}
}

// TestPopitemGuardsMatchPop directly compares popitem and pop guard
// behavior at the API level to ensure they agree.
func TestPopitemGuardsMatchPop(t *testing.T) {
	makeFrozen := func() *convert.GoMap {
		g := convert.NewGoMap(map[string]int{"a": 1})
		g.Freeze()
		return g
	}
	// pop on frozen
	gp := makeFrozen()
	popFn, err := gp.Attr("pop")
	if err != nil || popFn == nil {
		t.Fatal("no pop attr")
	}
	_, popErr := starlark.Call(&starlark.Thread{}, popFn.(*starlark.Builtin), starlark.Tuple{starlark.String("a")}, nil)
	// popitem on frozen
	gpi := makeFrozen()
	piFn, err := gpi.Attr("popitem")
	if err != nil || piFn == nil {
		t.Fatal("no popitem attr")
	}
	_, piErr := starlark.Call(&starlark.Thread{}, piFn.(*starlark.Builtin), nil, nil)

	if popErr == nil || piErr == nil {
		t.Fatalf("both must error on frozen: pop=%v popitem=%v", popErr, piErr)
	}
	if !strings.Contains(piErr.Error(), "frozen") {
		t.Fatalf("popitem frozen error should mention frozen, got %v", piErr)
	}
}

// TestPopitemStillWorks pins the happy path: popitem on a mutable,
// non-iterating map still pops the deterministic smallest key.
func TestPopitemStillWorks(t *testing.T) {
	m := map[string]int{"c": 3, "a": 1, "b": 2}
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"m":      m,
	}
	code := []byte(`
assert.Eq(m.popitem(), ("a", 1))
assert.Eq(m.popitem(), ("b", 2))
assert.Eq(len(m), 1)
`)
	if _, err := starlight.Eval(code, globals, nil); err != nil {
		t.Fatal(err)
	}
}

// TestDurationSymmetry verifies time.Duration maps bidirectionally to the
// standard Starlark time.duration. It used to convert one way only: into
// an opaque interface wrapper that scripts could not use as a duration,
// while time.duration values coming back were never unwrapped.
func TestDurationSymmetry(t *testing.T) {
	d := 90 * time.Second

	v, err := convert.ToValue(d)
	if err != nil {
		t.Fatal(err)
	}
	sd, ok := v.(startime.Duration)
	if !ok {
		t.Fatalf("expected startime.Duration, got %T", v)
	}
	if time.Duration(sd) != d {
		t.Fatalf("expected %v, got %v", d, time.Duration(sd))
	}

	back := convert.FromValue(v)
	gd, ok := back.(time.Duration)
	if !ok || gd != d {
		t.Fatalf("expected round-trip %v, got %v (%T)", d, back, back)
	}

	// script-visible: it is a real time.duration, with duration semantics
	globals := map[string]interface{}{"d": d}
	res, err := starlight.Eval([]byte(`
t = type(d)
double = d + d
seconds = d.seconds
`), globals, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res["t"] != "time.duration" {
		t.Fatalf("expected time.duration, got %v", res["t"])
	}
	if dd, ok := res["double"].(time.Duration); !ok || dd != 3*time.Minute {
		t.Fatalf("expected 3m, got %v (%T)", res["double"], res["double"])
	}
	if res["seconds"] != float64(90) {
		t.Fatalf("expected 90 seconds, got %v", res["seconds"])
	}

	// duration arguments convert back for Go functions
	var got time.Duration
	globals2 := map[string]interface{}{
		"d":   d,
		"fnD": func(in time.Duration) { got = in },
	}
	if _, err := starlight.Eval([]byte(`fnD(d)`), globals2, nil); err != nil {
		t.Fatal(err)
	}
	if got != d {
		t.Fatalf("expected %v, got %v", d, got)
	}
}

// TestIntLadderContract pins the documented FromValue integer ladder:
// int64 if it fits, else uint64, else *big.Int.
func TestIntLadderContract(t *testing.T) {
	small := convert.FromValue(starlark.MakeInt(42))
	if v, ok := small.(int64); !ok || v != 42 {
		t.Fatalf("expected int64 42, got %v (%T)", small, small)
	}

	negative := convert.FromValue(starlark.MakeInt(-42))
	if v, ok := negative.(int64); !ok || v != -42 {
		t.Fatalf("expected int64 -42, got %v (%T)", negative, negative)
	}

	wide := convert.FromValue(starlark.MakeUint64(10000000000000000000)) // > MaxInt64
	if v, ok := wide.(uint64); !ok || v != 10000000000000000000 {
		t.Fatalf("expected uint64 1e19, got %v (%T)", wide, wide)
	}

	huge, ok := new(big.Int).SetString("36893488147419103232", 10) // 2^65
	if !ok {
		t.Fatal("bad big literal")
	}
	b := convert.FromValue(starlark.MakeBigInt(huge))
	if v, ok := b.(*big.Int); !ok || v.Cmp(huge) != 0 {
		t.Fatalf("expected *big.Int 2^65, got %v (%T)", b, b)
	}
}

// TestKwargsErrorNotDoubleQuoted verifies the unexpected-keyword error names
// the argument once, not double-quoted. kwargs[0][0] is a starlark.String
// whose String() already quotes, so the old %q produced the literal
// "\"name\"".
func TestKwargsErrorNotDoubleQuoted(t *testing.T) {
	globals := map[string]interface{}{
		"quote":  func(s string) string { return s },
		"concat": func(ss ...string) string { return strings.Join(ss, "") },
	}
	for _, code := range []string{`quote("a", bogus=1)`, `concat("a", bogus=1)`} {
		_, err := starlight.Eval([]byte(code), globals, nil)
		if err == nil {
			t.Fatalf("%s: expected error", code)
		}
		msg := err.Error()
		if !strings.Contains(msg, `argument "bogus"`) {
			t.Fatalf("%s: expected single-quoted arg name, got %q", code, msg)
		}
		if strings.Contains(msg, `\"bogus\"`) || strings.Contains(msg, `"\"bogus\""`) {
			t.Fatalf("%s: argument name is double-quoted: %q", code, msg)
		}
	}
}

// reNarrowValue is a custom starlark.Value FromValue leaves as-is, used to
// reach the collection-element starlark.Value re-narrowing branch in
// convertElemValue (the one whose comment used to claim it was unreachable).
type reNarrowValue struct{ n int64 }

func (reNarrowValue) String() string        { return "reNarrowValue" }
func (reNarrowValue) Type() string          { return "reNarrowValue" }
func (reNarrowValue) Freeze()               {}
func (reNarrowValue) Truth() starlark.Bool  { return true }
func (reNarrowValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable") }

// TestConvertElemStarlarkValueBranchReachable proves the branch is live: a
// custom starlark.Value inside a collection passed to a typed Go parameter
// reaches the re-narrowing path and errors cleanly (it is not numeric).
func TestConvertElemStarlarkValueBranchReachable(t *testing.T) {
	fn := convert.MakeStarFn("fn", func(m map[string]int) int { return len(m) })
	globals := starlark.StringDict{"fn": fn, "c": reNarrowValue{n: 5}}
	_, err := starlark.ExecFile(&starlark.Thread{}, "t.star", `x = fn({"a": c})`, globals)
	if err == nil {
		t.Fatal("expected a conversion error for a non-numeric custom value")
	}
	if !strings.Contains(err.Error(), "reNarrowValue") {
		t.Fatalf("expected error to mention the custom type, got %v", err)
	}
}

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

// TestPointerToTimeTime: a *time.Time reached the struct case still as a
// pointer and was wrapped as a GoStruct instead of the Starlark time type.
func TestPointerToTimeTime(t *testing.T) {
	now := time.Date(2026, 6, 12, 1, 2, 3, 0, time.UTC)
	v, err := convert.ToValue(&now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := v.(startime.Time); !ok {
		t.Fatalf("expected startime.Time for *time.Time, got %T", v)
	}
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"t":      &now,
	}
	if _, err := starlight.Eval([]byte(`
assert.Eq(type(t), "time.time")
assert.Eq(t.year, 2026)
`), globals, nil); err != nil {
		t.Fatal(err)
	}
}

// TestNewStructNilMessage: NewStruct/NewStructWithTag formatted their panic
// with val.Interface(), which panics again on a nil (Invalid) arg. The panic
// must carry a clean "<nil>".
func TestNewStructNilMessage(t *testing.T) {
	for _, fn := range []func(){
		func() { convert.NewStruct(nil) },
		func() { convert.NewStructWithTag(nil, "tag") },
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("expected panic for nil arg")
				}
				msg := fmt.Sprint(r)
				if strings.Contains(msg, "zero Value") {
					t.Fatalf("panic-message formatting itself panicked: %v", r)
				}
				if !strings.Contains(msg, "<nil>") {
					t.Fatalf("expected '<nil>' in panic, got %v", r)
				}
			}()
			fn()
		}()
	}
}
