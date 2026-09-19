package convert

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"go.starlark.net/starlark"
)

// Consolidated white-box (package convert) tests for internal helpers:
// the bounded type-check cache (including concurrent capacity), key rendering,
// interface truth, and the conversion-boundary panic sentinel.

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

// TestStableKeyString covers every kind branch of the address-free key
// renderer and asserts equal values render equally (no addresses leak).
func TestStableKeyString(t *testing.T) {
	type inner struct {
		B bool
		F float64
	}
	type composite struct {
		N int
		U uint8
		S string
		P *int
		C complex128
		A [2]int
		I inner
	}
	mk := func() composite {
		x := 7
		return composite{N: -3, U: 200, S: "k", P: &x, C: 1 + 2i, A: [2]int{4, 5}, I: inner{B: true, F: 1.5}}
	}
	a, b := mk(), mk() // equal values, different *int addresses
	sa := stableKeyString(reflect.ValueOf(a))
	sb := stableKeyString(reflect.ValueOf(b))
	if sa != sb {
		t.Fatalf("equal values must render equally:\n a=%s\n b=%s", sa, sb)
	}
	for _, want := range []string{"-3", "200", "k", "7", "true", "1.5", "[4 5]"} {
		if !strings.Contains(sa, want) {
			t.Errorf("rendered key %q missing %q", sa, want)
		}
	}
	// nil pointer and channel branches
	if got := stableKeyString(reflect.ValueOf((*int)(nil))); got != "<nil>" {
		t.Errorf("nil ptr -> %q, want <nil>", got)
	}
	if got := stableKeyString(reflect.ValueOf(make(chan int))); got != "<chan>" {
		t.Errorf("chan -> %q, want <chan>", got)
	}
}

// TestStableKeyStringDistinguishes pins the property the deterministic-order
// invariant depends on: two DISTINCT composite keys that can legally coexist
// in one map must render to distinct strings. When they collide on their sort
// string (and share a type), sortableKeyLess ties, the sort leaves them in
// Go's randomized MapKeys order, and the "deterministic order" guarantee
// silently breaks. It covers the collision classes the render closes; it does
// not assert the documented irreducible ties (equal-pointee pointers/channels,
// NaN bit-patterns, types sharing reflect.Type.String).
func TestStableKeyStringDistinguishes(t *testing.T) {
	type ifKey struct{ V interface{} }
	render := func(v interface{}) string { return stableKeyString(reflect.ValueOf(v)) }

	groups := [][]interface{}{
		// string boundaries: {"a","b c"} and {"a b","c"} both rendered
		// "[a b c]" before the length prefix made strings self-delimiting
		{
			[2]string{"a", "b c"},
			[2]string{"a b", "c"},
		},
		// interface dynamic type: an interface field holding int8(1) vs
		// int64(1) vs "1" all rendered the same before the type tag; a nil
		// interface renders "<nil>", distinct from any of them
		{
			ifKey{V: int8(1)},
			ifKey{V: int64(1)},
			ifKey{V: uint8(1)},
			ifKey{V: "1"},
			ifKey{V: float64(1)},
			ifKey{V: nil},
		},
		// nested array of strings inside a struct
		{
			struct{ A [2]string }{A: [2]string{"x", "y z"}},
			struct{ A [2]string }{A: [2]string{"x y", "z"}},
		},
	}
	for gi, g := range groups {
		seen := map[string]int{}
		for vi, v := range g {
			s := render(v)
			if prev, ok := seen[s]; ok {
				t.Errorf("group %d: values %d and %d render the same key %q (collision breaks deterministic order)", gi, prev, vi, s)
			}
			seen[s] = vi
		}
	}

	// no identity leak: two keys with pointers to EQUAL pointees at different
	// addresses must render identically (else the sort key varies run to run).
	// This is the meaningful address-free check (a fixed literal would pass
	// even with the pointer render reverted to fmt.Sprint).
	mkPtrKey := func() interface{} {
		x := 7
		return struct {
			N int
			P *int
		}{N: 1, P: &x}
	}
	if a, b := render(mkPtrKey()), render(mkPtrKey()); a != b {
		t.Errorf("equal-pointee pointer keys render differently (address leaked): %q vs %q", a, b)
	}
}

// TestStableKeyStringCyclicTerminates: a self-referential pointer is a legal
// Go map key (type Node struct{ Next *Node }; n.Next = n). writeStableKey
// followed the chain without bound and overflowed the stack — an
// unrecoverable host crash (invariant: no host crash from input). The depth
// cap must make it terminate; reaching the assertion is the proof.
func TestStableKeyStringCyclicTerminates(t *testing.T) {
	type Node struct{ Next *Node }
	n := &Node{}
	n.Next = n
	// decorateKey unwraps the top pointer, so the rendered value is the
	// pointee struct; this mirrors map[*Node]V materialization.
	if got := stableKeyString(reflect.ValueOf(*n)); got == "" {
		t.Fatal("expected a bounded render, got empty")
	}
}

// TestGoInterfaceTruthNilable: Truth returned a blanket true for kinds it
// didn't enumerate, so a nil chan/map/func/slice wrapped in a GoInterface
// reported truthy. Nilable kinds should reflect their nil-ness.
func TestGoInterfaceTruthNilable(t *testing.T) {
	cases := []struct {
		v    interface{}
		want bool
	}{
		{(chan int)(nil), false},
		{make(chan int), true},
		{map[string]int(nil), false},
		{map[string]int{"a": 1}, true},
		{([]int)(nil), false},
		{[]int{1}, true},
		{(func())(nil), false},
	}
	for _, c := range cases {
		gi := &GoInterface{v: reflect.ValueOf(c.v)}
		if got := bool(gi.Truth()); got != c.want {
			t.Errorf("Truth(%T nil=%v) = %v, want %v", c.v, reflect.ValueOf(c.v).IsNil(), got, c.want)
		}
	}
}

// TestBoundedCacheConcurrentCapacity pins both caches at zero, one, and
// several entries even when distinct types race to fill the last slot.
func TestBoundedCacheConcurrentCapacity(t *testing.T) {
	types := make([]reflect.Type, 128)
	for i := range types {
		types[i] = reflect.ArrayOf(i+1, emptyIfaceType)
	}
	for _, boolean := range []bool{false, true} {
		for _, capacity := range []int{0, 1, 8} {
			t.Run(fmt.Sprintf("bool=%t/cap=%d", boolean, capacity), func(t *testing.T) {
				for round := 0; round < 20; round++ {
					c := newBoundedTypeCache(capacity)
					start := make(chan struct{})
					var wg sync.WaitGroup
					for _, typ := range types {
						wg.Add(1)
						go func(typ reflect.Type) {
							defer wg.Done()
							<-start
							if boolean {
								if !c.loadOrStoreBool(typ, true) {
									t.Error("lost computed bool")
								}
							} else if err := c.loadOrStore(typ, nil); err != nil {
								t.Errorf("lost nil error: %v", err)
							}
						}(typ)
					}
					close(start)
					wg.Wait()
					if n := c.size(); n != capacity {
						t.Fatalf("round %d: size = %d, want %d", round, n, capacity)
					}
				}
			})
		}
	}
}

// TestBoundedCacheConcurrentSameType pins shared-key counting and cached
// values for both value domains, including a full cache and uncached misses.
func TestBoundedCacheConcurrentSameType(t *testing.T) {
	typ, other := reflect.TypeOf(0), reflect.TypeOf("")
	sentinel := errors.New("cached error")
	for _, boolean := range []bool{false, true} {
		t.Run(fmt.Sprintf("bool=%t", boolean), func(t *testing.T) {
			c := newBoundedTypeCache(1)
			var wg sync.WaitGroup
			for i := 0; i < 128; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if boolean {
						if c.loadOrStoreBool(typ, false) {
							t.Error("lost cached false")
						}
					} else if c.loadOrStore(typ, sentinel) != sentinel {
						t.Error("lost cached error")
					}
				}()
			}
			wg.Wait()
			if c.size() != 1 {
				t.Fatalf("shared key counted %d times", c.size())
			}
			if boolean {
				if c.loadOrStoreBool(typ, true) {
					t.Error("replaced cached false")
				}
				if !c.loadOrStoreBool(other, true) {
					t.Error("lost uncached true")
				}
				if c.loadOrStoreBool(other, false) {
					t.Error("stored beyond capacity")
				}
			} else {
				if c.loadOrStore(typ, nil) != sentinel {
					t.Error("replaced cached error")
				}
				if c.loadOrStore(other, nil) != nil {
					t.Error("lost uncached nil")
				}
				if c.loadOrStore(other, sentinel) != sentinel {
					t.Error("stored beyond capacity")
				}
			}
			if c.size() != 1 {
				t.Fatal("cache grew beyond capacity")
			}
		})
	}
}

// TestBoundedCacheStoreRetainsFirst pins duplicate stores before capacity is
// reached; the error and bool caches must preserve even nil and false values.
func TestBoundedCacheStoreRetainsFirst(t *testing.T) {
	typ := reflect.TypeOf(0)
	for _, first := range []interface{}{nil, errors.New("first"), false, true} {
		c := newBoundedTypeCache(2)
		c.store(typ, first)
		c.store(typ, "replacement")
		if c.m[typ] != first || c.size() != 1 {
			t.Fatalf("duplicate store replaced %v or grew the cache", first)
		}
	}
}
