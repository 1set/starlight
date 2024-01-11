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
	Child    nested             `star:"children"`
	Change   *nested            `star:"change"`
	Another  *nested            `star:"another"`
	Multiple []*nested          `star:"many"`
	MoreMap  map[string]*nested `star:"more"`
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
		Multiple: []*nested{
			{
				Truth:  true,
				Name:   "one",
				Number: 1,
			},
			{
				Truth:  true,
				Name:   "two",
				Number: 2,
			},
		},
		MoreMap: map[string]*nested{
			"one": {
				Truth:  true,
				Name:   "I",
				Number: 1,
			},
			"two": {
				Truth:  true,
				Name:   "II",
				Number: 2,
			},
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
print(dir(m), dir(m.children), dir(m.change), dir(m.many[0]))
print(m.more["two"])

c = m.children.name
d = m.children.Truth

m.change.num = 100
e = dir(m.children) == dir(m.change)
m.another = m.change
f = dir(m.another) == dir(m.change)

g = len(m.many) == 2
m1 = m.many[0]
h = dir(m1) == ["Truth", "name", "num"]

i = len(m.more) == 2
m2 = m.more["one"]
j = dir(m2) == ["Truth", "name", "num"]
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
	if d := res["c"].(string); d != "alice" {
		t.Fatalf("expected d to be 'alice', but got %q", d)
	}

	boolChecks := []string{"d", "e", "f", "g", "h", "i", "j"}
	for _, label := range boolChecks {
		if v := res[label].(bool); v != true {
			t.Fatalf("expected %s to be true, but got %v", label, v)
		}
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

func TestMakeStringDict(t *testing.T) {
	type contact struct {
		Name   string `sl:"name"`
		Street string `sl:"address,omitempty"`
	}
	type testCase struct {
		name        string
		globals     map[string]interface{}
		codeSnippet string
		customTag   string
		wantErrConv bool
		wantErrExec bool
	}
	testCases := []testCase{
		{
			name: "simple",
			globals: map[string]interface{}{
				"a": 1,
				"b": "foo",
				"c": map[string]int{"a": 100, "b": 200},
			},
			codeSnippet: `
assert.Eq(a, 1)
assert.Eq(b, "foo")
assert.Eq(c["a"], 100)
assert.Eq(c["b"], 200)
assert.Eq(type(c), "starlight_map<map[string]int>")
`,
		},
		{
			name: "struct",
			globals: map[string]interface{}{
				"a": &contact{Name: "bob", Street: "oak"},
			},
			codeSnippet: `
assert.Eq(a.Name, "bob")
assert.Eq(a.Street, "oak")
assert.Eq(type(a), "starlight_struct<*convert_test.contact>")
assert.Eq(dir(a), ["Name", "Street"])
`,
		},
		{
			name: "struct with custom tag",
			globals: map[string]interface{}{
				"a": &contact{Name: "bob", Street: "oak"},
			},
			customTag: `sl`,
			codeSnippet: `
assert.Eq(a.name, "bob")
assert.Eq(a.address, "oak")
assert.Eq(type(a), "starlight_struct<*convert_test.contact>")
assert.Eq(dir(a), ["address", "name"])
`,
		},
		{
			name: "struct with custom tag and no value",
			globals: map[string]interface{}{
				"a": &contact{Name: "bob"},
			},
			customTag: `sl`,
			codeSnippet: `
assert.Eq(a.name, "bob")
assert.Eq(a.address, "")
assert.Eq(type(a), "starlight_struct<*convert_test.contact>")
assert.Eq(dir(a), ["address", "name"])
`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gs := tc.globals
			gs["assert"] = &assert{t: t}

			// convert go values to Starlark values as predefined globals
			var (
				env     starlark.StringDict
				errConv error
			)
			if tc.customTag != "" {
				env, errConv = convert.MakeStringDictWithTag(gs, tc.customTag)
			} else {
				env, errConv = convert.MakeStringDict(gs)
			}
			if errConv != nil == !tc.wantErrConv {
				t.Fatalf(`expected no error while converting globals, but got %v`, errConv)
			} else if errConv == nil && tc.wantErrConv {
				t.Fatalf(`expected an error while converting globals, but got none`)
			}
			if errConv != nil {
				return
			}

			// run the Starlark code to test the converted globals
			_, errExec := execStarlark(tc.codeSnippet, env)
			if errExec != nil && !tc.wantErrExec {
				t.Fatalf(`expected no error while executing code snippet, but got %v`, errExec)
			} else if errExec == nil && tc.wantErrExec {
				t.Fatalf(`expected an error while executing code snippet, but got none`)
			}
		})
	}
}
