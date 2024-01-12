package convert

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"go.starlark.net/starlark"
)

var (
	// DefaultPropertyTag is the default struct tag to use when converting
	DefaultPropertyTag = "starlark"
)

// NewStruct makes a new Starlark-compatible Struct from the given struct or pointer to struct.
// This will panic if you pass it nil or anything else.
func NewStruct(strct interface{}) *GoStruct {
	val := reflect.ValueOf(strct)
	if val.Kind() == reflect.Struct || (val.Kind() == reflect.Ptr && val.Elem().Kind() == reflect.Struct) {
		return &GoStruct{v: val}
	}
	panic(fmt.Errorf("value must be a struct or pointer to a struct, but was %T", val.Interface()))
}

// NewStructWithTag makes a new Starlark-compatible Struct from the given struct or pointer to struct,
// using the given struct tag to determine which fields to expose.
// This will panic if you pass it anything else.
func NewStructWithTag(strct interface{}, tagName string) *GoStruct {
	val := reflect.ValueOf(strct)
	if val.Kind() == reflect.Struct || (val.Kind() == reflect.Ptr && val.Elem().Kind() == reflect.Struct) {
		if tagName == "" {
			tagName = DefaultPropertyTag
		}
		return &GoStruct{v: val, tag: tagName}
	}
	panic(fmt.Errorf("value must be a struct or pointer to a struct, but was %T", val.Interface()))
}

// GoStruct is a wrapper around a Go struct to let it be manipulated by Starlark scripts.
type GoStruct struct {
	v   reflect.Value
	tag string
}

// Attr returns a Starlark value that wraps the method or field with the given name.
func (g *GoStruct) Attr(name string) (starlark.Value, error) {
	// check for its methods and its pointer's methods
	method := g.v.MethodByName(name)
	if method.Kind() != reflect.Invalid && method.CanInterface() {
		return makeStarFn(name, method, g.tag), nil
	}
	v := g.v
	if g.v.Kind() == reflect.Ptr {
		v = v.Elem()
		method = g.v.MethodByName(name)
		if method.Kind() != reflect.Invalid && method.CanInterface() {
			return makeStarFn(name, method, g.tag), nil
		}
	}

	// check for properties
	var (
		field reflect.Value
		found bool
	)

	// check each field
	tagName := g.tagName()
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		tag, ok := extractTagOrFieldName(t.Field(i), tagName)
		if !ok {
			continue
		}

		// check if the tag name matches the given name
		if tag == name {
			field = v.Field(i)
			found = true
			break
		}
	}

	// return the field if found
	if found && field.Kind() != reflect.Invalid {
		return toValue(field, g.tag)
	}

	// for not found
	return nil, nil
}

// AttrNames returns the list of all fields and methods on this struct.
func (g *GoStruct) AttrNames() []string {
	// count the number of methods and fields
	v := g.v
	count := v.NumMethod()
	if v.Kind() == reflect.Ptr {
		elem := v.Elem()
		count += elem.NumField() + elem.NumMethod()
	} else {
		count += v.NumField()
	}
	names := make([]string, 0, count)

	// get the defined tag name
	tagName := g.tagName()
	saveFieldName := func(f reflect.StructField) {
		tag, ok := extractTagOrFieldName(f, tagName)
		if ok {
			names = append(names, tag)
		}
	}

	// check each methods and fields
	for i := 0; i < v.NumMethod(); i++ {
		names = append(names, v.Type().Method(i).Name)
	}
	if v.Kind() == reflect.Ptr {
		t := v.Elem().Type()
		for i := 0; i < t.NumField(); i++ {
			saveFieldName(t.Field(i))
		}
		for i := 0; i < t.NumMethod(); i++ {
			names = append(names, t.Method(i).Name)
		}
	} else {
		for i := 0; i < v.NumField(); i++ {
			saveFieldName(v.Type().Field(i))
		}
	}

	// deduplicate names
	nn := make([]string, 0, len(names))
	ns := make(map[string]struct{}, len(names))
	for _, n := range names {
		if _, ok := ns[n]; !ok {
			nn = append(nn, n)
			ns[n] = struct{}{}
		}
	}
	return nn
}

// SetField sets the struct field with the given name with the given value.
func (g *GoStruct) SetField(name string, val starlark.Value) error {
	v := g.v
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	// check for properties
	var (
		field reflect.Value
		found bool
	)

	// check each field to find the field
	tagName := g.tagName()
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		tag, ok := extractTagOrFieldName(t.Field(i), tagName)
		if !ok {
			continue
		}

		// check if the tag name matches the given name
		if tag == name {
			field = v.Field(i)
			found = true
			break
		}
	}

	// if not found
	if !found {
		return fmt.Errorf("field %s is not found", name)
	}

	// try to set the field
	if field.CanSet() {
		val, err := tryConv(val, field.Type())
		if err != nil {
			return err
		}
		field.Set(val)
		return nil
	}
	return fmt.Errorf("%s is not a settable field", name)
}

// String returns the string representation of the value.
// Starlark string values are quoted as if by Python's repr.
func (g *GoStruct) String() string {
	return fmt.Sprint(g.v.Interface())
}

// Type returns a short string describing the value's type.
func (g *GoStruct) Type() string {
	return fmt.Sprintf("starlight_struct<%T>", g.v.Interface())
}

// Value returns reflect.Value of the underlying struct.
func (g *GoStruct) Value() reflect.Value {
	return g.v
}

// Freeze causes the value, and all values transitively
// reachable from it through collections and closures, to be
// marked as frozen.  All subsequent mutations to the data
// structure through this API will fail dynamically, making the
// data structure immutable and safe for publishing to other
// Starlark interpreters running concurrently.
func (g *GoStruct) Freeze() {}

// Truth returns the truth value of an object.
func (g *GoStruct) Truth() starlark.Bool {
	return true
}

// Hash returns a function of x such that Equals(x, y) => Hash(x) == Hash(y).
// Hash may fail if the value's type is not hashable, or if the value
// contains a non-hashable value.
func (g *GoStruct) Hash() (uint32, error) {
	return 0, errors.New("starlight_struct is not hashable")
}

func (g *GoStruct) tagName() string {
	if g.tag == "" {
		return DefaultPropertyTag
	}
	return g.tag
}

func extractTagOrFieldName(f reflect.StructField, tagName string) (name string, found bool) {
	if f.PkgPath != "" {
		// Skip unexported fields
		return
	}

	var tag string
	if tagName != "" {
		// get the tag value by name
		tag = f.Tag.Get(tagName)

		// Split tag into name and options
		tagParts := strings.SplitN(tag, ",", 2)
		if len(tagParts) > 1 {
			// The actual tag name is the first part before the comma.
			tag = tagParts[0]
		}

		// Skip fields with tag "-"
		if tag == "-" {
			return
		}
	}
	if tag == "" {
		// If both custom and default tag name are empty, just use the field name
		// If no related tag is defined or as empty, use the field name
		name = f.Name
	} else {
		name = tag
	}
	found = true
	return
}
