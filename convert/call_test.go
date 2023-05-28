package convert_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/1set/starlight/convert"
	"go.starlark.net/starlark"
)

// TestCallStarlarkFunctionInGo tests calling a Starlark function in Go with various arguments.
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
	jane, _ := convert.ToValue("Jane")
	if res, err := starlark.Call(thread, greet, starlark.Tuple{jane}, nil); err != nil {
		t.Fatalf(`expected no error while calling greet("Jane"), but got %v`, err)
	} else if resStr, ok := res.(starlark.String); !ok {
		t.Fatalf(`expected greet("Jane") to return a starlark.String, but got %T`, resStr)
	} else if resStr.GoString() != `Hello, Jane!` {
		t.Fatalf(`expected greet("Jane") to return "Hello, Jane!", but got %s`, resStr.GoString())
	}

	// call the starlark function with extra arguments
	doe, _ := convert.ToValue("Doe")
	if _, err := starlark.Call(thread, greet, starlark.Tuple{jane, doe}, nil); err == nil {
		t.Fatalf(`expected an error while calling greet("Jane", "Doe"), but got none`)
	}

	// call the starlark function and expect an error
	if _, err := starlark.Call(thread, greet, starlark.Tuple{starlark.String("null")}, nil); err == nil {
		t.Fatalf(`expected an error while calling greet("null"), but got none`)
	}
}

// TestUseGoValueInStarlark tests using various Go values in Starlark. It verifies:
// 1. the Go value can be converted to Starlark values as input;
// 2. the converted Starlark values can be used in Starlark code;
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

print('※ go_value: {}({})'.format(go_value, type(go_value)))
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

print('※ go_value: {}({})'.format(go_value, type(go_value)))
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

print('※ go_value: {}({})'.format(go_value, type(go_value)))
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
		{
			name:    "empty struct",
			goValue: struct{}{},
			codeSnippet: `
print('※ go_value: {}({})'.format(go_value, type(go_value)))
assert.Equal({}, go_value)
`,
			wantErrExec: true,
		},
		{
			name: "custom struct",
			goValue: struct {
				Name  string
				Value int
			}{Name: "Hello", Value: 42},
			codeSnippet: `
print('※ go_value: {}({})'.format(go_value, type(go_value)))
def test():
	if go_value.Name != 'Hello' or go_value.Value != 42:
		fail('go_value is not "aloha"')
test()
`,
		},
		{
			name: "custom function",
			goValue: func(name string) string {
				return "Hello " + name
			},
			codeSnippet: `
print('※ go_value: {}({})'.format(go_value, type(go_value)))
def test():
	if go_value("World") != 'Hello World':
		fail('go_value is not "Hello"')
test()
`,
		},
		{
			name:    "unsupported type",
			goValue: make(chan bool),
			codeSnippet: `
print('※ go_value: {}({})'.format(go_value, type(go_value)))
`,
			wantErrConv: true,
		},
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

// TestCallGoFunctionInStarlark tests calling Go functions in Starlark with various types of arguments and return values.
// It verifies:
// 1. Go functions can be converted to Starlark functions;
// 2. Return values of Go functions can be converted to Starlark values;
// 3. Starlark values can be converted to Go values;
func TestCallGoFunctionInStarlark(t *testing.T) {
	type customStruct struct {
		Name  string
		Value int
	}
	type testCase struct {
		name         string
		goFunc       interface{}
		codeSnippet  string
		expectResult interface{}
		wantErrExec  bool
		wantEqual    bool
	}
	testCases := []testCase{
		{
			name: "func() string",
			goFunc: func() string {
				return "Aloha!"
			},
			codeSnippet:  `sl_value = go_func()`,
			expectResult: "Aloha!",
			wantEqual:    true,
		},
		{
			name: "func(string) string",
			goFunc: func(name string) string {
				return "Hello " + name + "!"
			},
			codeSnippet:  `sl_value = go_func("World")`,
			expectResult: "Hello World!",
			wantEqual:    true,
		},
		{
			name: "func(string) int",
			goFunc: func(name string) int {
				return len(name)
			},
			codeSnippet:  `sl_value = go_func("World")`,
			expectResult: int64(5),
			wantEqual:    true,
		},
		{
			name: "func(string) (string, error)",
			goFunc: func(name string) (string, error) {
				return "Hello " + name + "!", nil
			},
			codeSnippet:  `sl_value = go_func("World")`,
			expectResult: "Hello World!",
			wantEqual:    true,
		},
		{
			name: "func(string) (error, error)",
			goFunc: func(name string) (error, error) {
				return fmt.Errorf("need %s", name), nil
			},
			codeSnippet:  `sl_value = go_func("attention")`,
			expectResult: errors.New("need attention"),
			wantEqual:    true,
		},
		{
			name: "unsupported func(chan) int",
			goFunc: func(ch chan int) int {
				return <-ch
			},
			codeSnippet: `sl_value = go_func(42)`,
			wantErrExec: true,
		},
		{
			name: "unsupported func(int) chan",
			goFunc: func(size int) chan int {
				return make(chan int, size)
			},
			codeSnippet: `sl_value = go_func(42)`,
			wantErrExec: true,
		},
		{
			name: "mismatched func(int) string",
			goFunc: func(name int) string {
				return fmt.Sprintf("Hello %d!", name)
			},
			codeSnippet: `sl_value = go_func("42")`,
			wantErrExec: true,
		},
		{
			name: "fuzzy func(string) int",
			goFunc: func(name string) int {
				return len(name)
			},
			codeSnippet:  `sl_value = go_func(42)`,
			expectResult: int64(1),
			wantEqual:    true,
		},
		{
			name: "invalid pointer: func(*string) string",
			goFunc: func(name *string) string {
				if name == nil {
					return "Hello World!"
				}
				return "Hello " + *name + "!"
			},
			codeSnippet: `sl_value = go_func("World")`,
			wantErrExec: true,
		},
		{
			name: "invalid pointer: func(string) *string",
			goFunc: func(name string) *string {
				return &name
			},
			codeSnippet: `
sl_value = go_func("World")
print('※ sl_value: {}({})'.format(sl_value, type(sl_value)))
`,
		},
		{
			name: "func([]string) (string)",
			goFunc: func(names []string) string {
				return strings.Join(names, ", ")
			},
			codeSnippet:  `sl_value = go_func(["Alice", "Bob", "Carol"])`,
			expectResult: "Alice, Bob, Carol",
			wantEqual:    true,
		},
		{
			name: "func([]int) string",
			goFunc: func(numbers []int8) int16 {
				x := int16(0)
				for _, n := range numbers {
					x += int16(n)
				}
				return x
			},
			codeSnippet:  `sl_value = go_func([1, 2, 3, 4, 5])`,
			expectResult: int64(15),
			wantEqual:    true,
		},
		{
			name: "func([5]int) int",
			goFunc: func(numbers [5]int) int {
				return numbers[0] + numbers[1] + numbers[2] + numbers[3] + numbers[4]
			},
			codeSnippet: `sl_value = go_func([1, 2, 3, 4, 5])`,
			wantErrExec: true, // TODO: support array as input
		},
		{
			name: "func([][]int) int",
			goFunc: func(numbers [][]int) int {
				x := 0
				for _, row := range numbers {
					for _, n := range row {
						x += n
					}
				}
				return x
			},
			codeSnippet: `sl_value = go_func([[1, 2, 3], [4, 5, 6]])`,
			wantErrExec: true, // TODO: support nested slice as input
		},
		{
			name: "func(map[string]int) int",
			goFunc: func(numbers map[string]int) int {
				x := 0
				for _, n := range numbers {
					x += n
				}
				return x
			},
			codeSnippet:  `sl_value = go_func({"a": 1, "b": 2, "c": 3})`,
			expectResult: int64(6),
			wantEqual:    true,
		},
		{
			name: "func(map[string]map[string]int) string",
			goFunc: func(numbers map[string]map[string]int) string {
				x := 0
				for _, row := range numbers {
					for _, n := range row {
						x += n
					}
				}
				return fmt.Sprintf("%d", x)
			},
			codeSnippet: `sl_value = go_func({"a": {"x": 1, "y": 2, "z": 3}, "b": {"x": 4, "y": 5, "z": 6}})`,
			wantErrExec: true, // TODO: support nested map as input
		},
		{
			name: "func(string) custom",
			goFunc: func(name string) customStruct {
				return customStruct{Name: name, Value: 42}
			},
			codeSnippet:  `sl_value = go_func("Alice")`,
			expectResult: customStruct{Name: "Alice", Value: 42},
			wantEqual:    true,
		},
		{
			name: "func(string) *custom",
			goFunc: func(name string) *customStruct {
				return &customStruct{Name: name, Value: 36}
			},
			codeSnippet:  `sl_value = go_func("Bob")`,
			expectResult: &customStruct{Name: "Bob", Value: 36},
			wantEqual:    true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			globals := map[string]interface{}{
				"go_func": tc.goFunc,
			}

			// convert go functions to Starlark values as predefined globals
			env, errConv := convert.MakeStringDict(globals)
			if errConv != nil {
				t.Fatalf(`expected no error while converting funcs, but got %v`, errConv)
			}

			// run the Starlark code to test the converted globals
			res, errExec := execStarlark(tc.codeSnippet, env)
			if errExec != nil && !tc.wantErrExec {
				t.Fatalf(`expected no error while executing code snippet, but got %v`, errExec)
			} else if errExec == nil && tc.wantErrExec {
				t.Fatalf(`expected an error while executing code snippet, but got none`)
			}
			if errExec != nil {
				return
			}

			// result value
			slValue, found := res["sl_value"]
			if !found {
				t.Fatalf(`expected sl_value in globals, but got none`)
			}

			// compare the result
			if gotEqual := reflect.DeepEqual(slValue, tc.expectResult); gotEqual != tc.wantEqual {
				t.Fatalf(`expected sl_value to be %v (%T), but got %v (%T), want equal: %v`, tc.expectResult, tc.expectResult, slValue, slValue, tc.wantEqual)
			}
		})
	}
}

// TestUseStarlarkValueInGo tests using various Starlark values in Go. It verifies:
// 1. the Starlark values can be converted to Go values as output;
// 2. the converted Go value can be used in Go code;
func TestUseStarlarkValueInGo(t *testing.T) {
}
