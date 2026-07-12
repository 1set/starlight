package convert_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
)

func TestInterfaceNil(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = convert.MakeGoInterface(nil)
}

func TestInterfaceInvalid(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = convert.MakeGoInterface(struct {
		Foo func()
	}{})
}

func TestInterfaceStructPtr(t *testing.T) {
	type resp struct {
		Body io.Reader
	}

	r := resp{Body: strings.NewReader("hi!")}

	globals := map[string]interface{}{
		"r":       r,
		"readAll": ioutil.ReadAll,
	}

	code := []byte(`
a = readAll(r.Body)
`)
	out, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, ok := out["a"].([]byte)
	if !ok {
		t.Fatalf("failed to find output: %#v", out)
	}
	expected := []byte("hi!")
	if !bytes.Equal(expected, b) {
		t.Fatalf("expected %q, got %q", expected, b)
	}
}

type Foo int

func (f Foo) Foo() string {
	return fmt.Sprintf("Foo: %v", f)
}

func (f *Foo) PFoo() bool {
	return true
}

func toFoo(i int) Foo {
	return Foo(i)
}

func toPFoo(i int) *Foo {
	f := Foo(i)
	return &f
}

func nilPtr() *Foo {
	return nil
}

type Name string

func (n Name) Double() string {
	return string(n + n)
}

func TestInterfaceCall(t *testing.T) {
	globals := map[string]interface{}{
		"toFoo":  toFoo,
		"assert": &assert{t: t},
		"fatal":  t.Fatal,
	}

	code := []byte(`
f = toFoo(1)
assert.Eq("Foo: 1", f.Foo())
`)
	_, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestInterfacePtrCall(t *testing.T) {
	globals := map[string]interface{}{
		"toPFoo": toPFoo,
		"assert": &assert{t: t},
		"fatal":  t.Fatal,
	}

	code := []byte(`
f = toPFoo(1)
# assert.Eq(True, f.PFoo())
assert.Eq("Foo: 1", f.Foo())
`)
	_, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestInterfaceTruth(t *testing.T) {
	toName := func(s string) Name {
		return Name(s)
	}

	globals := map[string]interface{}{
		"toFoo":  toFoo,
		"toPFoo": toPFoo,
		"assert": &assert{t: t},
		"fatal":  t.Fatal,
		"nilPtr": nilPtr,
		"toName": toName,
	}

	code := []byte(`
def do():
	if not toPFoo(0):
		fatal("expected non-nil pointer type to be true")
	if toFoo(0):
		fatal("expected zero int type to be false")
	if not toFoo(1):
		fatal("expected non-zero int type to be true")
	if nilPtr():
		fatal("expected nil pointer type to be false")
	if toName(""):
		fatal("expected empty string type to be false")
	if not toName("0"):
		fatal("expected non-empty string type to be true")
do()
`)
	_, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}
}

type FFloat float64

func (f FFloat) String() string {
	return fmt.Sprint(float64(f))
}

type OK bool

func (ok OK) String() string {
	return fmt.Sprint(bool(ok))
}

type Size uint16

func (s Size) String() string {
	return fmt.Sprint(uint16(s))
}

func TestInterfaceConvert(t *testing.T) {
	toName := func(s string) Name {
		return Name(s)
	}
	toFFloat := func(f float64) FFloat {
		return FFloat(f)
	}

	toOK := func(b bool) OK {
		return OK(b)
	}

	toSize := func(u uint64) Size {
		return Size(u)
	}
	globals := map[string]interface{}{
		"toFoo":    toFoo,
		"toPFoo":   toPFoo,
		"fatal":    t.Fatal,
		"fatalf":   t.Fatalf,
		"toName":   toName,
		"toFFloat": toFFloat,
		"toOK":     toOK,
		"toSize":   toSize,
	}

	code := []byte(`
def do():
	f = toPFoo(12).toInt()
	if f != 12:
		fatalf("expected typed int to return to int")
	if "phil" != toName("phil").toString():
		fatal("expected typed string to return to string")
	if 2.2 != toFFloat(2.2).toFloat():
		fatal("expected typed float to return to float")
	if True != toOK(True).toBool():
		fatal("expected typed bool to return to bool")
	if 5 != toSize(5).toUint():
		fatal("expected typed uint to return to uint")
do()
`)
	_, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}
}

type methodInt int

func (m methodInt) Marker() {}

// TestGoInterfaceNilPointerConversions: every To* dereferences a pointer, so
// each must return a clean error on a nil typed pointer instead of panicking
// (a zero reflect.Value's Interface() panics; the error formats g.v.Type()).
func TestGoInterfaceNilPointerConversions(t *testing.T) {
	g := convert.MakeGoInterface((*methodInt)(nil))
	if _, err := g.ToInt(); err == nil {
		t.Fatal("ToInt on a nil typed pointer should error, not panic")
	}
	if _, err := g.ToBool(); err == nil {
		t.Fatal("ToBool on a nil typed pointer should error, not panic")
	}
	if _, err := g.ToUint(); err == nil {
		t.Fatal("ToUint on a nil typed pointer should error, not panic")
	}
	// ToString/ToFloat now dereference too, so they take the same nil path
	if _, err := g.ToString(); err == nil {
		t.Fatal("ToString on a nil typed pointer should error, not panic")
	}
	if _, err := g.ToFloat(); err == nil {
		t.Fatal("ToFloat on a nil typed pointer should error, not panic")
	}
}

// TestGoInterfacePointerConversions: ToString/ToFloat rejected a pointer to a
// string/float while ToInt/ToBool/ToUint dereferenced their pointer variants
// — an asymmetry. All five now dereference one pointer level.
func TestGoInterfacePointerConversions(t *testing.T) {
	name := Name("hi")
	if got, err := convert.MakeGoInterface(&name).ToString(); err != nil || got != "hi" {
		t.Errorf(`(*Name).ToString() = %q, %v; want "hi", nil`, got, err)
	}
	f := FFloat(2.5)
	if got, err := convert.MakeGoInterface(&f).ToFloat(); err != nil || got != 2.5 {
		t.Errorf("(*FFloat).ToFloat() = %v, %v; want 2.5, nil", got, err)
	}
	// the error message names the concrete Go type, not "reflect.Value"
	if _, err := convert.MakeGoInterface(&f).ToString(); err == nil || !strings.Contains(err.Error(), "FFloat") {
		t.Errorf("ToString on *FFloat should name the type, got %v", err)
	}
}

type taggedChild struct {
	Name string `custom:"nick"`
}

type childID int

func (childID) Child() taggedChild { return taggedChild{Name: "x"} }

// TestMethodResultPreservesTag: a slice/map/struct child that is a
// method-bearing named scalar became a GoInterface whose tag was dropped, so
// a struct returned by one of its methods exposed its fields under the default
// tag instead of the one the parent used. The tag must flow through both the
// child wrapper and (after slicing) the sliced copy.
func TestMethodResultPreservesTag(t *testing.T) {
	items, err := convert.ToValueWithTag([]childID{1, 2}, "custom")
	if err != nil {
		t.Fatal(err)
	}
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"items":  items,
	}
	code := []byte(`
assert.Eq(items[0].Child().nick, "x")      # element is a method-bearing scalar
assert.Eq(items[0:2][0].Child().nick, "x") # tag survives Slice, then the method result
`)
	if _, err := starlight.Eval(code, globals, nil); err != nil {
		t.Fatalf("method result lost the struct-field tag: %v", err)
	}
}

type nilSafe struct{}

// Tagged has a pointer receiver but does not dereference it, so it is callable
// on a nil *nilSafe.
func (*nilSafe) Tagged() taggedChild { return taggedChild{Name: "y"} }

// TestNilPtrReceiverChildPreservesTag: a nil pointer with a method set still
// goes through makeGoInterface (hasMethods is true for *T with pointer-receiver
// methods), so the tag fix must hold for a nil receiver too — a nil-safe method
// returning a struct exposes its fields under the parent's tag.
func TestNilPtrReceiverChildPreservesTag(t *testing.T) {
	items, err := convert.ToValueWithTag([]*nilSafe{nil}, "custom")
	if err != nil {
		t.Fatal(err)
	}
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"items":  items,
	}
	code := []byte(`assert.Eq(items[0].Tagged().nick, "y")`)
	if _, err := starlight.Eval(code, globals, nil); err != nil {
		t.Fatalf("nil pointer-receiver child lost the struct-field tag: %v", err)
	}
}
