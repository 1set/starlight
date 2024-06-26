package convert

import (
	"reflect"
	"sync"
)

// DoNotCompare is an embedded zero-sized struct used to disallow comparison operations (== and !=) on the containing struct.
type DoNotCompare [0]func()

var (
	emptyStr string
	errType  = reflect.TypeOf((*error)(nil)).Elem()
	rd       = newRecursionDetector()
)

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
