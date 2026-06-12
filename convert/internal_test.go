package convert

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

// Consolidated white-box (package convert) tests for internal helpers:
// the bounded type-check cache and the conversion-boundary panic sentinel.

// TestTypeCacheBounded verifies the type-check caches cannot grow without
// bound. hashableGoValue mints a fresh reflect.ArrayOf(N, ...) type per
// tuple/bytes key length; a script controls N, so storing every such type
// forever is a script-reachable OOM in a sandboxing library. The cache
// must stop growing past its cap while still returning correct results.
func TestTypeCacheBounded(t *testing.T) {
	c := newBoundedTypeCache(8)
	for i := 0; i < 1000; i++ {
		at := reflect.ArrayOf(i+1, emptyIfaceType)
		c.loadOrStore(at, checkCollectionElemTypes(at, nil))
	}
	if got := c.size(); got > 8 {
		t.Fatalf("cache exceeded cap: size=%d, cap=8", got)
	}
	// correctness is independent of caching: a value past the cap still
	// computes the right answer
	at := reflect.ArrayOf(5000, emptyIfaceType)
	if err := c.loadOrStore(at, checkCollectionElemTypes(at, nil)); err != nil {
		t.Fatalf("expected nil error for [N]interface{}, got %v", err)
	}
	bad := reflect.TypeOf(map[string]chan int(nil))
	if err := c.loadOrStore(bad, checkCollectionElemTypes(bad, nil)); err == nil {
		t.Fatal("expected error for map[string]chan int")
	}
}

// TestTupleKeysDoNotLeakCache drives the end-to-end script vector: inserting
// tuple keys of growing length into a wrapped map[interface{}]V and
// materializing them must not pin an unbounded number of array types.
func TestTupleKeysDoNotLeakCache(t *testing.T) {
	before := elemTypeCheckCache.size()
	for n := 1; n <= 600; n++ {
		m := map[interface{}]interface{}{}
		g := NewGoMap(m)
		key := make(starlark.Tuple, n)
		for i := range key {
			key[i] = starlark.MakeInt(i)
		}
		if err := g.SetKey(key, starlark.MakeInt(n)); err != nil {
			t.Fatal(err)
		}
		// materialize the key back to Starlark (the path that caches its type)
		_ = g.Keys()
		_ = g.Items()
	}
	after := elemTypeCheckCache.size()
	if grew := after - before; grew > elemTypeCacheCap {
		t.Fatalf("cache grew by %d (> cap %d) from script-controlled tuple lengths", grew, elemTypeCacheCap)
	}
}

// TestBoundedCacheReturnsCachedAndFresh verifies the cache returns a cached
// value on hit and computes on miss (the optimization still works).
func TestBoundedCacheReturnsCachedAndFresh(t *testing.T) {
	c := newBoundedTypeCache(64)
	mt := reflect.TypeOf(map[string]int(nil))
	if err := c.loadOrStore(mt, checkCollectionElemTypes(mt, nil)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// second call hits the cache and must agree
	if err := c.loadOrStore(mt, fmt.Errorf("should-not-be-used")); err != nil {
		t.Fatalf("cached hit should return the stored nil error, got %v", err)
	}
}

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

// TestComparableByValue covers every branch of the map-key value-comparability
// check: scalars and value-only composites are value-comparable; pointers,
// channels, unsafe.Pointers, interfaces, and any array/struct transitively
// containing them are not.
func TestComparableByValue(t *testing.T) {
	type valStruct struct {
		A int
		B string
	}
	type ptrStruct struct {
		A int
		P *int
	}
	type ifaceStruct struct {
		V interface{}
	}
	cases := []struct {
		t    reflect.Type
		want bool
	}{
		{reflect.TypeOf(0), true},               // scalar -> default true
		{reflect.TypeOf(""), true},              // scalar
		{reflect.TypeOf(1.5), true},             // scalar
		{reflect.TypeOf((*int)(nil)), false},    // ptr
		{reflect.TypeOf(make(chan int)), false}, // chan
		{reflect.TypeOf([2]int{}), true},        // array of scalar
		{reflect.TypeOf([2]*int{}), false},      // array of ptr
		{reflect.TypeOf(valStruct{}), true},     // struct, all value
		{reflect.TypeOf(ptrStruct{}), false},    // struct with ptr field
		{reflect.TypeOf(ifaceStruct{}), false},  // struct with interface field
		{reflect.TypeOf([1][2]int{}), true},     // nested array of scalar
		{reflect.TypeOf([1]valStruct{}), true},  // array of value struct
		{reflect.TypeOf([1]ptrStruct{}), false}, // array of ptr-bearing struct
	}
	for _, c := range cases {
		if got := comparableByValue(c.t); got != c.want {
			t.Errorf("comparableByValue(%s) = %v, want %v", c.t, got, c.want)
		}
	}
}
