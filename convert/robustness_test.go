package convert_test

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
	"go.starlark.net/starlark"
)

// Host robustness: a script or host call must never panic the process,
// and the bridge contracts hold.
//
// Sections:
//   1. Script-reachable panics (strided slice, nil map, arrays, elem types,
//      func pointers, unsupported dynamic values)
//   2. Self-referential String() (no fatal stack overflow)
//   3. Bridge contracts (kwargs, error convention, MakeDict key, pointer
//      methods, dir())
//   4. Freeze semantics, conversion concurrency, and reference cycles

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

// TestConstructorPrechecksElemTypes verifies the public NewGoMap/NewGoSlice
// constructors run the same static element-type check ToValue runs, so an
// unsupported element type panics at construction instead of building a
// wrapper that panics later inside Items/Keys/Index/iteration — methods that
// cannot return an error. This closes the gap where the safe ToValue path
// rejected such collections but the direct constructors did not (invariant:
// methods that can't return errors must never reach panic).
func TestConstructorPrechecksElemTypes(t *testing.T) {
	for _, m := range []interface{}{
		map[string]chan int{"c": make(chan int)},
		map[complex128]string{},
		map[string][]chan int{},
	} {
		assertConstructPanics(t, fmt.Sprintf("NewGoMap(%T)", m), func() { convert.NewGoMap(m) })
	}
	for _, s := range []interface{}{
		[]chan int{make(chan int)},
		[]complex128{1i},
		[2]chan int{},
	} {
		assertConstructPanics(t, fmt.Sprintf("NewGoSlice(%T)", s), func() { convert.NewGoSlice(s) })
	}

	// Supported element types (including interface{}, checked dynamically)
	// must still construct without panicking.
	assertNoConstructPanic(t, "NewGoMap supported", func() {
		_ = convert.NewGoMap(map[string]interface{}{"a": 1})
	})
	assertNoConstructPanic(t, "NewGoSlice supported", func() {
		_ = convert.NewGoSlice([]interface{}{1, "x"})
	})
}

func assertConstructPanics(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("%s: expected a panic for an unsupported element type, got none", what)
		}
	}()
	fn()
}

func assertNoConstructPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s: unexpected panic: %v", what, r)
		}
	}()
	fn()
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

// Regression tests for self-referential Go values: String() used fmt.Sprint
// with no cycle detection, so a Go map or slice that reaches itself made
// String() recurse forever and killed the process with an unrecoverable
// fatal stack overflow. Cyclic values must format as "<cyclic TYPE>"; plain
// values must keep their exact previous formatting.

// TestGoMapSelfRefString verifies a self-referential map formats safely.
func TestGoMapSelfRefString(t *testing.T) {
	m := map[string]interface{}{}
	m["self"] = m
	got := convert.NewGoMap(m).String()
	if !strings.Contains(got, "cyclic") {
		t.Fatalf("expected cyclic marker, got %q", got)
	}
}

// TestGoSliceSelfRefString verifies a self-referential slice formats safely.
func TestGoSliceSelfRefString(t *testing.T) {
	s := make([]interface{}, 1)
	s[0] = s
	v, err := convert.ToValue(s)
	if err != nil {
		t.Fatal(err)
	}
	got := v.String()
	if !strings.Contains(got, "cyclic") {
		t.Fatalf("expected cyclic marker, got %q", got)
	}
}

// TestGoStructSelfRefString verifies a struct whose field holds a
// self-referential map formats safely.
func TestGoStructSelfRefString(t *testing.T) {
	type node struct {
		M map[string]interface{}
	}
	n := node{M: map[string]interface{}{}}
	n.M["self"] = n.M
	v, err := convert.ToValue(n)
	if err != nil {
		t.Fatal(err)
	}
	got := v.String()
	if !strings.Contains(got, "cyclic") {
		t.Fatalf("expected cyclic marker, got %q", got)
	}
}

// TestScriptStrSelfRef verifies str() in a script cannot crash the host on
// self-referential values. m["self"] unwraps to the GoMap itself (empty
// interfaces are unwrapped to their dynamic value), so both str(m) and
// str(m["self"]) format the cyclic map safely.
func TestScriptStrSelfRef(t *testing.T) {
	m := map[string]interface{}{}
	m["self"] = m
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"m":      m,
	}
	code := []byte(`
s1 = str(m)
s2 = str(m["self"])
assert.Eq("cyclic" in s1, True)
assert.Eq("cyclic" in s2, True)
`)
	if _, err := starlight.Eval(code, globals, nil); err != nil {
		t.Fatal(err)
	}
}

// TestStringFormattingUnchanged pins the exact previous formatting for
// ordinary (acyclic) values.
func TestStringFormattingUnchanged(t *testing.T) {
	if got := convert.NewGoMap(map[string]int{"a": 1}).String(); got != "map[a:1]" {
		t.Fatalf("map formatting changed: %q", got)
	}
	sv, err := convert.ToValue([]int{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if got := sv.String(); got != "[1 2 3]" {
		t.Fatalf("slice formatting changed: %q", got)
	}
	type pair struct {
		A int
		B string
	}
	pv, err := convert.ToValue(pair{A: 1, B: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	if got := pv.String(); got != "{1 bob}" {
		t.Fatalf("struct formatting changed: %q", got)
	}
}

// TestStringDeepNesting verifies that absurdly deep (but finite) values are
// elided rather than allowed to recurse toward a stack overflow.
func TestStringDeepNesting(t *testing.T) {
	var nest interface{} = []interface{}{}
	for i := 0; i < 500; i++ {
		nest = []interface{}{nest}
	}
	v, err := convert.ToValue(nest)
	if err != nil {
		t.Fatal(err)
	}
	got := v.String()
	if !strings.Contains(got, "cyclic") {
		t.Fatalf("expected deep value to be elided, got %d chars", len(got))
	}
}

// Regression tests for five broken bridge contracts:
//  1. wrapped Go functions silently swallowed keyword arguments, so
//     fn(bogus=1) succeeded as if the kwarg had been applied;
//  2. the error-return convention only matched the exact `error` interface
//     type, so functions returning a concrete error type leaked it as a
//     value instead of raising;
//  3. MakeDict ignored SetKey errors, silently dropping entries whose key
//     is unhashable on the Starlark side;
//  4. pointer-receiver methods were invisible on addressable struct values
//     (e.g. fields of a wrapped *struct);
//  5. GoInterface's dir() omitted the synthetic to* conversion attributes
//     that Attr actually supports, and double-listed methods for pointers.

// TestKwargsRejected verifies wrapped Go functions reject keyword
// arguments instead of silently dropping them.
func TestKwargsRejected(t *testing.T) {
	globals := map[string]interface{}{
		"quote":  func(s string) string { return `"` + s + `"` },
		"concat": func(ss ...string) string { return strings.Join(ss, "") },
	}
	for _, code := range []string{`v = quote("a", bogus=1)`, `v = concat("a", "b", bogus=1)`} {
		_, err := starlight.Eval([]byte(code), globals, nil)
		if err == nil || !strings.Contains(err.Error(), "unexpected keyword argument") {
			t.Fatalf("%s: expected unexpected-keyword error, got %v", code, err)
		}
	}
	// no kwargs still works
	res, err := starlight.Eval([]byte(`v = quote("a")`), globals, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res["v"] != `"a"` {
		t.Fatalf("expected quoted a, got %v", res["v"])
	}
}

type customErr struct {
	msg string
}

func (e *customErr) Error() string { return e.msg }

// TestConcreteErrorReturn verifies functions returning a concrete error
// type raise in Starlark like `error`-typed returns do, and that a typed
// nil error does not raise.
func TestConcreteErrorReturn(t *testing.T) {
	globals := map[string]interface{}{
		"fail":   func() *customErr { return &customErr{msg: "boom"} },
		"calc":   func(ok bool) (string, *customErr) { return "fine", nil },
		"calc2":  func() (string, *customErr) { return "", &customErr{msg: "nope"} },
		"legacy": func() (string, error) { return "v", nil },
	}
	_, err := starlight.Eval([]byte(`fail()`), globals, nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected concrete error to raise, got %v", err)
	}
	res, err := starlight.Eval([]byte(`v = calc(True)`), globals, nil)
	if err != nil {
		t.Fatalf("typed nil error must not raise: %v", err)
	}
	if res["v"] != "fine" {
		t.Fatalf("expected fine, got %v", res["v"])
	}
	_, err = starlight.Eval([]byte(`v = calc2()`), globals, nil)
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("expected concrete error to raise, got %v", err)
	}
	if res, err = starlight.Eval([]byte(`v = legacy()`), globals, nil); err != nil || res["v"] != "v" {
		t.Fatalf("plain error convention broken: %v %v", res, err)
	}
}

// TestMakeDictKeyError verifies MakeDict reports keys that are unhashable
// on the Starlark side instead of silently dropping the entries.
func TestMakeDictKeyError(t *testing.T) {
	// [2]string is a comparable Go map key, but converts to a Starlark
	// value (a slice wrapper) that is not hashable
	m := map[[2]string]int{{"a", "b"}: 1}
	if _, err := convert.MakeDict(m); err == nil {
		t.Fatal("expected unhashable-key error from MakeDict, got nil")
	}
	// plain dicts still convert
	v, err := convert.MakeDict(map[string]int{"a": 1})
	if err != nil {
		t.Fatal(err)
	}
	if v.(*starlark.Dict).Len() != 1 {
		t.Fatalf("expected 1 entry, got %v", v)
	}
}

type innerCounter struct {
	N int
}

func (c *innerCounter) Incr() { c.N++ }
func (c innerCounter) Get() int {
	return c.N
}

type outerHolder struct {
	Inner innerCounter
}

// TestPointerMethodsOnAddressableValue verifies pointer-receiver methods
// are reachable on addressable struct values, e.g. fields of a wrapped
// pointer — and that their mutations are visible to the host.
func TestPointerMethodsOnAddressableValue(t *testing.T) {
	o := &outerHolder{}
	globals := map[string]interface{}{"o": o}
	code := []byte(`
o.Inner.Incr()
o.Inner.Incr()
v = o.Inner.Get()
`)
	res, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}
	if o.Inner.N != 2 {
		t.Fatalf("expected mutations visible on host (N=2), got %+v", o)
	}
	if res["v"] != int64(2) {
		t.Fatalf("expected v=2, got %v", res["v"])
	}
}

type loudInt int

func (l loudInt) Doubled() int { return int(l) * 2 }

// TestGoInterfaceDirHonest verifies dir() on a GoInterface lists exactly
// what Attr supports: real methods plus the synthetic to* conversions,
// without duplicates.
func TestGoInterfaceDirHonest(t *testing.T) {
	v, err := convert.ToValue(loudInt(21))
	if err != nil {
		t.Fatal(err)
	}
	gi, ok := v.(*convert.GoInterface)
	if !ok {
		t.Fatalf("expected GoInterface, got %T", v)
	}
	names := gi.AttrNames()
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Fatalf("duplicate attr name %q in %v", n, names)
		}
		seen[n] = true
	}
	for _, want := range []string{"Doubled", "toInt", "toString", "toFloat", "toUint", "toBool"} {
		if !seen[want] {
			t.Fatalf("expected %q in AttrNames, got %v", want, names)
		}
	}
	if !sort.StringsAreSorted(names) {
		t.Fatalf("expected sorted attr names, got %v", names)
	}
	// every listed name must actually resolve through Attr
	for _, n := range names {
		av, err := gi.Attr(n)
		if err != nil || av == nil {
			t.Fatalf("AttrNames lists %q but Attr returns (%v, %v)", n, av, err)
		}
	}
	// and the synthetic conversion works from a script
	globals := map[string]interface{}{"x": v, "assert": nil}
	delete(globals, "assert")
	res, err := starlight.Eval([]byte(`v = x.Doubled()
i = x.toInt()`), globals, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res["v"] != int64(42) || res["i"] != int64(21) {
		t.Fatalf("expected 42/21, got %v", res)
	}
	_ = fmt.Sprint // silence unused import on some build tags
}

// Regression tests for freeze semantics and conversion concurrency:
//   - GoStruct.Freeze was an empty function, so scripts kept mutating host
//     structs through frozen values;
//   - the package-level recursion detector was shared across goroutines by
//     pointer, so concurrent conversions of the same list/dict spuriously
//     saw "already visited" and silently returned nil (data loss), and the
//     iteration counters raced.

type frozenTarget struct {
	Name string
	Num  int
}

// TestFreezeGoStruct verifies that a frozen GoStruct rejects writes through
// both of its write paths (attribute assignment and index assignment).
func TestFreezeGoStruct(t *testing.T) {
	s := &frozenTarget{Name: "a", Num: 1}
	v, err := convert.ToValue(s)
	if err != nil {
		t.Fatal(err)
	}
	v.Freeze()
	globals := map[string]interface{}{"s": v}

	for _, code := range []string{`s.Name = "b"`, `s["Num"] = 2`} {
		_, err = starlight.Eval([]byte(code), globals, nil)
		if err == nil || !strings.Contains(err.Error(), "frozen") {
			t.Fatalf("%s: expected frozen error, got %v", code, err)
		}
	}
	if s.Name != "a" || s.Num != 1 {
		t.Fatalf("expected struct unchanged, got %+v", s)
	}

	// unfrozen wrappers still accept writes
	s2 := &frozenTarget{Name: "a", Num: 1}
	globals2 := map[string]interface{}{"s": s2}
	if _, err := starlight.Eval([]byte(`s.Name = "b"`), globals2, nil); err != nil {
		t.Fatal(err)
	}
	if s2.Name != "b" {
		t.Fatalf("expected write to unfrozen struct to work, got %+v", s2)
	}
}

// TestConcurrentFromValue verifies that concurrent conversions of the same
// Starlark list/dict are complete and race-free: with the shared
// package-level recursion detector, goroutines spuriously saw each other's
// in-progress markers and silently got nil back.
func TestConcurrentFromValue(t *testing.T) {
	l := starlark.NewList(nil)
	for i := 0; i < 10; i++ {
		if err := l.Append(starlark.MakeInt(i)); err != nil {
			t.Fatal(err)
		}
	}
	d := starlark.NewDict(10)
	for i := 0; i < 10; i++ {
		if err := d.SetKey(starlark.String(fmt.Sprintf("k%d", i)), starlark.MakeInt(i)); err != nil {
			t.Fatal(err)
		}
	}
	// per the Starlark contract, values shared across threads must be
	// frozen first (unfrozen values race on L0's own iteration counters)
	l.Freeze()
	d.Freeze()

	const goroutines = 8
	const rounds = 20
	var wg sync.WaitGroup
	errs := make(chan error, goroutines*rounds*2)
	for gi := 0; gi < goroutines; gi++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				if got := convert.FromList(l); len(got) != 10 {
					errs <- fmt.Errorf("FromList returned %d elements, want 10", len(got))
					return
				}
				if got := convert.FromDict(d); len(got) != 10 {
					errs <- fmt.Errorf("FromDict returned %d entries, want 10", len(got))
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// TestConcurrentIterate verifies concurrent read-only iteration over the
// same wrapped Go map and slice is race-free (the iteration counters used
// to be plain ints).
func TestConcurrentIterate(t *testing.T) {
	gm := convert.NewGoMap(map[string]int{"a": 1, "b": 2, "c": 3})
	gs := convert.NewGoSlice([]int{1, 2, 3})

	const goroutines = 8
	var wg sync.WaitGroup
	for gi := 0; gi < goroutines; gi++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 0; r < 50; r++ {
				var v starlark.Value
				it := gm.Iterate()
				for it.Next(&v) {
				}
				it.Done()
				it = gs.Iterate()
				for it.Next(&v) {
				}
				it.Done()
			}
		}()
	}
	wg.Wait()
}

// TestFromListCycle pins the documented cycle behavior: a list reaching
// itself converts with nil in place of the cyclic reference, instead of
// recursing forever.
func TestFromListCycle(t *testing.T) {
	l := starlark.NewList(nil)
	if err := l.Append(starlark.MakeInt(1)); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(l); err != nil {
		t.Fatal(err)
	}
	got := convert.FromList(l)
	if len(got) != 2 {
		t.Fatalf("expected 2 elements, got %#v", got)
	}
	if got[0] != int64(1) {
		t.Fatalf("expected first element 1, got %#v", got[0])
	}
	if s, ok := got[1].([]interface{}); !ok || s != nil {
		t.Fatalf("expected cyclic reference to convert to a nil slice, got %#v", got[1])
	}
}

// TestFromDictCycle verifies a self-referential dict converts its cyclic
// reference to nil instead of recursing forever. The package-level
// recursion detector this used to rely on was replaced (#43) by a visited
// set threaded through fromValue/fromDict; this and TestFromListCycle /
// TestCrossCollectionCycle are the black-box coverage for that protection
// (the old white-box recursionDetector unit test was removed with it).
func TestFromDictCycle(t *testing.T) {
	d := starlark.NewDict(1)
	if err := d.SetKey(starlark.String("self"), d); err != nil {
		t.Fatal(err)
	}
	if err := d.SetKey(starlark.String("v"), starlark.MakeInt(1)); err != nil {
		t.Fatal(err)
	}
	got := convert.FromDict(d)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %#v", got)
	}
	if got["v"] != int64(1) {
		t.Fatalf("expected v=1, got %#v", got["v"])
	}
	if m, ok := got["self"].(map[interface{}]interface{}); !ok || m != nil {
		t.Fatalf("expected self-reference to convert to a nil map, got %#v", got["self"])
	}
}

// TestCrossCollectionCycle verifies a cycle spanning multiple collection
// kinds (list -> dict -> back to list) is broken at the revisit, not
// followed into a stack overflow.
func TestCrossCollectionCycle(t *testing.T) {
	l := starlark.NewList(nil)
	d := starlark.NewDict(1)
	if err := d.SetKey(starlark.String("back"), l); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(starlark.MakeInt(1)); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(d); err != nil {
		t.Fatal(err)
	}
	got := convert.FromList(l)
	if len(got) != 2 || got[0] != int64(1) {
		t.Fatalf("expected [1, {back:nil}], got %#v", got)
	}
	inner, ok := got[1].(map[interface{}]interface{})
	if !ok {
		t.Fatalf("expected inner dict, got %#v", got[1])
	}
	// the back-reference to the outer list is broken (nil), no stack overflow
	if s, ok := inner["back"].([]interface{}); !ok || s != nil {
		t.Fatalf("expected back-reference to convert to nil slice, got %#v", inner["back"])
	}
}

// TestDeepNestingNoStackOverflow verifies very deep (but acyclic) nesting
// converts without overflowing the stack — the visited set keys on the
// pointers actually on the current path, so depth alone is fine.
func TestDeepNestingNoStackOverflow(t *testing.T) {
	const depth = 2000
	cur := starlark.NewList(nil)
	root := cur
	for i := 0; i < depth; i++ {
		next := starlark.NewList(nil)
		if err := cur.Append(next); err != nil {
			t.Fatal(err)
		}
		cur = next
	}
	got := convert.FromList(root)
	// walk down to confirm it materialized fully
	n := 0
	for len(got) == 1 {
		nxt, ok := got[0].([]interface{})
		if !ok {
			break
		}
		got = nxt
		n++
	}
	if n != depth {
		t.Fatalf("expected to walk %d levels, got %d", depth, n)
	}
}
