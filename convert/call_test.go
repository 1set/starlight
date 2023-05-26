package convert_test

import (
	"go.starlark.net/starlark"
	"testing"
)

func TestCallStarlarkFunctionInGo(t *testing.T) {
	code := `
def greet(name="John"):
	return "Hello, " + name + "!"

greet_func = greet
`
	globals, err := execStarlark(code, nil)
	if err != nil {
		t.Fatalf(`expected no error, but got %v`, err)
	}
	greet, ok := globals["greet_func"].(*starlark.Function)
	if !ok {
		t.Fatalf(`expected greet_func to be a *starlark.Function, but got %T`, globals["greet_func"])
	}
	t.Logf(`greet_func: %v`, greet)
}
