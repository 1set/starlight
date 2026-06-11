package convert

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"sync"
)

// DoNotCompare is an embedded zero-sized struct used to disallow comparison operations (== and !=) on the containing struct.
type DoNotCompare [0]func()

var (
	emptyStr       string
	errType        = reflect.TypeOf((*error)(nil)).Elem()
	emptyIfaceType = reflect.TypeOf((*interface{})(nil)).Elem()
	byteType       = reflect.TypeOf(byte(0))
	rd             = newRecursionDetector()
)

// sortedMapKeys returns the keys of the given map value in a deterministic
// order: keys are sorted by type rank (nil < bool < int < uint < float <
// string < other), then by value within the same rank; "other" keys compare
// by their printed form. reflect's MapKeys returns keys in Go's randomized
// map iteration order, which would otherwise leak into Starlark everywhere
// a wrapped Go map is materialized (keys/items/values/iteration/popitem and
// MakeDict) and make script output non-reproducible across runs.
//
// The sort key for every map key is extracted exactly once up front
// (decorate-sort-undecorate), so the comparison function stays free of
// reflection.
func sortedMapKeys(m reflect.Value) []reflect.Value {
	keys := m.MapKeys()
	decorated := make(sortableKeys, len(keys))
	for i, k := range keys {
		decorated[i] = decorateKey(k)
	}
	sort.Sort(decorated)
	for i := range decorated {
		keys[i] = decorated[i].orig
	}
	return keys
}

// sortableKeys implements sort.Interface with concrete methods (the
// reflect-based swapping of sort.Slice* is measurably slower here).
type sortableKeys []sortableKey

func (s sortableKeys) Len() int           { return len(s) }
func (s sortableKeys) Less(i, j int) bool { return sortableKeyLess(s[i], s[j]) }
func (s sortableKeys) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// sortableKey carries a map key's type rank and its primitive sort key,
// extracted once so sorting does not call into reflect per comparison.
type sortableKey struct {
	rank int
	i    int64
	u    uint64
	f    float64
	s    string
	orig reflect.Value
}

// decorateKey classifies a map key for sorting: it unwraps interface keys
// to their dynamic value and extracts the primitive used for same-rank
// comparison.
func decorateKey(v reflect.Value) sortableKey {
	k := sortableKey{orig: v}
	u := v
	if u.Kind() == reflect.Interface {
		if u.IsNil() {
			return k // rank 0: nil sorts first
		}
		u = u.Elem()
	}
	switch u.Kind() {
	case reflect.Bool:
		k.rank = 1
		if u.Bool() {
			k.i = 1
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		k.rank = 2
		k.i = u.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		k.rank = 3
		k.u = u.Uint()
	case reflect.Float32, reflect.Float64:
		k.rank = 4
		k.f = u.Float()
	case reflect.String:
		k.rank = 5
		k.s = u.String()
	default:
		k.rank = 6
		k.s = fmt.Sprint(u.Interface())
	}
	return k
}

// sortableKeyLess is the strict weak ordering used by sortedMapKeys. Keys
// of different concrete types can tie on rank and value (e.g. int8(5) and
// int64(5) in an interface-keyed map); the tie is broken by the concrete
// type name, lazily, so the order stays fully deterministic without paying
// for type-name extraction on every key.
func sortableKeyLess(a, b sortableKey) bool {
	if a.rank != b.rank {
		return a.rank < b.rank
	}
	switch a.rank {
	case 1, 2:
		if a.i != b.i {
			return a.i < b.i
		}
	case 4:
		// NaN sorts before all other floats so the order stays total
		an, bn := math.IsNaN(a.f), math.IsNaN(b.f)
		if an || bn {
			return an && !bn
		}
		if a.f != b.f {
			return a.f < b.f
		}
	case 3:
		if a.u != b.u {
			return a.u < b.u
		}
	case 5, 6:
		if a.s != b.s {
			return a.s < b.s
		}
	default:
		return false
	}
	return keyTypeName(a.orig) < keyTypeName(b.orig)
}

// keyTypeName names the concrete type of a map key for tie-breaking.
func keyTypeName(v reflect.Value) string {
	if v.Kind() == reflect.Interface && !v.IsNil() {
		v = v.Elem()
	}
	return v.Type().String()
}

// maxSafeStringDepth bounds the pre-scan in safeGoString; values nested
// deeper than this are treated as unsafe to print, since fmt.Sprint would
// recurse at least as deep on them.
const maxSafeStringDepth = 100

// safeGoString formats a wrapped Go value like fmt.Sprint, but first scans
// it for reference cycles: fmt.Sprint recurses forever on a map or slice
// that reaches itself, killing the process with an unrecoverable fatal
// stack overflow. Values containing a cycle (or nested deeper than
// maxSafeStringDepth) format as "<cyclic TYPE>" instead; all other values
// format exactly as fmt.Sprint does. The scan is skipped entirely for
// types that cannot hold a cycle (see typeCanCycle).
func safeGoString(v reflect.Value) string {
	if !v.IsValid() {
		return "<invalid>"
	}
	if typeCanCycle(v.Type()) && hasRefCycle(v, make(map[uintptr]bool), 0) {
		return fmt.Sprintf("<cyclic %s>", v.Type())
	}
	return fmt.Sprint(v.Interface())
}

// typeCanCycleCache memoizes typeCanCycle per reflect.Type; the set of
// types a process converts is small and fixed, so this never grows
// unboundedly.
var typeCanCycleCache sync.Map // reflect.Type -> bool

// typeCanCycle reports whether a value of type t can possibly contain a
// reference cycle: that requires reaching an interface kind (which can hold
// anything, including the value itself) or a recursive type definition
// (e.g. type M map[string]M). For statically acyclic types like
// map[string]int the cycle pre-scan in safeGoString is pure overhead and
// is skipped.
func typeCanCycle(t reflect.Type) bool {
	if c, ok := typeCanCycleCache.Load(t); ok {
		return c.(bool)
	}
	c := typeCanCycleWalk(t, nil)
	typeCanCycleCache.Store(t, c)
	return c
}

// typeCanCycleWalk is the uncached recursion behind typeCanCycle; onPath
// tracks the types on the current descent so recursive type definitions
// are detected (pass nil to start).
func typeCanCycleWalk(t reflect.Type, onPath map[reflect.Type]bool) bool {
	if onPath[t] {
		return true // recursive type definition
	}
	switch t.Kind() {
	case reflect.Interface:
		return true
	case reflect.Map, reflect.Slice, reflect.Array, reflect.Ptr, reflect.Struct:
		if onPath == nil {
			onPath = make(map[reflect.Type]bool)
		}
		onPath[t] = true
		defer delete(onPath, t)
		switch t.Kind() {
		case reflect.Map:
			return typeCanCycleWalk(t.Key(), onPath) || typeCanCycleWalk(t.Elem(), onPath)
		case reflect.Slice, reflect.Array, reflect.Ptr:
			return typeCanCycleWalk(t.Elem(), onPath)
		case reflect.Struct:
			for i := 0; i < t.NumField(); i++ {
				if typeCanCycleWalk(t.Field(i).Type, onPath) {
					return true
				}
			}
		}
	}
	return false
}

// hasRefCycle reports whether v reaches itself through maps, slices,
// pointers, or interfaces, or nests deeper than maxSafeStringDepth. The
// visited set tracks pointers on the current DFS path only, so shared
// (but acyclic) substructures are not misreported.
func hasRefCycle(v reflect.Value, visited map[uintptr]bool, depth int) bool {
	if depth > maxSafeStringDepth {
		return true
	}
	switch v.Kind() {
	case reflect.Map, reflect.Slice, reflect.Ptr:
		if v.IsNil() {
			return false
		}
		p := v.Pointer()
		if visited[p] {
			return true
		}
		visited[p] = true
		defer delete(visited, p)
	}
	switch v.Kind() {
	case reflect.Map:
		for _, k := range v.MapKeys() {
			if hasRefCycle(k, visited, depth+1) || hasRefCycle(v.MapIndex(k), visited, depth+1) {
				return true
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if hasRefCycle(v.Index(i), visited, depth+1) {
				return true
			}
		}
	case reflect.Ptr, reflect.Interface:
		if !v.IsNil() {
			return hasRefCycle(v.Elem(), visited, depth+1)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if hasRefCycle(v.Field(i), visited, depth+1) {
				return true
			}
		}
	}
	return false
}

func newRecursionDetector() *recursionDetector {
	return &recursionDetector{visited: make(map[uintptr]struct{})}
}

// recursionDetector is used to detect infinite recursion in the data structure being converted, usually for starlark.Dict and starlark.List.
// Only pointers are checked, other types will cause panic.
type recursionDetector struct {
	sync.RWMutex
	visited map[uintptr]struct{}
}

func (r *recursionDetector) addr(v interface{}) uintptr {
	// v is a uintptr, so we can't use reflect.ValueOf(v).Pointer()
	if v == nil {
		return 0
	} else if p, ok := v.(uintptr); ok {
		return p
	}
	return reflect.ValueOf(v).Pointer()
}

func (r *recursionDetector) hasVisited(v interface{}) bool {
	r.RLock()
	defer r.RUnlock()
	_, ok := r.visited[r.addr(v)]
	return ok
}

func (r *recursionDetector) setVisited(v interface{}) {
	r.Lock()
	defer r.Unlock()
	r.visited[r.addr(v)] = struct{}{}
}

func (r *recursionDetector) clearVisited(v interface{}) {
	r.Lock()
	defer r.Unlock()
	delete(r.visited, r.addr(v))
}
