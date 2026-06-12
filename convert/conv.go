// Package convert provides functions for converting data and functions between Go and Starlark.
package convert

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"runtime/debug"
	"time"

	startime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
)

// PanicError reports a panic recovered at the conversion boundary. The
// conversion entry points are not supposed to panic; if this error
// surfaces, it indicates a bug in starlight (or a misbehaving custom
// type), and the captured stack identifies where the panic started.
type PanicError struct {
	Value interface{} // the recovered panic value
	Stack []byte      // the goroutine stack captured at recovery
}

// Error implements the error interface. The message keeps the historic
// "panic recovered" prefix and appends the captured stack.
func (e *PanicError) Error() string {
	return fmt.Sprintf("panic recovered: %v\n%s", e.Value, e.Stack)
}

// ToValue attempts to convert the given value to a starlark.Value.
// It supports all int, uint, and float numeric types, plus strings and booleans.
// It supports structs, maps, slices, and functions that use the aforementioned.
// Any starlark.Value is passed through as-is.
func ToValue(v interface{}) (starlark.Value, error) {
	if val, ok := v.(starlark.Value); ok {
		return val, nil
	}
	return toValue(reflect.ValueOf(v), emptyStr)
}

// ToValueWithTag attempts to convert the given value to a starlark.Value.
// It works like ToValue, but also accepts a tag name to use for all nested struct fields.
func ToValueWithTag(v interface{}, tagName string) (starlark.Value, error) {
	if val, ok := v.(starlark.Value); ok {
		return val, nil
	}
	return toValue(reflect.ValueOf(v), tagName)
}

func hasMethods(val reflect.Value) bool {
	if val.NumMethod() > 0 {
		return true
	}
	if val.Kind() == reflect.Ptr && val.Elem().IsValid() && val.Elem().NumMethod() > 0 {
		return true
	}
	return false
}

func toValue(val reflect.Value, tagName string) (result starlark.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &PanicError{Value: r, Stack: debug.Stack()}
		}
	}()

	if val.IsValid() {
		if _, ok := val.Interface().(starlark.Value); ok {
			// let Starlark values pass through, no conversion needed
			return val.Interface().(starlark.Value), nil
		}
		// time.Duration maps to the standard Starlark time.duration, the
		// mirror of the FromValue case below; without this it would be
		// grabbed by the method check and wrapped as an opaque interface
		if val.Type() == durationType {
			return startime.Duration(val.Interface().(time.Duration)), nil
		}
		if hasMethods(val) {
			// this handles all basic types with methods (numbers, strings, booleans)
			ifc, ok := makeGoInterface(val)
			if ok {
				return ifc, nil
			}
			// TODO: maps, functions, and slices with methods
		}
	}

	kind := val.Kind()
	if kind == reflect.Ptr {
		if val.Elem().IsValid() {
			kind = val.Elem().Kind()
			// for pointers to basic/collection types and funcs, dereference
			// them (a non-nil *func becomes a callable; without this the Func
			// case would call makeStarFn on the pointer and panic)
			switch kind {
			case reflect.Bool,
				reflect.String,
				reflect.Float32, reflect.Float64,
				reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
				reflect.Slice, reflect.Array, reflect.Map, reflect.Func:
				val = val.Elem()
			}
		} else {
			// If the pointer is nil and points to a struct, make a GoInterface for it
			if val.Type().Elem().Kind() == reflect.Struct {
				return &GoInterface{v: val}, nil
			}
		}
	}

	switch kind {
	case reflect.Bool:
		return starlark.Bool(val.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return starlark.MakeInt64(val.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return starlark.MakeUint64(val.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return starlark.Float(val.Float()), nil
	case reflect.Func:
		return makeStarFn("fn", val, tagName), nil
	case reflect.Map:
		if err := checkCollectionElemTypesCached(val.Type()); err != nil {
			return nil, err
		}
		return &GoMap{v: val, tag: tagName}, nil
	case reflect.String:
		return starlark.String(val.String()), nil
	case reflect.Slice, reflect.Array:
		if err := checkCollectionElemTypesCached(val.Type()); err != nil {
			return nil, err
		}
		return &GoSlice{v: arrayToSlice(val), tag: tagName}, nil
	case reflect.Struct:
		// handle special case from standard starlark lib
		switch val.Type() {
		case reflect.TypeOf(time.Time{}):
			return startime.Time(val.Interface().(time.Time)), nil
		}
		return &GoStruct{v: val, tag: tagName}, nil
	case reflect.Interface:
		// unwrap empty interfaces to their dynamic value: JSON-shaped data
		// (map[string]interface{}, []interface{}) was unusable otherwise,
		// since the opaque wrapper supports no comparison or arithmetic
		// (m["a"] == 1 was False, m["a"] + 1 failed). Interfaces with
		// methods keep the wrapper, which exposes those methods.
		//
		// Caveat: unwrapping routes the value through Starlark, so a sized
		// numeric (int16, float32, ...) stored in an interface collection
		// round-trips through the FromValue ladder and comes back widened
		// (int64 / float64), not its exact original Go type.
		if val.Type().NumMethod() == 0 {
			if val.IsNil() {
				return starlark.None, nil
			}
			if uv, err := toValue(val.Elem(), tagName); err == nil {
				return uv, nil
			}
			// the dynamic value has no Starlark form (e.g. a chan, or a
			// collection with an unsupported element type) — fall back to
			// the opaque wrapper instead of erroring: the static pre-check
			// cannot see through interface{}, and this error would
			// otherwise surface inside methods that cannot return errors
			// (Items, Index, iterators) and escape as a panic
			return &GoInterface{v: val, tag: tagName}, nil
		}
		return &GoInterface{v: val, tag: tagName}, nil
	case reflect.Invalid:
		return starlark.None, nil
	}

	return nil, fmt.Errorf("type %T is not a supported starlark type", val.Interface())
}

// elemTypeCacheCap bounds elemTypeCheckCache. checkCollectionElemTypes is
// run on every collection wrap; real hosts convert a small, fixed set of
// declared collection types, and the cap is the ceiling that keeps
// script-minted array key types (see boundedTypeCache) from pinning
// unbounded memory.
const elemTypeCacheCap = 4096

var elemTypeCheckCache = newBoundedTypeCache(elemTypeCacheCap) // reflect.Type -> error

// checkCollectionElemTypesCached is the cached front of
// checkCollectionElemTypes; use this on conversion hot paths.
func checkCollectionElemTypesCached(t reflect.Type) error {
	return elemTypeCheckCache.loadOrStore(t, checkCollectionElemTypes(t, nil))
}

// checkCollectionElemTypes verifies that the key and element types of a map,
// or the element type of a slice or array, are convertible by toValue,
// recursing through nested collection and pointer types. Without this
// pre-check, a collection like map[string]chan int converted fine and blew
// up with a panic only later, when items() or iteration touched a value of
// the unsupported type. Struct types are not descended into: their fields
// are reached through GoStruct.Attr, which reports a regular error. The
// visited set guards against recursive Go types (e.g. type M map[string]M);
// pass nil to start.
func checkCollectionElemTypes(t reflect.Type, visited map[reflect.Type]bool) error {
	if visited[t] {
		return nil
	}
	if visited == nil {
		visited = make(map[reflect.Type]bool)
	}
	visited[t] = true
	switch t.Kind() {
	case reflect.Chan, reflect.Complex64, reflect.Complex128, reflect.UnsafePointer, reflect.Uintptr:
		return fmt.Errorf("type %s is not a supported starlark type", t)
	case reflect.Map:
		if err := checkCollectionElemTypes(t.Key(), visited); err != nil {
			return err
		}
		return checkCollectionElemTypes(t.Elem(), visited)
	case reflect.Ptr:
		// a *func element is unsupported: a nil one errors in toValue, and
		// GoSlice.Index / iterators cannot return that error (they would
		// panic). A direct func element is fine (it stays callable).
		if t.Elem().Kind() == reflect.Func {
			return fmt.Errorf("type %s is not a supported starlark type", t)
		}
		return checkCollectionElemTypes(t.Elem(), visited)
	case reflect.Slice, reflect.Array:
		return checkCollectionElemTypes(t.Elem(), visited)
	}
	return nil
}

// FromValue converts a starlark value to a go value.
//
// Integer contract: a starlark.Int converts down a fixed ladder — int64 if
// the value fits, else uint64 if it fits, else *math/big.Int. The Go type
// of a round-tripped integer therefore depends on its magnitude and is not
// guaranteed to be identical to what was originally converted in.
func FromValue(v starlark.Value) interface{} {
	return fromValue(v, nil)
}

// fromValue implements FromValue; visited tracks the lists/dicts on the
// current conversion path so self-referential structures convert their
// cyclic references to nil instead of recursing forever. It is passed down
// the call chain instead of living in a package-level detector: a shared
// detector made concurrent conversions of the same value spuriously see
// each other's in-progress markers and silently return nil. Pass nil to
// start.
func fromValue(v starlark.Value, visited map[uintptr]bool) interface{} {
	switch v := v.(type) {
	case starlark.Bool:
		return bool(v)
	case starlark.Int:
		// the integer ladder documented on FromValue:
		// int64 -> uint64 -> *big.Int
		if i, ok := v.Int64(); ok {
			return i
		}
		if i, ok := v.Uint64(); ok {
			return i
		}
		return v.BigInt()
	case starlark.Float:
		return float64(v)
	case starlark.String:
		return string(v)
	case starlark.Bytes:
		return []byte(v)
	case *starlark.List:
		return fromList(v, visited)
	case starlark.Tuple:
		return fromTuple(v, visited)
	case *starlark.Dict:
		return fromDict(v, visited)
	case *starlark.Set:
		return FromSet(v)
	case starlark.NoneType:
		return nil
	case startime.Time:
		return time.Time(v)
	case startime.Duration:
		return time.Duration(v)
	case *GoStruct:
		return v.v.Interface()
	case *GoInterface:
		return v.v.Interface()
	case *GoMap:
		return v.v.Interface()
	case *GoSlice:
		return v.v.Interface()
	default:
		// dunno, hope it's a custom type that the receiver knows how to deal with.
		// This can happen with custom-written go types that implement starlark.Value.
		// Or maybe it's a Starlark function, module, or struct.
		return v
	}
}

// MakeStringDict makes a StringDict from the given arg. The types supported are the same as ToValue.
// It returns an empty dict for nil input.
func MakeStringDict(m map[string]interface{}) (starlark.StringDict, error) {
	return makeStringDictTag(m, emptyStr)
}

// MakeStringDictWithTag makes a StringDict from the given arg with custom tag. The types supported are the same as ToValueWithTag.
// It returns an empty dict for nil input.
func MakeStringDictWithTag(m map[string]interface{}, tagName string) (starlark.StringDict, error) {
	return makeStringDictTag(m, tagName)
}

func makeStringDictTag(m map[string]interface{}, tagName string) (starlark.StringDict, error) {
	dict := make(starlark.StringDict, len(m))
	for k, v := range m {
		val, err := ToValueWithTag(v, tagName)
		if err != nil {
			return nil, err
		}
		dict[k] = val
	}
	return dict, nil
}

// FromStringDict makes a map[string]interface{} from the given arg. Any inconvertible values are ignored.
func FromStringDict(m starlark.StringDict) map[string]interface{} {
	ret := make(map[string]interface{}, len(m))
	for k, v := range m {
		ret[k] = FromValue(v)
	}
	return ret
}

// MakeTuple makes a Starlark Tuple from the given Go slice. The types supported are the same as ToValue.
// It returns an empty tuple for nil input.
func MakeTuple(v []interface{}) (starlark.Tuple, error) {
	tuple := make(starlark.Tuple, len(v))
	for i, val := range v {
		item, err := ToValue(val)
		if err != nil {
			return nil, err
		}
		tuple[i] = item
	}
	return tuple, nil
}

// FromTuple converts a starlark.Tuple into a []interface{}.
func FromTuple(v starlark.Tuple) []interface{} {
	return fromTuple(v, nil)
}

func fromTuple(v starlark.Tuple, visited map[uintptr]bool) []interface{} {
	ret := make([]interface{}, len(v))
	for i := range v {
		ret[i] = fromValue(v[i], visited)
	}
	return ret
}

// MakeList makes a Starlark List from the given Go slice. The types supported are the same as ToValue.
// It returns an empty list for nil input.
func MakeList(v []interface{}) (*starlark.List, error) {
	values := make([]starlark.Value, len(v))
	for i := range v {
		item, err := ToValue(v[i])
		if err != nil {
			return nil, err
		}
		values[i] = item
	}
	return starlark.NewList(values), nil
}

// FromList creates a go slice from the given starlark list. A list that
// reaches itself converts its cyclic reference to nil.
func FromList(l *starlark.List) []interface{} {
	return fromList(l, nil)
}

func fromList(l *starlark.List, visited map[uintptr]bool) []interface{} {
	// return nil to avoid infinite recursion
	p := reflect.ValueOf(l).Pointer()
	if visited[p] {
		return nil
	}
	if visited == nil {
		visited = make(map[uintptr]bool)
	}
	visited[p] = true
	defer delete(visited, p)

	ret := make([]interface{}, 0, l.Len())
	var v starlark.Value
	i := l.Iterate()
	defer i.Done()
	for i.Next(&v) {
		val := fromValue(v, visited)
		ret = append(ret, val)
	}
	return ret
}

// MakeDict makes a Dict from the given map. The acceptable keys and values are the same as ToValue.
// For nil input, it returns an empty Dict. It panics if the input is not a map.
func MakeDict(v interface{}) (starlark.Value, error) {
	return makeDictTag(reflect.ValueOf(v), emptyStr)
}

// MakeDictWithTag makes a Dict from the given map with custom tag. The acceptable keys and values are the same as ToValueWithTag.
// For nil input, it returns an empty Dict. It panics if the input is not a map.
func MakeDictWithTag(v interface{}, tagName string) (starlark.Value, error) {
	return makeDictTag(reflect.ValueOf(v), tagName)
}

func makeDictTag(val reflect.Value, tagName string) (starlark.Value, error) {
	dict := starlark.NewDict(1)
	// check if the value is not nil and is a map
	if valid := val.IsValid(); valid && val.Kind() != reflect.Map {
		// panic if not a map
		panic(fmt.Errorf("can't make map of %T", val.Interface()))
	} else if valid {
		// iterate over the map and convert each key and value, in
		// deterministic key order: Starlark dicts preserve insertion order,
		// so the random order of MapKeys would be script-visible
		for _, k := range sortedMapKeys(val) {
			vk, err := toValue(k, tagName)
			if err != nil {
				return nil, err
			}
			vv, err := toValue(val.MapIndex(k), tagName)
			if err != nil {
				return nil, err
			}
			// SetKey fails for keys that are unhashable on the Starlark
			// side; dropping the entry silently is data loss
			if err := dict.SetKey(vk, vv); err != nil {
				return nil, fmt.Errorf("dict key %s: %v", vk.String(), err)
			}
		}
	}
	return dict, nil
}

// bigIntKey is the comparable, value-stable Go map key form of a
// starlark.Int too large for int64/uint64. FromValue yields such ints as
// *big.Int, which is comparable only by pointer identity — so two equal
// large ints would become two distinct map keys. Wrapping the decimal
// string in a named struct keeps the key value-stable and distinct from a
// plain string key of the same text (mirroring how bytes keys stay
// distinct from string keys).
type bigIntKey struct{ s string }

// hashableGoValue converts a Starlark value into a Go value whose dynamic
// type is comparable AND compares by value, so it is safe and correct as a
// Go map key. It matches FromValue except for the types whose natural Go
// form is unsuitable as a key:
//
//   - starlark.Tuple becomes a fixed-size [N]interface{} array (elements are
//     converted recursively), so equal tuples stay equal as map keys;
//   - starlark.Bytes becomes a fixed-size [N]byte array, which keeps bytes
//     keys distinct from equal string keys;
//   - a large starlark.Int (FromValue's *big.Int, comparable only by
//     pointer identity) becomes a bigIntKey, so equal large ints map to the
//     same Go key.
//
// It returns an error for values with no comparable Go form (e.g. dicts,
// lists, sets, or custom values that convert to non-comparable Go types);
// using such a value as a Go map key would otherwise escape as a runtime
// "hash of unhashable type" panic and kill the host process.
func hashableGoValue(v starlark.Value) (interface{}, error) {
	switch v := v.(type) {
	case starlark.Tuple:
		arr := reflect.New(reflect.ArrayOf(len(v), emptyIfaceType)).Elem()
		for i, elem := range v {
			g, err := hashableGoValue(elem)
			if err != nil {
				return nil, err
			}
			if g != nil {
				arr.Index(i).Set(reflect.ValueOf(g))
			}
		}
		return arr.Interface(), nil
	case starlark.Bytes:
		arr := reflect.New(reflect.ArrayOf(len(v), byteType)).Elem()
		reflect.Copy(arr, reflect.ValueOf([]byte(v)))
		return arr.Interface(), nil
	}
	g := FromValue(v)
	if g == nil {
		return nil, nil
	}
	if bi, ok := g.(*big.Int); ok {
		return bigIntKey{s: bi.String()}, nil
	}
	gt := reflect.TypeOf(g)
	if !gt.Comparable() {
		return nil, fmt.Errorf("value of type %s converts to Go type %T which is not hashable", v.Type(), g)
	}
	if !comparableByValue(gt) {
		// Comparable() is true for pointers/chans, but they compare by
		// identity, not value — so equal Starlark values (e.g. two equal
		// structs, or a custom pointer-backed value) would become distinct,
		// unretrievable Go map keys. Reject rather than silently misbehave,
		// matching the unhashable-key contract. (*big.Int is intercepted
		// above; FromDict falls back to the key's printed form.)
		return nil, fmt.Errorf("value of type %s converts to Go type %T which compares by identity, not value, so it is not usable as a stable map key", v.Type(), g)
	}
	return g, nil
}

// comparableByValue reports whether two equal values of type t compare equal
// as Go map keys by VALUE. reflect.Type.Comparable() is necessary but not
// sufficient: pointers, channels, and unsafe.Pointers are comparable yet
// compare by identity, and a struct or array is value-comparable only if
// every element is. Interface fields are treated as identity (their dynamic
// value is unknown).
func comparableByValue(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Ptr, reflect.Chan, reflect.UnsafePointer, reflect.Interface:
		return false
	case reflect.Array:
		return comparableByValue(t.Elem())
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if !comparableByValue(t.Field(i).Type) {
				return false
			}
		}
		return true
	}
	return true
}

// tryKeyConv converts a Starlark value used as a map key into a
// reflect.Value suitable for indexing a Go map with key type t. Unlike
// tryConv, it guarantees the result is usable as a Go map key: for
// interface key types it routes through hashableGoValue, so tuples become
// comparable arrays and values with no comparable Go form yield an error
// instead of a runtime "hash of unhashable type" panic.
func tryKeyConv(v starlark.Value, t reflect.Type) (reflect.Value, error) {
	if t.Kind() == reflect.Interface {
		g, err := hashableGoValue(v)
		if err != nil {
			return reflect.Value{}, err
		}
		if g == nil {
			return reflect.Zero(t), nil
		}
		out := reflect.ValueOf(g)
		if t.NumMethod() > 0 && !out.Type().Implements(t) {
			return reflect.Value{}, fmt.Errorf("value of type %s cannot be converted to type %s", out.Type(), t)
		}
		return out, nil
	}
	// Non-interface Go map key types are comparable by construction, so the
	// regular conversion is already safe.
	return tryConv(v, t)
}

// FromDict converts a starlark.Dict to a map[interface{}]interface{}.
// Keys are converted with the same rules as TryFromDict; a key with no
// comparable Go form falls back to its printed form (k.String()) so the
// entry is preserved instead of panicking. Use TryFromDict to detect such
// keys instead of silently accepting the fallback.
func FromDict(m *starlark.Dict) map[interface{}]interface{} {
	return fromDict(m, nil)
}

func fromDict(m *starlark.Dict, visited map[uintptr]bool) map[interface{}]interface{} {
	// return nil to avoid infinite recursion
	p := reflect.ValueOf(m).Pointer()
	if visited[p] {
		return nil
	}
	if visited == nil {
		visited = make(map[uintptr]bool)
	}
	visited[p] = true
	defer delete(visited, p)

	ret := make(map[interface{}]interface{}, m.Len())
	for _, k := range m.Keys() {
		key, err := hashableGoValue(k)
		if err != nil {
			// no comparable Go form: keep the entry under its printed form
			// rather than panicking or dropping it silently
			key = k.String()
		}
		// should never be not found or unhashable, so ignore err and found.
		val, _, _ := m.Get(k)
		ret[key] = fromValue(val, visited)
	}
	return ret
}

// TryFromDict converts a starlark.Dict to a map[interface{}]interface{}.
// It works like FromDict, but returns an error if any key has no comparable
// Go form (e.g. a custom value that converts to a non-comparable Go type)
// instead of falling back to the key's printed form.
func TryFromDict(m *starlark.Dict) (map[interface{}]interface{}, error) {
	ret := make(map[interface{}]interface{}, m.Len())
	for _, k := range m.Keys() {
		key, err := hashableGoValue(k)
		if err != nil {
			return nil, fmt.Errorf("dict key %s: %v", k.String(), err)
		}
		// should never be not found or unhashable, so ignore err and found.
		val, _, _ := m.Get(k)
		ret[key] = fromValue(val, nil)
	}
	return ret, nil
}

// MakeSet makes a Set from the given map. The acceptable keys the same as ToValue.
// For nil input, it returns an empty Set.
func MakeSet(s map[interface{}]bool) (*starlark.Set, error) {
	set := starlark.Set{}
	for k := range s {
		key, err := ToValue(k)
		if err != nil {
			return nil, err
		}
		if err = set.Insert(key); err != nil {
			return nil, err
		}
	}
	return &set, nil
}

// MakeSetFromSlice makes a Set from the given slice. The acceptable keys the same as ToValue.
// For nil input, it returns an empty Set.
func MakeSetFromSlice(s []interface{}) (*starlark.Set, error) {
	set := starlark.Set{}
	for i := range s {
		key, err := ToValue(s[i])
		if err != nil {
			return nil, err
		}
		if err = set.Insert(key); err != nil {
			return nil, err
		}
	}
	return &set, nil
}

// FromSetToSlice converts a starlark.Set into a []interface{}, preserving
// the set's iteration (insertion) order. Use it instead of FromSet when the
// order of members matters: FromSet returns a Go map, which has no defined
// iteration order.
func FromSetToSlice(s *starlark.Set) []interface{} {
	ret := make([]interface{}, 0, s.Len())
	var v starlark.Value
	i := s.Iterate()
	defer i.Done()
	for i.Next(&v) {
		ret = append(ret, FromValue(v))
	}
	return ret
}

// FromSet converts a starlark.Set to a map[interface{}]bool.
// Elements are converted with the same rules as TryFromSet; an element with
// no comparable Go form falls back to its printed form (v.String()) so the
// member is preserved instead of panicking. Use TryFromSet to detect such
// elements instead of silently accepting the fallback.
func FromSet(s *starlark.Set) map[interface{}]bool {
	ret := make(map[interface{}]bool, s.Len())
	var v starlark.Value
	i := s.Iterate()
	defer i.Done()
	for i.Next(&v) {
		val, err := hashableGoValue(v)
		if err != nil {
			// no comparable Go form: keep the member under its printed form
			// rather than panicking or dropping it silently
			val = v.String()
		}
		ret[val] = true
	}
	return ret
}

// TryFromSet converts a starlark.Set to a map[interface{}]bool.
// It works like FromSet, but returns an error if any element has no
// comparable Go form (e.g. a custom value that converts to a non-comparable
// Go type) instead of falling back to the element's printed form.
func TryFromSet(s *starlark.Set) (map[interface{}]bool, error) {
	ret := make(map[interface{}]bool, s.Len())
	var v starlark.Value
	i := s.Iterate()
	defer i.Done()
	for i.Next(&v) {
		val, err := hashableGoValue(v)
		if err != nil {
			return nil, fmt.Errorf("set element %s: %v", v.String(), err)
		}
		ret[val] = true
	}
	return ret, nil
}

// Kwarg is a single instance of a python foo=bar style named argument.
type Kwarg struct {
	Name  string
	Value interface{}
}

// FromKwargs converts a Python style name=val, name2=val2 list of tuples into a
// []Kwarg.  It is an error if any tuple is not exactly 2 values,
// or if the first one is not a string.
func FromKwargs(kwargs []starlark.Tuple) ([]Kwarg, error) {
	args := make([]Kwarg, 0, len(kwargs))
	for _, t := range kwargs {
		tup := FromTuple(t)
		if len(tup) != 2 {
			return nil, fmt.Errorf("kwarg tuple should have 2 vals, has %v", len(tup))
		}
		s, ok := tup[0].(string)
		if !ok {
			return nil, fmt.Errorf("expected name of kwarg to be string, but was %T (%#v)", tup[0], tup[0])
		}
		args = append(args, Kwarg{Name: s, Value: tup[1]})
	}
	return args, nil
}

// MakeStarFn creates a wrapper around the given function that can be called from a starlark script. Argument support is the same as ToValue.
// If the last value the function returns is an error, it will cause an error to be returned from the starlark function.
// If there are no other errors, the function will return None.
// If there's exactly one other value, the function will return the starlark equivalent of that value.
// If there is more than one return value, they'll be returned as a tuple.
// MakeStarFn will panic if you pass it something other than a function, like nil or a non-function.
func MakeStarFn(name string, gofn interface{}) *starlark.Builtin {
	v := reflect.ValueOf(gofn)
	if v.Kind() != reflect.Func {
		panic(errors.New("fn is not a function"))
	}
	return makeStarFn(name, v, emptyStr)
}

func makeStarFn(name string, gofn reflect.Value, tagName string) *starlark.Builtin {
	if gofn.Type().IsVariadic() {
		return makeVariadicStarFn(name, gofn, tagName)
	}
	return starlark.NewBuiltin(name, func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (sv starlark.Value, ef error) {
		defer func() {
			if r := recover(); r != nil {
				sv = starlark.None
				ef = fmt.Errorf("panic in func %s: %v", name, r)
			}
		}()

		if len(kwargs) > 0 {
			return starlark.None, fmt.Errorf("%s: unexpected keyword argument %s (wrapped Go functions accept positional arguments only)", name, kwargs[0][0].String())
		}
		if len(args) != gofn.Type().NumIn() {
			return starlark.None, fmt.Errorf("expected %d args but got %d", gofn.Type().NumIn(), len(args))
		}

		// convert all the args
		vals := FromTuple(args)
		rvs := make([]reflect.Value, 0, len(vals))
		for i, v := range vals {
			val := reflect.ValueOf(v)
			argT := gofn.Type().In(i)

			var err error
			val, err = convertReflectValue(val, argT)
			if err != nil {
				return starlark.None, fmt.Errorf("arg %d: %v", i, err)
			}

			rvs = append(rvs, val)
		}

		out := gofn.Call(rvs)
		return makeOut(out, tagName)
	})
}

func makeVariadicStarFn(name string, gofn reflect.Value, tagName string) *starlark.Builtin {
	return starlark.NewBuiltin(name, func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (sv starlark.Value, ef error) {
		defer func() {
			if r := recover(); r != nil {
				sv = starlark.None
				ef = fmt.Errorf("panic in func %s: %v", name, r)
			}
		}()

		if len(kwargs) > 0 {
			return starlark.None, fmt.Errorf("%s: unexpected keyword argument %s (wrapped Go functions accept positional arguments only)", name, kwargs[0][0].String())
		}
		minArgs := gofn.Type().NumIn() - 1
		if len(args) < minArgs {
			return starlark.None, fmt.Errorf("expected at least %d args but got %d", minArgs, len(args))
		}

		// convert all the args
		vals := FromTuple(args)
		rvs := make([]reflect.Value, 0, len(args))

		// grab all the non-variadics first
		for i := 0; i < minArgs; i++ {
			val := reflect.ValueOf(vals[i])
			argT := gofn.Type().In(i)

			var err error
			val, err = convertReflectValue(val, argT)
			if err != nil {
				return starlark.None, fmt.Errorf("arg %d: %v", i, err)
			}

			rvs = append(rvs, val)
		}
		// last "in" type by definition must be a slice of something. We need to
		// know what something, so we can convert things as needed.
		vtype := gofn.Type().In(gofn.Type().NumIn() - 1).Elem()
		// the rest of the args need to be batched into a slice for the variadic
		for i := minArgs; i < len(vals); i++ {
			val := reflect.ValueOf(vals[i])

			var err error
			val, err = convertReflectValue(val, vtype)
			if err != nil {
				return starlark.None, fmt.Errorf("arg %d: %v", i, err)
			}
			rvs = append(rvs, val)
		}
		out := gofn.Call(rvs)
		return makeOut(out, tagName)
	})
}

func makeOut(out []reflect.Value, tagName string) (starlark.Value, error) {
	if len(out) == 0 {
		return starlark.None, nil
	}
	last := out[len(out)-1]
	var err error
	// the error-return convention matches any type implementing error, not
	// just the error interface itself: a concrete error type in the last
	// position used to leak through as a regular value instead of raising.
	//
	// Edge case: a value (non-pointer) type whose Error() has a value
	// receiver can never be nil, so the isNil guard below cannot treat its
	// zero value as "no error" — such a return always raises. Idiomatic Go
	// errors are pointer/interface types (nilable), so this is rare; use a
	// nilable error type if a zero-value "no error" sentinel is needed.
	if last.Type() == errType || last.Type().Implements(errType) {
		isNil := false
		switch last.Kind() {
		case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
			isNil = last.IsNil()
		}
		if !isNil {
			err = last.Interface().(error)
		}
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return starlark.None, err
	}
	if len(out) == 1 {
		v, err2 := toValue(out[0], tagName)
		if err2 != nil {
			return starlark.None, err2
		}
		return v, err
	}
	// tuple-up multiple values
	res := make([]starlark.Value, 0, len(out))
	for i := range out {
		val, err3 := toValue(out[i], tagName)
		if err3 != nil {
			return starlark.None, err3
		}
		res = append(res, val)
	}
	return starlark.Tuple(res), err
}

// convertReflectValue converts a reflect.Value to a given type. Conversions
// go through checkedConvert, so values that ConvertibleTo would silently
// corrupt (codepoint conversion, wrap-around, truncation) are errors. An
// invalid val (a Starlark None) is accepted only for nullable types, the
// same policy tryConv applies (it used to be silently zeroed here).
func convertReflectValue(val reflect.Value, argT reflect.Type) (reflect.Value, error) {
	if !val.IsValid() {
		switch argT.Kind() {
		case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Interface, reflect.Func:
			return reflect.Zero(argT), nil
		default:
			return reflect.Value{}, fmt.Errorf("value of type None cannot be converted to non-nullable type %s", argT)
		}
	}
	if val.Type().AssignableTo(argT) {
		return val, nil
	}
	if val.Type().ConvertibleTo(argT) {
		return checkedConvert(val, argT)
	}
	if val.Kind() == reflect.Slice && argT.Kind() == reflect.Slice {
		return convertSlice(val, argT)
	}
	if val.Kind() == reflect.Map && argT.Kind() == reflect.Map {
		return convertMap(val, argT)
	}
	return reflect.Value{}, fmt.Errorf("expected type %v got %v", argT, val.Type())
}

func convertSlice(val reflect.Value, argT reflect.Type) (reflect.Value, error) {
	argElem := argT.Elem()
	valLen := val.Len()
	newSlice := reflect.MakeSlice(argT, valLen, valLen)

	for i := 0; i < valLen; i++ {
		elem := val.Index(i)

		// a None element arrives as a nil interface value; apply the same
		// policy as scalar arguments before any elem.Elem() access (which
		// panics on a zero Value)
		if (elem.Kind() == reflect.Interface || elem.Kind() == reflect.Ptr) && elem.IsNil() {
			switch argElem.Kind() {
			case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Interface, reflect.Func:
				// the zero element is already the nil of the target type
				continue
			default:
				return reflect.Value{}, fmt.Errorf("slice element %d: value of type None cannot be converted to non-nullable type %s", i, argElem)
			}
		}

		if elem.Type().AssignableTo(argElem) {
			newSlice.Index(i).Set(elem)
		} else if elem.Type().ConvertibleTo(argElem) {
			cv, err := checkedConvert(elem, argElem)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("slice element %d: %v", i, err)
			}
			newSlice.Index(i).Set(cv)
		} else if (elem.Kind() == reflect.Interface || elem.Kind() == reflect.Ptr) && elem.Elem().Type().ConvertibleTo(argElem) {
			// only unwrap interface/pointer elements; elem.Elem() panics on a
			// concrete-kind element (e.g. an int8 from a wrapped []int8)
			cv, err := checkedConvert(elem.Elem(), argElem)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("slice element %d: %v", i, err)
			}
			newSlice.Index(i).Set(cv)
		} else {
			return reflect.Value{}, fmt.Errorf("expected slice element type %v got %v", argElem, elem.Type())
		}
	}

	return newSlice, nil
}

func convertMap(val reflect.Value, argT reflect.Type) (reflect.Value, error) {
	argKey := argT.Key()
	argElem := argT.Elem()
	newMap := reflect.MakeMapWithSize(argT, val.Len())

	for _, key := range val.MapKeys() {
		newKey, err := convertElemValue(key, argKey)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("map key conversion failed: %v", err)
		}

		valElem := val.MapIndex(key)
		newElem, err := convertElemValue(valElem, argElem)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("map value conversion failed: %v", err)
		}

		newMap.SetMapIndex(newKey, newElem)
	}

	return newMap, nil
}

func convertElemValue(val reflect.Value, targetType reflect.Type) (reflect.Value, error) {
	if val.Type().AssignableTo(targetType) || val.Type().ConvertibleTo(targetType) {
		return checkedConvert(val, targetType)
	} else if val.Kind() == reflect.Ptr || val.Kind() == reflect.Interface {
		if val.IsNil() {
			// same None policy as the other entry points: nullable target
			// types accept it as their zero value
			switch targetType.Kind() {
			case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Interface, reflect.Func:
				return reflect.Zero(targetType), nil
			default:
				return reflect.Value{}, fmt.Errorf("value of type None cannot be converted to non-nullable type %s", targetType)
			}
		}
		if val.Elem().Type().ConvertibleTo(targetType) {
			return checkedConvert(val.Elem(), targetType)
		} else if val.Type().Kind() == reflect.Interface {
			unwrapped := val.Elem()
			if unwrapped.Type().ConvertibleTo(targetType) {
				return checkedConvert(unwrapped, targetType)
			} else if sv, ok := unwrapped.Interface().(starlark.Value); ok {
				// reached when a host passes a custom starlark.Value inside a
				// collection argument (e.g. map[string]interface{}{"k":
				// myStarlarkValue}) to a typed Go parameter: FromValue leaves
				// such values as-is, so we unwrap and re-narrow here. Route
				// through checkedConvert so out-of-range narrowing errors
				// instead of silently wrapping around.
				gv := reflect.ValueOf(FromValue(sv))
				if !gv.IsValid() {
					return reflect.Value{}, fmt.Errorf("nil value cannot be converted to type %v", targetType)
				}
				return checkedConvert(gv, targetType)
			}
		}
	}
	return reflect.Value{}, fmt.Errorf("expected type %v got %v", targetType, val.Type())
}

// tryConv tries to convert starlark.Value v to Go t if v is not assignable to t.
func tryConv(v starlark.Value, t reflect.Type) (reflect.Value, error) {
	if v == starlark.None {
		switch t.Kind() {
		case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Interface, reflect.Func:
			return reflect.Zero(t), nil
		default:
			return reflect.Value{}, fmt.Errorf("value of type None cannot be converted to non-nullable type %s", t)
		}
	}
	out := reflect.ValueOf(FromValue(v))
	return checkedConvert(out, t)
}

// checkedConvert converts val to type t like reflect's Convert, but refuses
// the conversions ConvertibleTo permits that silently corrupt the value on
// the way through:
//
//   - integer -> string is Go's codepoint conversion (65 -> "A"), never
//     what a script means;
//   - numeric narrowing that overflows the target wraps around
//     (1000 -> int8(-24));
//   - float -> integer conversion truncates (3.9 -> 3); whole floats
//     convert fine.
//
// Values already assignable to t pass through untouched.
func checkedConvert(val reflect.Value, t reflect.Type) (reflect.Value, error) {
	if val.Type().AssignableTo(t) {
		return val, nil
	}
	if !val.Type().ConvertibleTo(t) {
		return reflect.Value{}, fmt.Errorf("value of type %s cannot be converted to type %s", val.Type(), t)
	}
	zt := reflect.Zero(t)
	switch t.Kind() {
	case reflect.String:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return reflect.Value{}, fmt.Errorf("value of type %s cannot be converted to type %s (integer to string would be a codepoint conversion)", val.Type(), t)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if zt.OverflowInt(val.Int()) {
				return reflect.Value{}, fmt.Errorf("value %d out of range for type %s", val.Int(), t)
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			u := val.Uint()
			if u > math.MaxInt64 || zt.OverflowInt(int64(u)) {
				return reflect.Value{}, fmt.Errorf("value %d out of range for type %s", u, t)
			}
		case reflect.Float32, reflect.Float64:
			f := val.Float()
			if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) {
				return reflect.Value{}, fmt.Errorf("value %v would be truncated when converted to type %s", f, t)
			}
			if f < math.MinInt64 || f >= math.MaxInt64 || zt.OverflowInt(int64(f)) {
				return reflect.Value{}, fmt.Errorf("value %v out of range for type %s", f, t)
			}
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			i := val.Int()
			if i < 0 || zt.OverflowUint(uint64(i)) {
				return reflect.Value{}, fmt.Errorf("value %d out of range for type %s", i, t)
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if zt.OverflowUint(val.Uint()) {
				return reflect.Value{}, fmt.Errorf("value %d out of range for type %s", val.Uint(), t)
			}
		case reflect.Float32, reflect.Float64:
			f := val.Float()
			if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) {
				return reflect.Value{}, fmt.Errorf("value %v would be truncated when converted to type %s", f, t)
			}
			if f < 0 || f >= math.MaxUint64 || zt.OverflowUint(uint64(f)) {
				return reflect.Value{}, fmt.Errorf("value %v out of range for type %s", f, t)
			}
		}
	case reflect.Float32, reflect.Float64:
		switch val.Kind() {
		case reflect.Float32, reflect.Float64:
			if zt.OverflowFloat(val.Float()) {
				return reflect.Value{}, fmt.Errorf("value %v out of range for type %s", val.Float(), t)
			}
		}
	}
	return val.Convert(t), nil
}
