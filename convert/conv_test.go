package convert

import (
	"fmt"
	"reflect"
	"testing"

	"go.starlark.net/starlark"
)

func TestMakeTuple(t *testing.T) {
	tuple1, err := MakeTuple(nil)
	if err != nil {
		t.Errorf("unexpected error 1: %v", err)
		return
	}
	tuple2, err := MakeTuple([]interface{}{"a", 1, true, 0.1})
	if err != nil {
		t.Errorf("unexpected error 2: %v", err)
		return
	}
	if _, err = MakeTuple([]interface{}{make(chan int)}); err == nil {
		t.Errorf("expected error 3, got nil")
		return
	}

	globals := map[string]starlark.Value{
		"tuple_empty": tuple1,
		"tuple_has":   tuple2,
	}
	code := `
c1 = len(tuple_empty)
c2 = len(tuple_has)
t1 = type(tuple_empty)
t2 = type(tuple_has)
t2a = type(tuple_has[0])
t2b = type(tuple_has[1])
t2c = type(tuple_has[2])
t2d = type(tuple_has[3])
`
	expRes := starlark.StringDict{
		"c1":  starlark.MakeInt(0),
		"c2":  starlark.MakeInt(4),
		"t1":  starlark.String("tuple"),
		"t2":  starlark.String("tuple"),
		"t2a": starlark.String("string"),
		"t2b": starlark.String("int"),
		"t2c": starlark.String("bool"),
		"t2d": starlark.String("float"),
	}
	res, err := starlark.ExecFile(&starlark.Thread{}, "foo.star", []byte(code), globals)
	if err != nil {
		t.Errorf("unexpected error to exec: %v", err)
		return
	}
	if !reflect.DeepEqual(res, expRes) {
		t.Errorf("expected %v, got %v", expRes, res)
	}
}

func TestFromTuple(t *testing.T) {
	code := `
t0 = ()
t1 = (10,)
t2 = ("a", 1, True, 0.1)
`
	expRes := map[string][]interface{}{
		"t0": {},
		"t1": {int64(10)},
		"t2": {"a", int64(1), true, 0.1},
	}
	res, err := starlark.ExecFile(&starlark.Thread{}, "foo.star", []byte(code), nil)
	if err != nil {
		t.Errorf("unexpected error to exec: %v", err)
		return
	}
	actRes := map[string][]interface{}{}
	for k, v := range res {
		actRes[k] = FromTuple(v.(starlark.Tuple))
	}
	if !reflect.DeepEqual(actRes, expRes) {
		t.Errorf("expected %v, got %v", expRes, res)
	}
}

func TestMakeList(t *testing.T) {
	list1, err := MakeList(nil)
	if err != nil {
		t.Errorf("unexpected error 1: %v", err)
		return
	}
	list2, err := MakeList([]interface{}{"a", 1, true, 0.1})
	if err != nil {
		t.Errorf("unexpected error 2: %v", err)
		return
	}
	if _, err = MakeList([]interface{}{make(chan int)}); err == nil {
		t.Errorf("expected error 3, got nil")
		return
	}
	globals := map[string]starlark.Value{
		"list_empty": list1,
		"list_has":   list2,
	}
	code := `
c1 = len(list_empty)
c2 = len(list_has)
t1 = type(list_empty)
t2 = type(list_has)
t2a = type(list_has[0])
t2b = type(list_has[1])
t2c = type(list_has[2])
t2d = type(list_has[3])
`
	expRes := starlark.StringDict{
		"c1":  starlark.MakeInt(0),
		"c2":  starlark.MakeInt(4),
		"t1":  starlark.String("list"),
		"t2":  starlark.String("list"),
		"t2a": starlark.String("string"),
		"t2b": starlark.String("int"),
		"t2c": starlark.String("bool"),
		"t2d": starlark.String("float"),
	}
	res, err := starlark.ExecFile(&starlark.Thread{}, "foo.star", []byte(code), globals)
	if err != nil {
		t.Errorf("unexpected error to exec: %v", err)
		return
	}
	if !reflect.DeepEqual(res, expRes) {
		t.Errorf("expected %v, got %v", expRes, res)
	}
}

func TestMakeSet(t *testing.T) {
	if _, err := MakeSet(nil); err != nil {
		t.Errorf("unexpected error 1: %v", err)
		return
	}
	if _, err := MakeSet(map[interface{}]bool{"a": true, 1: true, true: true, 0.1: true}); err != nil {
		t.Errorf("unexpected error 2: %v", err)
		return
	}
	if _, err := MakeSet(map[interface{}]bool{make(chan int): true}); err == nil {
		t.Errorf("expected error 3, got nil")
		return
	}
}

func TestMakeSetFromSlice(t *testing.T) {
	set1, err := MakeSetFromSlice(nil)
	if err != nil {
		t.Errorf("unexpected error 1: %v", err)
		return
	}
	set2, err := MakeSetFromSlice([]interface{}{"a", 1, true, 0.1})
	if err != nil {
		t.Errorf("unexpected error 2: %v", err)
		return
	}
	if _, err = MakeSetFromSlice([]interface{}{make(chan int)}); err == nil {
		t.Errorf("expected error 3, got nil")
		return
	}
	if _, err = MakeSetFromSlice([]interface{}{[]int{1, 2}}); err == nil {
		t.Errorf("expected error 4, got nil")
		return
	}
	globals := map[string]starlark.Value{
		"set_empty": set1,
		"set_has":   set2,
	}
	code := `
c1 = len(set_empty)
c2 = len(set_has)
t1 = type(set_empty)
t2 = type(set_has)
a = all([x in set_has for x in ["a", 1, True, 0.1]])
`
	expRes := starlark.StringDict{
		"c1": starlark.MakeInt(0),
		"c2": starlark.MakeInt(4),
		"t1": starlark.String("set"),
		"t2": starlark.String("set"),
		"a":  starlark.True,
	}
	res, err := starlark.ExecFile(&starlark.Thread{}, "foo.star", []byte(code), globals)
	if err != nil {
		t.Errorf("unexpected error to exec: %v", err)
		return
	}
	if !reflect.DeepEqual(res, expRes) {
		t.Errorf("expected %v, got %v", expRes, res)
	}
}

func TestKwargs(t *testing.T) {
	// Mental note: starlark numbers pop out as int64s
	data := []byte(`
func("a", 1, foo=1, bar=2)
`)

	thread := &starlark.Thread{
		Print: func(_ *starlark.Thread, msg string) { fmt.Println(msg) },
	}

	var goargs []interface{}
	var gokwargs []Kwarg

	fn := func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var err error
		goargs = FromTuple(args)
		gokwargs, err = FromKwargs(kwargs)
		if err != nil {
			return starlark.None, err
		}
		return starlark.None, nil
	}

	globals := map[string]starlark.Value{
		"func": starlark.NewBuiltin("func", fn),
	}
	_, err := starlark.ExecFile(thread, "foo.star", data, globals)
	if err != nil {
		t.Fatal(err)
	}

	expArgs := []interface{}{"a", int64(1)}
	if len(expArgs) != len(goargs) {
		t.Fatalf("expected %d args, but got %d", len(expArgs), len(goargs))
	}
	expKwargs := []Kwarg{{Name: "foo", Value: int64(1)}, {Name: "bar", Value: int64(2)}}

	if !reflect.DeepEqual(expArgs, goargs) {
		t.Errorf("expected args %#v, got args %#v", expArgs, goargs)
	}

	if !reflect.DeepEqual(expKwargs, gokwargs) {
		t.Fatalf("expected kwargs %#v, but got %#v", expKwargs, gokwargs)
	}
}

func TestMakeStarFn(t *testing.T) {
	fn := func(s string, i int64, b bool, f float64) (int, string, error) {
		return 5, "hi!", nil
	}

	skyf := MakeStarFn("boo", fn)
	// Mental note: starlark numbers pop out as int64s
	data := []byte(`
a = boo("a", 1, True, 0.1)
b = 0.1
	`)

	thread := &starlark.Thread{
		Print: func(_ *starlark.Thread, msg string) { fmt.Println(msg) },
	}

	envs := map[string]starlark.Value{
		"boo": skyf,
	}
	globals, err := starlark.ExecFile(thread, "foo.star", data, envs)
	if err != nil {
		t.Fatal(err)
	}
	v := FromStringDict(globals)
	if !reflect.DeepEqual(v["a"], []interface{}{int64(5), "hi!"}) {
		t.Fatalf(`expected a = [5, "hi!"], but got %#v`, v)
	}
}

func TestStructToValue(t *testing.T) {
	type contact struct {
		Name, Street string
	}
	c := &contact{Name: "bob", Street: "oak"}

	s := NewStruct(c)
	v, err := ToValue(s)
	if err != nil {
		t.Fatal(err)
	}
	_, ok := v.(*GoStruct)
	if !ok {
		t.Fatalf("expected v to be *Struct, but was %T", v)
	}
}

func TestMakeNamedList(t *testing.T) {
	type Strings []string
	v := Strings{"foo", "bar"}
	val, err := ToValue(v)
	if err != nil {
		t.Fatal(err)
	}
	_, ok := val.(*GoSlice)
	if !ok {
		t.Fatalf("value should be *GoSlice but was %T", val)
	}
}

type contact struct {
	Name        string
	age         int
	PhoneNumber string `starlark:"phone"`
	Secret      int    `starlark:"-"`
	Empty       int    `starlark:""`
}

func (c contact) Foo()  {}
func (c *contact) Bar() {}

// reflect can't find non-exported functions... but can find non-exported
// methods ¯\_(ツ)_/¯

func (c *contact) bar() {}
func (c contact) foo()  {}

func TestStructAttrNames(t *testing.T) {
	c := &contact{}
	s := NewStruct(c)
	names := s.AttrNames()
	expected := []string{"Name", "Foo", "phone", "Empty", "Bar"}
	for _, s := range names {
		if !contains(expected, s) {
			t.Errorf("output contains extra value %q", s)
		}
	}
	for _, s := range expected {
		if !contains(names, s) {
			t.Errorf("output is missing value %q", s)
		}
	}
	t.Logf("%q", names)
}

func TestStructAttr(t *testing.T) {
	c := &contact{
		Name:        "bob",
		PhoneNumber: "123",
	}
	s := NewStruct(c)
	envs := map[string]starlark.Value{
		"contact": s,
	}

	code := []byte(`
name = contact.Name
phone = contact.phone
`)

	thread := &starlark.Thread{
		Name: "test",
	}

	// read the value
	globals, err := starlark.ExecFile(thread, "foo.star", code, envs)
	if err != nil {
		t.Fatal(err)
	}
	v := FromStringDict(globals)
	if v["name"] != "bob" {
		t.Errorf("expected name to be bob, but got %q", v["name"])
	}
	if v["phone"] != "123" {
		t.Errorf("expected phone to be 123, but got %q", v["phone"])
	}

	// set the value
	code = []byte(`
contact.Name = "alice"
contact.phone = "456"
`)
	globals, err = starlark.ExecFile(thread, "foo.star", code, envs)
	if err != nil {
		t.Fatal(err)
	}
}

func contains(list []string, s string) bool {
	for _, l := range list {
		if s == l {
			return true
		}
	}
	return false
}
