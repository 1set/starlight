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
func sortedMapKeys(m reflect.Value) []reflect.Value {
	keys := m.MapKeys()
	sort.SliceStable(keys, func(i, j int) bool { return keyLess(keys[i], keys[j]) })
	return keys
}

// keyRank classifies a map key for sorting: it unwraps interface keys to
// their dynamic value and returns the type rank along with the unwrapped
// value used for same-rank comparison.
func keyRank(v reflect.Value) (int, reflect.Value) {
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return 0, v
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Bool:
		return 1, v
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return 2, v
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return 3, v
	case reflect.Float32, reflect.Float64:
		return 4, v
	case reflect.String:
		return 5, v
	}
	return 6, v
}

// keyLess is the strict weak ordering used by sortedMapKeys.
func keyLess(a, b reflect.Value) bool {
	ra, va := keyRank(a)
	rb, vb := keyRank(b)
	if ra != rb {
		return ra < rb
	}
	switch ra {
	case 1:
		return !va.Bool() && vb.Bool()
	case 2:
		return va.Int() < vb.Int()
	case 3:
		return va.Uint() < vb.Uint()
	case 4:
		fa, fb := va.Float(), vb.Float()
		// NaN sorts before all other floats so the order stays total
		if math.IsNaN(fa) {
			return !math.IsNaN(fb)
		}
		if math.IsNaN(fb) {
			return false
		}
		return fa < fb
	case 5:
		return va.String() < vb.String()
	case 6:
		return fmt.Sprint(va.Interface()) < fmt.Sprint(vb.Interface())
	}
	return false
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
// format exactly as fmt.Sprint does.
func safeGoString(v reflect.Value) string {
	if !v.IsValid() {
		return "<invalid>"
	}
	if hasRefCycle(v, make(map[uintptr]bool), 0) {
		return fmt.Sprintf("<cyclic %s>", v.Type())
	}
	return fmt.Sprint(v.Interface())
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
