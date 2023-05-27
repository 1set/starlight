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
	type testCase struct {
		name        string
		goValue     interface{}
		codeSnippet string
		wantErrConv bool
		wantErrExec bool
	}
	testCases := []testCase{
		{
			name:        "int",
			goValue:     123,
			codeSnippet: `assert.Eq(123, go_value)`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			globals := map[string]interface{}{
				"assert":   &assert{t: t},
				"go_value": tc.goValue,
			}
			env, errConv := convert.MakeStringDict(globals)
			if errConv != nil == !tc.wantErrConv {
				t.Fatalf(`expected no error while converting globals, but got %v`, errConv)
			} else if errConv == nil && tc.wantErrConv {
				t.Fatalf(`expected an error while converting globals, but got none`)
			}
			if errConv != nil {
				return
			}

			_, errExec := execStarlark(tc.codeSnippet, env)
			if errExec != nil && !tc.wantErrExec {
				t.Fatalf(`expected no error while executing code snippet, but got %v`, errExec)
			} else if errExec == nil && tc.wantErrExec {
				t.Fatalf(`expected an error while executing code snippet, but got none`)
			}
		})
	}
}
