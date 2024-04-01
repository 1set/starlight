package convert_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
	"go.starlark.net/starlark"
)

// Helper function to execute a Starlark script with given global functions and data
func execStarlark(script string, envs map[string]starlark.Value) (map[string]interface{}, error) {
	thread := &starlark.Thread{
		Print: func(_ *starlark.Thread, msg string) { fmt.Println("❤", msg) },
	}

	data := []byte(script)
	globals, err := starlark.ExecFile(thread, "foo.star", data, envs)
	if err != nil {
		return nil, err
	}

	return convert.FromStringDict(globals), nil
}

func TestVariadic(t *testing.T) {
	gm := map[string]string{
		"key": "value",
	}
	t.Logf("Go Map Print: %v", gm)

	globals := map[string]interface{}{
		"sprint":  fmt.Sprint,
		"fatal":   t.Fatal,
		"sprintf": fmt.Sprintf,
	}

	code := []byte(`
def do(): 
	v = sprint(False)
	if v != "false" :
		fatal("unexpected output1: ", v)
	v = sprint(False, 1)
	if v != "false 1" :
		fatal("unexpected output2:", v)
	v = sprint(False, 1, " hi ", {"key":"value"})
	if v != 'false 1 hi map[key:value]' :
		fatal("unexpected output3:", v)
	v = sprintf("this is your %dst formatted message", 1)
	if v != "this is your 1st formatted message":
		fatal("unexpected output4:", v)
do()
`)

	_, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMakeStarFnOneRet(t *testing.T) {
	fn := func(s string) string {
		return "hi " + s
	}

	skyf := convert.MakeStarFn("boo", fn)
	// Mental note: starlark numbers pop out as int64s
	data := []byte(`
a = boo("starlight")
`)

	thread := &starlark.Thread{
		Print: func(_ *starlark.Thread, msg string) { fmt.Println(msg) },
	}

	globals := map[string]starlark.Value{
		"boo": skyf,
	}
	globals, err := starlark.ExecFile(thread, "foo.star", data, globals)
	if err != nil {
		t.Fatal(err)
	}
	v := convert.FromStringDict(globals)
	if v["a"] != "hi starlight" {
		t.Fatalf(`expected a = "hi starlight", but got %#v`, v["a"])
	}
}

// Test a function with no return value
func TestMakeStarFnNoRet(t *testing.T) {
	fn := func(s string) {
		fmt.Println("hi " + s)
	}

	skyf := convert.MakeStarFn("boo", fn)

	globals := map[string]starlark.Value{
		"boo": skyf,
	}

	_, err := execStarlark(`boo("starlight")`, globals)
	if err != nil {
		t.Fatal(err)
	}
}

// Test a function with one non-error return value
func TestMakeStarFnOneRetNonError(t *testing.T) {
	fn := func(s string) string {
		return "hi " + s
	}

	skyf := convert.MakeStarFn("boo", fn)

	globals := map[string]starlark.Value{
		"boo": skyf,
	}

	v, err := execStarlark(`a = boo("starlight")`, globals)
	if err != nil {
		t.Fatal(err)
	}

	if v["a"] != "hi starlight" {
		t.Fatalf(`expected a = "hi starlight", but got %#v`, v["a"])
	}
}

// Test a function with one error return value
func TestMakeStarFnOneRetError(t *testing.T) {
	fn := func(s string) error {
		if s == "error" {
			return fmt.Errorf("error occurred")
		}
		return nil
	}

	skyf := convert.MakeStarFn("boo", fn)

	globals := map[string]starlark.Value{
		"boo": skyf,
	}

	if _, err := execStarlark(`err = boo("wtf")`, globals); err != nil {
		t.Fatalf(`expected no err, but got err: %v`, err)
	}
	if v, err := execStarlark(`err = boo("error")`, globals); err == nil {
		t.Fatalf(`expected err = "error occurred", but got no err: %v`, v)
	}
}

// Test a function with two non-error return values
func TestMakeStarFnTwoRetNonError(t *testing.T) {
	fn := func(s string) (string, string) {
		return "hi " + s, "bye " + s
	}
	skyf := convert.MakeStarFn("boo", fn)
	globals := map[string]starlark.Value{
		"boo": skyf,
	}

	v, err := execStarlark(`a, b = boo("starlight")`, globals)
	if err != nil {
		t.Fatal(err)
	}

	if v["a"] != "hi starlight" || v["b"] != "bye starlight" {
		t.Fatalf(`expected a = "hi starlight", b = "bye starlight", but got a=%#v, b=%#v`, v["a"], v["b"])
	}
}

// Test a function with one non-error return value and one error return value
func TestMakeStarFnOneRetNonErrorAndError(t *testing.T) {
	fn := func(s string) (string, error) {
		if s == "error" {
			return "", fmt.Errorf("error occurred")
		}
		return "hi " + s, nil
	}

	skyf := convert.MakeStarFn("boo", fn)

	globals := map[string]starlark.Value{
		"boo": skyf,
	}

	if v, err := execStarlark(`a = boo("starlight")`, globals); err != nil {
		t.Fatalf(`expected a = "hi starlight", err = nil, but got a=%v, err=%v`, v, err)
	}
	if v, err := execStarlark(`a = boo("error")`, globals); err == nil {
		t.Fatalf(`expected err = "error occurred", but got no err: a=%v`, v)
	}
}

func TestMakeStarFnOneRetErrorAndNonError(t *testing.T) {
	fn := func(s string) (error, string) {
		if s == "" {
			return errors.New("input is empty"), ""
		}
		return nil, "hi " + s
	}

	skyf := convert.MakeStarFn("boo", fn)

	globals := map[string]starlark.Value{
		"boo": skyf,
	}

	if v, err := execStarlark(`e, a = boo("")`, globals); err != nil {
		t.Fatalf(`expected a = "", err = "input is empty", but got a=%v, err=%v`, v, err)
	} else if e := v["e"]; e == nil {
		t.Fatalf(`expected e = "input is empty", but got e=nil`)
	}
}

func TestMakeStarFnOneRetTwoNonErrorAndError(t *testing.T) {
	fn := func(s string, n int) (string, int, error) {
		if s == "" {
			return "", 0, errors.New("input is empty")
		}
		return "hi " + s, n + 5, nil
	}

	skyf := convert.MakeStarFn("boo", fn)

	globals := map[string]starlark.Value{
		"boo": skyf,
	}

	if v, err := execStarlark(`a, b = boo("", 5)`, globals); err == nil {
		t.Fatalf(`expected a = "", b = 0, err = "input is empty", but got a=%v, b=%v, err=%v`, v["a"], v["b"], err)
	}
	if v, err := execStarlark(`a, b = boo("starlight", 5)`, globals); err != nil {
		t.Fatalf(`expected a = "hi starlight", b = 10, err = nil, but got a=%v, b=%v, err=%v`, v["a"], v["b"], err)
	}
}

func TestMakeStarFnCustomTag(t *testing.T) {
	type contact struct {
		Name   string `sl:"name"`
		Street string `sl:"address,omitempty"`
	}
	type profile struct {
		NickName string `star:"nickname"`
		Location string `star:"location"`
	}
	fn := func(n, s string) (*contact, *profile) {
		return &contact{
				Name:   n,
				Street: s,
			}, &profile{
				NickName: n,
				Location: s,
			}
	}
	tag := "sl"
	skyf, err := convert.ToValueWithTag(fn, tag)
	if err != nil {
		t.Errorf("Unexpected error for function conversion: %v", err)
	}
	asrt, err := convert.ToValueWithTag(&assert{t: t}, tag)
	if err != nil {
		t.Errorf("Unexpected error for assert conversion: %v", err)
	}

	globals := map[string]starlark.Value{
		"boo":    skyf,
		"assert": asrt,
	}
	code1 := `
dc, dp = boo("a", "b")
assert.Eq("a", dc.name)
assert.Eq("b", dc.address)
assert.Eq("a", dp.NickName)
assert.Eq("b", dp.Location)
`
	if _, err := execStarlark(code1, globals); err != nil {
		t.Errorf("Unexpected error for runtime: %v", err)
	}
}

func TestMakeStarFnSlice(t *testing.T) {
	fn := func(s1 []string, s2 []int) (int, string, error) {
		cnt := 10
		if len(s1) != 2 || s1[0] != "hello" || s1[1] != "world" {
			return 0, "", errors.New("incorrect slice input1")
		}
		if len(s2) != 2 || s2[0] != 1 || s2[1] != 2 {
			return 0, "", errors.New("incorrect slice input2")
		}

		// TODO: nested slice like [["slice", "test"], ["hello", "world"]], [[[1, 2]]]) is not supported yet
		return cnt, "hey!", nil
	}

	skyf := convert.MakeStarFn("boo", fn)

	data := []byte(`
a = boo(["hello", "world"], [1, 2])
b = 0.1
    `)

	thread := &starlark.Thread{
		Print: func(_ *starlark.Thread, msg string) { fmt.Println(msg) },
	}

	globals := map[string]starlark.Value{
		"boo": skyf,
	}
	globals, err := starlark.ExecFile(thread, "foo.star", data, globals)
	if err != nil {
		t.Fatal(err)
	}
	v := convert.FromStringDict(globals)
	if !reflect.DeepEqual(v["a"], []interface{}{int64(10), "hey!"}) {
		t.Fatalf(`expected a = [10, "hey!"], but got %#v`, v)
	}
}

func TestMakeStarFnMap(t *testing.T) {
	fn := func(m1 map[string]int32, m2 map[string]int, m3 map[string]float32, m4 map[uint8]uint64, m5 map[int16]int8) (int, string, error) {
		cnt := int32(0)
		for k, v := range m1 {
			if k == "hello" && v == 1 {
				cnt += v
			} else if k == "world" && v == 2 {
				cnt += v
			} else {
				return 0, "", errors.New("incorrect map input1")
			}
		}
		for range m2 {
			cnt += 1
		}
		for range m3 {
			cnt += 1
		}
		for range m4 {
			cnt += 1
		}
		for range m5 {
			cnt += 1
		}

		// TODO: nested map like map[int16][]int8 {1000: [1, 2, 3]} is not supported yet
		return int(cnt), "hey!", nil
	}

	skyf := convert.MakeStarFn("boo", fn)

	data := []byte(`
a = boo({"hello": 1, "world": 2}, {"int": 100}, {"float32": 0.1}, {10: 5}, {1000: 100})
b = 0.1
    `)

	thread := &starlark.Thread{
		Print: func(_ *starlark.Thread, msg string) { fmt.Println(msg) },
	}

	globals := map[string]starlark.Value{
		"boo": skyf,
	}
	globals, err := starlark.ExecFile(thread, "foo.star", data, globals)
	if err != nil {
		t.Fatal(err)
	}
	v := convert.FromStringDict(globals)
	if !reflect.DeepEqual(v["a"], []interface{}{int64(7), "hey!"}) {
		t.Fatalf(`expected a = [7, "hey!"], but got %#v`, v)
	}
}

func TestMakeStarFnArgumentType(t *testing.T) {
	type testCase struct {
		name          string
		funcToConvert interface{}
		valToPass     interface{}
		codeSnippet   string
		shouldPanic   bool
		wantErr       bool
	}
	testCases := []testCase{
		{
			name:          "Nil",
			funcToConvert: nil,
			codeSnippet:   `x = boo()`,
			shouldPanic:   true,
		},
		{
			name:          "Non-function type",
			funcToConvert: 123,
			codeSnippet:   `x = boo()`,
			shouldPanic:   true,
		},
		{
			name:          "Call with less argument count",
			funcToConvert: func(a int, b int) {},
			codeSnippet:   `x = boo(12)`,
			wantErr:       true,
		},
		{
			name:          "Call with more argument count",
			funcToConvert: func(a int, b int) {},
			codeSnippet:   `x = boo(12, 34, 56)`,
			wantErr:       true,
		},
		{
			name: "Call with variadic argument",
			funcToConvert: func(a int, b ...int) int {
				return len(b)
			},
			codeSnippet: `x = boo(12)`,
		},
		{
			name: "Call with more variadic argument",
			funcToConvert: func(a int, b ...int) int {
				return len(b)
			},
			codeSnippet: `x = boo(12, 34, 56)`,
		},
		{
			name: "Call with wrong variadic argument type",
			funcToConvert: func(a int, b ...int) int {
				return len(b)
			},
			codeSnippet: `x = boo(12, "hello")`,
			wantErr:     true,
		},
		{
			name:          "Call with wrong argument type",
			funcToConvert: func(a int) {},
			codeSnippet:   `x = boo("hello")`,
			wantErr:       true,
		},
		{
			name:          "Call with wrong argument type 2",
			funcToConvert: func(a int) int { return 0 },
			codeSnippet:   `x = boo(["a", "b"])`,
			wantErr:       true,
		},
		{
			name: "Call with slice argument",
			funcToConvert: func(a []string) int {
				return len(a)
			},
			codeSnippet: `x = boo(["a", "b"])`,
		},
		{
			name: "Call with wrong slice argument",
			funcToConvert: func(a []int) int {
				t.Logf("[Go⭐️] %v", a)
				return len(a)
			},
			codeSnippet: `x = boo(["123", "456"])`,
			wantErr:     true,
		},
		{
			name: "Call with non-matched slice argument",
			funcToConvert: func(a []struct{}) int {
				t.Logf("[Go⭐️] %v", a)
				return len(a)
			},
			codeSnippet: `x = boo(["123", "456"])`,
			wantErr:     true,
		},
		{
			name: "Call for nested slice argument (not handle yet)",
			funcToConvert: func(a [][]string) int {
				return len(a)
			},
			codeSnippet: `x = boo([["a", "b"]])`,
			wantErr:     true,
		},
		{
			name: "Call for nested slice argument 2 (not handle yet)",
			funcToConvert: func(a []map[int]int) int {
				return len(a)
			},
			codeSnippet: `x = boo([{1: 2}])`,
			wantErr:     true,
		},
		{
			name: "Call with map interface argument",
			funcToConvert: func(a map[string]interface{}) int {
				for k, v := range a {
					t.Logf("[Go⭐️] %#v: %#v (%T)", k, v, v)
				}
				return len(a)
			},
			codeSnippet: `x = boo({"a": 1, "b": True, "c": [1,2,3]})`,
		},
		{
			name: "Call with map typed argument",
			funcToConvert: func(a map[string]int) int {
				return len(a)
			},
			codeSnippet: `x = boo({"a": 1, "b": 2})`,
		},
		{
			name: "Call with map typed argument using starlark.Value",
			funcToConvert: func(a map[string]int8) int {
				return len(a)
			},
			valToPass:   100,
			codeSnippet: `x = boo({"a": val, "b": val})`,
		},
		{
			name: "Call with map starlark interface argument",
			funcToConvert: func(a map[string]starlark.Value) int {
				return len(a)
			},
			codeSnippet: `x = boo({"a": 1, "b": 2})`,
			wantErr:     true,
		},
		{
			name: "Call with map starlark int argument",
			funcToConvert: func(a map[string]starlark.Int) int {
				return len(a)
			},
			codeSnippet: `x = boo({"a": 1, "b": 2})`,
			wantErr:     true,
		},
		{
			name: "Call with nested map argument (not handle yet)",
			funcToConvert: func(a map[string]map[string]int) int {
				return len(a)
			},
			codeSnippet: `x = boo({"a": {"b": 1}})`,
			wantErr:     true,
		},
		{
			name: "Call with nested map argument (not handle yet)",
			funcToConvert: func(a map[string]map[string]int) int {
				return len(a)
			},
			codeSnippet: `x = boo({"a": {"b": 1}})`,
			wantErr:     true,
		},
		{
			name: "Call with nested map argument 2 (not handle yet)",
			funcToConvert: func(a map[string][]int) int {
				return len(a)
			},
			codeSnippet: `x = boo({"a": [1, 2, 3]})`,
			wantErr:     true,
		},
		{
			name: "Call with nested map argument 3 (not handle yet)",
			funcToConvert: func(a map[string][]int) int {
				return len(a)
			},
			valToPass:   starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.MakeInt(2), starlark.MakeInt(3)}),
			codeSnippet: `x = boo({"a": val})`,
			wantErr:     true,
		},
		{
			name: "Call with nested map argument with goslice",
			funcToConvert: func(a map[string][]int) int {
				return len(a)
			},
			valToPass:   []int{1, 2, 3},
			codeSnippet: `x = boo({"a": val})`,
		},
		{
			name: "Call with mistyped map argument",
			funcToConvert: func(a map[string]string) int {
				return len(a)
			},
			codeSnippet: `x = boo({"a": [1, 2, 3]})`,
			wantErr:     true,
		},
		{
			name: "Call with mistyped set argument",
			funcToConvert: func(a map[string]struct{}) int {
				return len(a)
			},
			codeSnippet: `x = boo(set(["a", "B"]))`,
			wantErr:     true,
		},
		{
			name: "Call with mapped set argument",
			funcToConvert: func(a map[string]bool) int {
				return len(a)
			},
			codeSnippet: `x = boo(set(["a", "B"]))`,
		},
		{
			name: "Call with func argument",
			funcToConvert: func(a func(int) int) int {
				return a(10)
			},
			codeSnippet: `x = boo(lambda x: x * 2)`,
			wantErr:     true,
		},
		{
			name: "Call with slice for array argument (not handle yet)",
			funcToConvert: func(a [5]int) int {
				return len(a)
			},
			codeSnippet: `x = boo([1, 2, 3, 4, 5])`,
			wantErr:     true,
		},
		{
			name: "Call with various arguments",
			funcToConvert: func(s string, i int64, b bool, f float64, ss []string, m map[string]int) (int, string, error) {
				if len(ss) != 2 || ss[0] != "slice" || ss[1] != "test" {
					return 0, "", errors.New("incorrect slice input")
				}

				if len(m) != 1 || m["key"] != 10 {
					return 0, "", errors.New("incorrect map input")
				}

				return 5, "hi!", nil
			},
			codeSnippet: `x = boo("a", 1, True, 0.1, ["slice", "test"], {"key": 10})`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					if !tc.shouldPanic {
						err := r.(error)
						t.Errorf("Unexpected panic: %v", err)
					}
				}
			}()

			starFn := convert.MakeStarFn("testFn", tc.funcToConvert)

			// Use this function in a Starlark script
			thread := &starlark.Thread{
				Name: "my thread",
				Print: func(_ *starlark.Thread, msg string) {
					t.Log("[Starlark⭐️]", msg)
				},
			}
			script := tc.codeSnippet
			globals := starlark.StringDict{
				"boo": starFn,
			}

			// For additional values
			if tc.valToPass != nil {
				sv, err := convert.ToValue(tc.valToPass)
				if err != nil {
					t.Errorf("Unexpected error for conversion: %v", err)
					return
				}
				globals["val"] = sv
			}

			_, err := starlark.ExecFile(thread, "script.star", script, globals)
			if tc.wantErr && err == nil {
				t.Errorf("Expected error, but got none")
			} else if !tc.wantErr && err != nil {
				t.Errorf("Unexpected error for runtime: %v", err)
			}
		})
	}
}
