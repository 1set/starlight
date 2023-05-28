package convert_test

import (
	"fmt"
	"testing"

	"github.com/1set/starlight/convert"
	"go.starlark.net/starlark"
)

func TestCallStarlarkFunctionInGo(t *testing.T) {
	code := `
def greet(name="John"):
	if name == "null":
		fail("name cannot be 'null'")
	return "Hello, " + name + "!"

greet_func = greet
`
	// run the starlark code
	globals, err := execStarlark(code, nil)
	if err != nil {
		t.Fatalf(`expected no error, but got %v`, err)
	}

	// retrieve the starlark function
	greet, ok := globals["greet_func"].(*starlark.Function)
	if !ok {
		t.Fatalf(`expected greet_func to be a *starlark.Function, but got %T`, globals["greet_func"])
	}
	thread := &starlark.Thread{
		Name:  "test",
		Print: func(_ *starlark.Thread, msg string) { fmt.Println("🌟", msg) },
	}

	// call the starlark function with no arguments
	if res, err := starlark.Call(thread, greet, starlark.Tuple{}, nil); err != nil {
		t.Fatalf(`expected no error while calling greet(), but got %v`, err)
	} else if resStr, ok := res.(starlark.String); !ok {
		t.Fatalf(`expected greet() to return a starlark.String, but got %T`, resStr)
	} else if resStr.GoString() != `Hello, John!` {
		t.Fatalf(`expected greet() to return "Hello, John!", but got %s`, resStr.GoString())
	}

	// call the starlark function with one argument
	if res, err := starlark.Call(thread, greet, starlark.Tuple{starlark.String("Jane")}, nil); err != nil {
		t.Fatalf(`expected no error while calling greet("Jane"), but got %v`, err)
	} else if resStr, ok := res.(starlark.String); !ok {
		t.Fatalf(`expected greet("Jane") to return a starlark.String, but got %T`, resStr)
	} else if resStr.GoString() != `Hello, Jane!` {
		t.Fatalf(`expected greet("Jane") to return "Hello, Jane!", but got %s`, resStr.GoString())
	}

	// call the starlark function with extra arguments
	if _, err := starlark.Call(thread, greet, starlark.Tuple{starlark.String("Jane"), starlark.String("Doe")}, nil); err == nil {
		t.Fatalf(`expected an error while calling greet("Jane", "Doe"), but got none`)
	}

	// call the starlark function and expect an error
	if _, err := starlark.Call(thread, greet, starlark.Tuple{starlark.String("null")}, nil); err == nil {
		t.Fatalf(`expected an error while calling greet("null"), but got none`)
	}
}

func TestUseGoValueInStarlark(t *testing.T) {
	// for common go values, convert them to starlark values and run the starlark code with go assert and starlark test assert
	codeCompareList := `

print('※ go_value: {}({})'.format(go_value, type(go_value)))
def test():
	for i in range(len(exp)):
		if go_value[i] != exp[i]:
			fail('go_value[{}] {}({}) is not equal to {}({})'.format(i, go_value[i],type(go_value[i]), exp[i],type(exp[i])))
		else:
			print('go_value[{}] {}({}) == {}({})'.format(i, go_value[i],type(go_value[i]), exp[i],type(exp[i])))
test()
`
	codeCompareMapDict := `

print('※ go_value: {}({})'.format(go_value, type(go_value)))
def test():
	el = sorted(list(exp.items()))
	al = sorted(list(go_value.items()))
	if el != al:
		fail('go_value {}({}) is not equal to {}({})'.format(go_value,type(go_value), exp,type(exp)))
`

	type testCase struct {
		name        string
		goValue     interface{}
		codeSnippet string
		wantErrConv bool
		wantErrExec bool
	}
	testCases := []testCase{
		{
			name:    "nil",
			goValue: nil,
			codeSnippet: `
assert.Equal(None, go_value)

print('go_value: {}({})'.format(go_value, type(go_value)))
def test():
	if go_value != None:
		fail('go_value is not None')
test()
`,
		},
		{
			name:    "int",
			goValue: 123,
			codeSnippet: `
assert.Equal(123, go_value)

print('go_value: {}({})'.format(go_value, type(go_value)))
def test():
	if go_value != 123:
		fail('go_value is not 123')
test()
`,
		},
		{
			name:    "string",
			goValue: "aloha",
			codeSnippet: `
assert.Equal('aloha', go_value)

print('go_value: {}({})'.format(go_value, type(go_value)))
def test():
	if go_value != 'aloha':
		fail('go_value is not "aloha"')
test()
`,
		},
		{
			name:        "slice of interface",
			goValue:     []interface{}{123, "world"},
			codeSnippet: `exp = [123, "world"]` + codeCompareList,
			wantErrExec: true, // for []interface{}, convert to GoSlice+GoInterface
		},
		{
			name:        "complex slice of interface",
			goValue:     []interface{}{123, "world", []int{1, 2, 3}, []string{"hello", "world"}},
			codeSnippet: `exp = [123, "world", [1, 2, 3], ["hello", "world"]]` + codeCompareList,
			wantErrExec: true, // for complex []interface{}, convert to GoSlice+GoInterface
		},
		{
			name:        "slice of int",
			goValue:     []int{123, 456},
			codeSnippet: `exp = [123, 456]` + codeCompareList,
		},
		{
			name:        "slice of string",
			goValue:     []string{"hello", "world"},
			codeSnippet: `exp = ["hello", "world"]` + codeCompareList,
		},
		{
			name:        "slice of bool",
			goValue:     []bool{true, false},
			codeSnippet: `exp = [True, False]` + codeCompareList,
		},
		{
			name:        "array of interface",
			goValue:     [2]interface{}{123, "world"},
			codeSnippet: `exp = [123, "world"]` + codeCompareList,
			wantErrExec: true, // for [2]interface{}, convert to GoSlice+GoInterface
		},
		{
			name:        "complex array of interface",
			goValue:     [4]interface{}{123, "world", []int{1, 2, 3}, []string{"hello", "world"}},
			codeSnippet: `exp = [123, "world", [1, 2, 3], ["hello", "world"]]` + codeCompareList,
			wantErrExec: true, // for complex [4]interface{}, convert to GoSlice+GoInterface
		},
		{
			name:        "array of int",
			goValue:     [2]int{123, 456},
			codeSnippet: `exp = [123, 456]` + codeCompareList,
		},
		{
			name:        "array of string",
			goValue:     [2]string{"hello", "world"},
			codeSnippet: `exp = ["hello", "world"]` + codeCompareList,
		},
		{
			name:        "array of bool",
			goValue:     [2]bool{true, false},
			codeSnippet: `exp = [True, False]` + codeCompareList,
		},
		{
			name:        "map of string to int",
			goValue:     map[string]int{"one": 1, "two": 2},
			codeSnippet: `exp = {"one": 1, "two": 2}` + codeCompareMapDict,
		},
		{
			name:        "map of int to string",
			goValue:     map[int]string{1: "one", 2: "two"},
			codeSnippet: `exp = {1: "one", 2: "two"}` + codeCompareMapDict,
		},
		{
			name:        "map of string to slice of int",
			goValue:     map[string][]int{"one": {1, 2}, "two": {3, 4}},
			codeSnippet: `exp = {"one": [1, 2], "two": [3, 4]}` + codeCompareMapDict,
		},
		// INSERT MORE TEST CASES HERE
		// ...
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			globals := map[string]interface{}{
				"assert":   &assert{t: t},
				"go_value": tc.goValue,
			}

			// convert go values to Starlark values as predefined globals
			env, errConv := convert.MakeStringDict(globals)
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
