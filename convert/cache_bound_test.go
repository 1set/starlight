package convert

import (
	"fmt"
	"reflect"
	"testing"

	"go.starlark.net/starlark"
)

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
