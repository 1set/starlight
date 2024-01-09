package convert_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
	"go.starlark.net/starlark"
)

type nested struct {
	Truth  bool
	Name   string  `star:"name"`
	Number int     `star:"num,omitempty,nil"`
	Value  float64 `star:"-"`
}

type mega struct {
	Bool     bool
	Int      int
	Int64    int64 `star:"hate,omitempty,null"`
	Body     io.Reader
	String   string `star:"love"`
	Map      map[string]string
	Time     time.Time
	Now      func() time.Time
	Bytes    []byte
	Child    nested    `star:"children"`
	Change   *nested   `star:"change"`
	Another  *nested   `star:"another"`
	Multiple []*nested `star:"many"`
}

func (m *mega) GetTime() time.Time {
	return m.Time
}

func (m *mega) getBool() bool {
	return m.Bool
}

func TestStructs(t *testing.T) {
	m := &mega{
		Bool:  true,
		Int:   1,
		Int64: 2,
		Body:  strings.NewReader("hi!"),
		Map:   map[string]string{"foo": "bar"},
		Time:  time.Now(),
		Now:   time.Now,
		Bytes: []byte("hi!"),
	}
	globals := map[string]interface{}{
		"m":          m,
		"assert":     &assert{t: t},
		"bytesEqual": bytes.Equal,
		"readAll":    ioutil.ReadAll,
	}

	code := []byte(`
assert.Eq(m.Bool, True)
assert.Eq(m.Int, 1)
assert.Eq(m.Int64, 2)
assert.Eq(m.Map["foo"], "bar")
assert.Eq(m.Time.year, m.Now().year)
assert.Eq(m.GetTime().year, m.Now().year)
assert.Eq(True, bytesEqual(readAll(m.Body), m.Bytes))
`)

	_, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}

	code = []byte(`m.Bool = 200`)
	_, err = starlight.Eval(code, globals, nil)
	expectErr(t, err, "value of type int64 cannot be converted to type bool")

	code = []byte(`m.Int = None`)
	_, err = starlight.Eval(code, globals, nil)
	expectErr(t, err, "value of type None cannot be converted to non-nullable type int")
}

func TestCannotCallUnexported(t *testing.T) {
	code := []byte(`
a = m.getBool()
`)
	globals := map[string]interface{}{
		"m": &mega{},
	}
	_, err := starlight.Eval(code, globals, nil)
	expectErr(t, err, "starlight_struct<*convert_test.mega> has no .getBool field or method (did you mean .Bool?)")
}

func TestStructWithCustomTag(t *testing.T) {
	m := &mega{
		String: "hi!",
		Int64:  100,
		Child: nested{
			Truth:  true,
			Name:   "alice",
			Number: 100,
			Value:  1.8,
		},
		Change: &nested{
			Truth:  false,
			Name:   "bob",
			Number: 200,
			Value:  2.8,
		},
	}
	globals := map[string]interface{}{
		"m": convert.NewStructWithTag(m, "star"),
	}
	code := []byte(`
a = m.love
b = m.hate
m.love = "bye!"
m.hate = 60
print(dir(m), dir(m.children), dir(m.change))
c = m.children.Truth
d = m.children.name
m.change.num = 100
e = dir(m.children) == dir(m.change)
m.another = m.change
f = dir(m.another) == dir(m.change)
`)
	res, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.String != "bye!" {
		t.Fatalf("expected m.String to be 'bye!', but got %q", m.String)
	}
	if a := res["a"].(string); a != "hi!" {
		t.Fatalf("expected a to be 'hi!', but got %q", a)
	}
	if b := res["b"].(int64); b != 100 {
		t.Fatalf("expected b to be 100, but got %d", b)
	}
	if c := res["c"].(bool); c != true {
		t.Fatalf("expected c to be true, but got %v", c)
	}
	if d := res["d"].(string); d != "alice" {
		t.Fatalf("expected d to be 'alice', but got %q", d)
	}
	if e := res["e"].(bool); e != true {
		t.Fatalf("expected e to be true, but got %v", e)
	}
	if f := res["f"].(bool); f != true {
		t.Fatalf("expected f to be true, but got %v", f)
	}
}

func TestStructAddressable(t *testing.T) {
	s := customStruct{
		Name:  "Static",
		Value: 10,
	}
	p := &customStruct{
		Name:  "Pointer",
		Value: 20,
	}
	globals := map[string]starlark.Value{
		"s": convert.NewStruct(s),
		"p": convert.NewStruct(p),
	}

	// for struct string
	code1 := `
s.Name = "More"
`
	if m1, err := execStarlark(code1, globals); err == nil {
		t.Errorf("expected error for non-addressable struct string value")
	} else {
		t.Logf("verify s: %v - %v", s, m1)
	}

	// for struct int
	code2 := `
s.Value += 1
`
	if m2, err := execStarlark(code2, globals); err == nil {
		t.Errorf("expected error for non-addressable struct int value")
	} else {
		t.Logf("verify s: %v - %v", s, m2)
	}

	// for pointer string
	code3 := `
p.Name = "More"
`
	if m3, err := execStarlark(code3, globals); err != nil {
		t.Errorf("unexpected error for addressable pointer string value: %v", err)
	} else {
		t.Logf("verify p: %v - %v", p, m3)
	}

	// for pointer int
	code4 := `
p.Value += 1
`
	if m4, err := execStarlark(code4, globals); err != nil {
		t.Errorf("unexpected error for addressable pointer int value: %v", err)
	} else {
		t.Logf("verify p: %v - %v", p, m4)
	}
}
