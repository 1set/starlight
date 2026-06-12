package convert

import (
	"errors"
	"fmt"
	"reflect"
	"sort"

	"go.starlark.net/starlark"
)

// MakeGoInterface converts the given value into a GoInterface.
// This will panic if the value is nil or the type is not a bool, string, float kind, int kind, or uint kind.
func MakeGoInterface(v interface{}) *GoInterface {
	val := reflect.ValueOf(v)
	ifc, ok := makeGoInterface(val)
	if !ok {
		panic(fmt.Errorf("value of type %T is not supported by GoInterface", val.Interface()))
	}
	return ifc
}

func makeGoInterface(val reflect.Value) (*GoInterface, bool) {
	// we accept pointers to anything except structs, which should go through GoStruct.
	if val.Kind() == reflect.Ptr && val.Elem().Kind() == reflect.Struct {
		return nil, false
	}
	switch val.Kind() {
	case reflect.Ptr,
		reflect.Bool,
		reflect.String,
		reflect.Float32, reflect.Float64,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &GoInterface{v: val}, true
	}
	return nil, false
}

// GoInterface wraps a go value to expose its methods to starlark scripts. Basic
// types will not behave as their base type (you can't add 2 to an ID, even if
// it is an int underneath).
type GoInterface struct {
	_   DoNotCompare
	v   reflect.Value
	tag string
}

// Attr returns a starlark value that wraps the method or field with the given name.
func (g *GoInterface) Attr(name string) (starlark.Value, error) {
	switch name {
	case "toInt":
		return MakeStarFn(name, g.ToInt), nil
	case "toString":
		return MakeStarFn(name, g.ToString), nil
	case "toFloat":
		return MakeStarFn(name, g.ToFloat), nil
	case "toUint":
		return MakeStarFn(name, g.ToUint), nil
	case "toBool":
		return MakeStarFn(name, g.ToBool), nil
	}

	method := g.v.MethodByName(name)
	if method.Kind() != reflect.Invalid && method.CanInterface() {
		return makeStarFn(name, method, g.tag), nil
	}
	return nil, nil
}

// interfaceAttrNames are the synthetic conversion attributes every
// GoInterface supports through Attr, in addition to the wrapped value's
// own methods.
var interfaceAttrNames = []string{"toBool", "toFloat", "toInt", "toString", "toUint"}

// AttrNames returns the names Attr resolves for this value: the wrapped
// value's methods plus the synthetic to* conversion attributes, sorted and
// without duplicates.
func (g *GoInterface) AttrNames() []string {
	if !g.v.IsValid() {
		return nil
	}

	names := make([]string, 0, g.v.NumMethod()+len(interfaceAttrNames))
	names = append(names, interfaceAttrNames...)
	// a pointer's method set already includes its element's methods, so a
	// single pass over g.v covers both
	for i := 0; i < g.v.NumMethod(); i++ {
		names = append(names, g.v.Type().Method(i).Name)
	}
	sort.Strings(names)
	// deduplicate (a method may shadow a synthetic name)
	nn := names[:0]
	for i, n := range names {
		if i == 0 || n != names[i-1] {
			nn = append(nn, n)
		}
	}
	return nn
}

// String returns the string representation of the value.
// Cyclic or overly deep values are elided as "<cyclic TYPE>" instead of
// overflowing the stack; see safeGoString.
func (g *GoInterface) String() string {
	return safeGoString(g.v)
}

// Type returns a short string describing the value's type.
func (g *GoInterface) Type() string {
	return fmt.Sprintf("starlight_interface<%T>", g.v.Interface())
}

// Value returns reflect.Value of the underlying value.
func (g *GoInterface) Value() reflect.Value {
	return g.v
}

// Freeze is a no-op: GoInterface exposes no write path, so there is
// nothing to freeze. The wrapped Go value itself is not protected — the
// host can still mutate it.
func (g *GoInterface) Freeze() {}

// Truth returns the truth value of an object.
func (g *GoInterface) Truth() starlark.Bool {
	switch g.v.Kind() {
	case reflect.Ptr:
		return starlark.Bool(!g.v.IsNil())
	case reflect.Bool:
		return starlark.Bool(g.v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return g.v.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return g.v.Uint() > 0
	case reflect.Float32, reflect.Float64:
		return g.v.Float() != 0
	case reflect.String:
		return g.v.String() != ""
	case reflect.Chan, reflect.Map, reflect.Func, reflect.Slice, reflect.Interface:
		// nilable kinds: a nil value is falsy, like the Ptr case above
		return starlark.Bool(!g.v.IsNil())
	}
	// otherwise: assume truthy (e.g. structs, arrays, complex)
	return true
}

// Hash returns a function of x such that Equals(x, y) => Hash(x) == Hash(y).
// Hash may fail if the value's type is not hashable, or if the value
// contains a non-hashable value.
func (g *GoInterface) Hash() (uint32, error) {
	return 0, errors.New("starlight_interface is not hashable")
}

// Below are conversion functions, they only work on the appropriate underlying type.
// Note that there is no ToBool because Truth() already serves that purpose.

// ToInt converts the interface value into a starlark int.  This will fail if
// the underlying type is not an int type or pointer to an int type.
func (g *GoInterface) ToInt() (int64, error) {
	v := g.v
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int(), nil
	}
	return 0, fmt.Errorf("can't convert type %s to int64", g.v.Type())
}

// ToBool converts the interface value into a starlark bool.  This will fail if
// the underlying type is not a bool type or pointer to a bool type.
func (g *GoInterface) ToBool() (bool, error) {
	v := g.v
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Bool:
		return v.Bool(), nil
	}
	return false, fmt.Errorf("can't convert type %s to bool", g.v.Type())
}

// ToUint converts the interface value into a starlark int.  This will fail if
// the underlying type is not an uint type or pointer to an uint type.
func (g *GoInterface) ToUint() (uint64, error) {
	v := g.v
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint(), nil
	}
	return 0, fmt.Errorf("can't convert type %s to uint64", g.v.Type())
}

// ToString converts the interface value into a starlark string.  This will fail if
// the underlying type is not a string (including if the underlying type is a
// pointer to a string).
func (g *GoInterface) ToString() (string, error) {
	switch g.v.Kind() {
	case reflect.String:
		return g.v.String(), nil
	}
	return "", fmt.Errorf("can't convert type %T to string", g.v)
}

// ToFloat converts the interface value into a starlark float.  This will fail
// if the underlying type is not a float type (including if the underlying type
// is a pointer to a float).
func (g *GoInterface) ToFloat() (float64, error) {
	switch g.v.Kind() {
	case reflect.Float32, reflect.Float64:
		return g.v.Float(), nil
	}
	return 0, fmt.Errorf("can't convert type %T to float64", g.v)
}
