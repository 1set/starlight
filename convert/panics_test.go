package convert_test

import (
	"strings"
	"testing"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
)

// Regression tests for the four script-reachable panics:
//  1. strided slicing (l[::2]) passed the element type to MakeSlice and
//     panicked on every use — strided slices never worked;
//  2. first write to a wrapped nil Go map panicked with "assignment to
//     entry in nil map";
//  3. Go arrays were wrapped as GoSlice but append/assignment panicked
//     (reflect.Append on array / unaddressable value);
//  4. collections with element types toValue cannot convert (e.g. chan)
//     wrapped fine but panicked later in items()/iteration.

// TestStridedSlice verifies strided slicing works instead of panicking.
func TestStridedSlice(t *testing.T) {
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"l":      []int{1, 2, 3, 4, 5, 6},
	}
	code := []byte(`
l2 = l[::2]
assert.Eq(len(l2), 3)
assert.Eq(l2[0], 1)
assert.Eq(l2[1], 3)
assert.Eq(l2[2], 5)
l3 = l[::-1]
assert.Eq(len(l3), 6)
assert.Eq(l3[0], 6)
assert.Eq(l3[5], 1)
l4 = l[1:5:2]
assert.Eq(len(l4), 2)
assert.Eq(l4[0], 2)
assert.Eq(l4[1], 4)
`)
	if _, err := starlight.Eval(code, globals, nil); err != nil {
		t.Fatal(err)
	}
}

// TestNilMapWrite verifies the first write to a wrapped nil Go map is a
// regular error instead of a panic, and that reads stay graceful.
func TestNilMapWrite(t *testing.T) {
	var m map[string]int
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"m":      m,
	}
	code := []byte(`
assert.Eq(len(m), 0)
assert.Eq(m.get("a", 42), 42)
`)
	if _, err := starlight.Eval(code, globals, nil); err != nil {
		t.Fatal(err)
	}
	_, err := starlight.Eval([]byte(`m["a"] = 1`), globals, nil)
	if err == nil || !strings.Contains(err.Error(), "nil map") {
		t.Fatalf("expected nil map error, got %v", err)
	}
	_, err = starlight.Eval([]byte(`m.setdefault("a", 1)`), globals, nil)
	if err == nil || !strings.Contains(err.Error(), "nil map") {
		t.Fatalf("expected nil map error, got %v", err)
	}
}

// TestArrayWrap verifies Go arrays are usable from scripts: they are copied
// into a slice at conversion time, so append/assignment work on the copy
// instead of panicking.
func TestArrayWrap(t *testing.T) {
	arr := [3]int{1, 2, 3}
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"a":      arr,
	}
	code := []byte(`
assert.Eq(len(a), 3)
a.append(4)
assert.Eq(len(a), 4)
assert.Eq(a[3], 4)
a[0] = 10
assert.Eq(a[0], 10)
`)
	if _, err := starlight.Eval(code, globals, nil); err != nil {
		t.Fatal(err)
	}
	if arr != [3]int{1, 2, 3} {
		t.Fatalf("expected host array to be unchanged (copy semantics), got %v", arr)
	}
	// the explicit constructor follows the same copy rule
	g := convert.NewGoSlice([2]string{"a", "b"})
	if g.Len() != 2 {
		t.Fatalf("expected wrapped array len 2, got %d", g.Len())
	}
}

// TestUnsupportedElemType verifies collections whose static element (or
// key) type cannot be converted are rejected at conversion time with an
// error, instead of converting fine and panicking later in items() or
// iteration.
func TestUnsupportedElemType(t *testing.T) {
	for _, v := range []interface{}{
		map[string]chan int{"c": make(chan int)},
		[]chan int{make(chan int)},
		map[chan int]string{},
		map[string][]chan int{},
		[][]chan int{},
		[]complex128{1i},
	} {
		if _, err := convert.ToValue(v); err == nil {
			t.Errorf("expected conversion error for %T, got nil", v)
		}
	}

	// recursive types must not hang the type pre-check
	type recMap map[string]interface{}
	rm := recMap{}
	rm["self"] = rm
	if _, err := convert.ToValue(rm); err != nil {
		t.Errorf("expected recursive-but-supported type to convert, got %v", err)
	}

	// script-level: the error must surface as a regular error, not a panic
	globals := map[string]interface{}{
		"m": map[string]chan int{"c": make(chan int)},
	}
	_, err := starlight.Eval([]byte(`x = len(m)`), globals, nil)
	if err == nil {
		t.Fatal("expected conversion error for chan-valued map global")
	}
}

// recursiveMapType is a genuinely recursive Go TYPE definition (its element
// type is itself), unlike recMap above whose element type is interface{}.
// It exercises the visited-set guard in checkCollectionElemTypes: without
// it, the type pre-check would recurse forever on the type graph.
type recursiveMapType map[string]recursiveMapType

// recursiveSliceType is the slice analogue (element type is itself).
type recursiveSliceType []recursiveSliceType

// TestRecursiveTypeDefinition verifies the type pre-check terminates on
// recursive type definitions (the documented purpose of the visited set).
// A timeout/hang here is the failure mode being guarded against.
func TestRecursiveTypeDefinition(t *testing.T) {
	rm := recursiveMapType{"a": recursiveMapType{}}
	if _, err := convert.ToValue(rm); err != nil {
		t.Fatalf("recursive map type should convert, got %v", err)
	}
	rs := recursiveSliceType{recursiveSliceType{}}
	if _, err := convert.ToValue(rs); err != nil {
		t.Fatalf("recursive slice type should convert, got %v", err)
	}
	// a recursive type that bottoms out in an unsupported element still
	// terminates and reports the error rather than hanging
	type badRec map[string]chan int
	if _, err := convert.ToValue(badRec{}); err == nil {
		t.Fatal("expected error for recursive-shaped type with chan element")
	}
}

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
