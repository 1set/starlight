package convert_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
	"go.starlark.net/starlark"
)

// Regression tests for five broken bridge contracts:
//  1. wrapped Go functions silently swallowed keyword arguments, so
//     fn(bogus=1) succeeded as if the kwarg had been applied;
//  2. the error-return convention only matched the exact `error` interface
//     type, so functions returning a concrete error type leaked it as a
//     value instead of raising;
//  3. MakeDict ignored SetKey errors, silently dropping entries whose key
//     is unhashable on the Starlark side;
//  4. pointer-receiver methods were invisible on addressable struct values
//     (e.g. fields of a wrapped *struct);
//  5. GoInterface's dir() omitted the synthetic to* conversion attributes
//     that Attr actually supports, and double-listed methods for pointers.

// TestKwargsRejected verifies wrapped Go functions reject keyword
// arguments instead of silently dropping them.
func TestKwargsRejected(t *testing.T) {
	globals := map[string]interface{}{
		"quote":  func(s string) string { return `"` + s + `"` },
		"concat": func(ss ...string) string { return strings.Join(ss, "") },
	}
	for _, code := range []string{`v = quote("a", bogus=1)`, `v = concat("a", "b", bogus=1)`} {
		_, err := starlight.Eval([]byte(code), globals, nil)
		if err == nil || !strings.Contains(err.Error(), "unexpected keyword argument") {
			t.Fatalf("%s: expected unexpected-keyword error, got %v", code, err)
		}
	}
	// no kwargs still works
	res, err := starlight.Eval([]byte(`v = quote("a")`), globals, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res["v"] != `"a"` {
		t.Fatalf("expected quoted a, got %v", res["v"])
	}
}

type customErr struct {
	msg string
}

func (e *customErr) Error() string { return e.msg }

// TestConcreteErrorReturn verifies functions returning a concrete error
// type raise in Starlark like `error`-typed returns do, and that a typed
// nil error does not raise.
func TestConcreteErrorReturn(t *testing.T) {
	globals := map[string]interface{}{
		"fail":   func() *customErr { return &customErr{msg: "boom"} },
		"calc":   func(ok bool) (string, *customErr) { return "fine", nil },
		"calc2":  func() (string, *customErr) { return "", &customErr{msg: "nope"} },
		"legacy": func() (string, error) { return "v", nil },
	}
	_, err := starlight.Eval([]byte(`fail()`), globals, nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected concrete error to raise, got %v", err)
	}
	res, err := starlight.Eval([]byte(`v = calc(True)`), globals, nil)
	if err != nil {
		t.Fatalf("typed nil error must not raise: %v", err)
	}
	if res["v"] != "fine" {
		t.Fatalf("expected fine, got %v", res["v"])
	}
	_, err = starlight.Eval([]byte(`v = calc2()`), globals, nil)
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("expected concrete error to raise, got %v", err)
	}
	if res, err = starlight.Eval([]byte(`v = legacy()`), globals, nil); err != nil || res["v"] != "v" {
		t.Fatalf("plain error convention broken: %v %v", res, err)
	}
}

// TestMakeDictKeyError verifies MakeDict reports keys that are unhashable
// on the Starlark side instead of silently dropping the entries.
func TestMakeDictKeyError(t *testing.T) {
	// [2]string is a comparable Go map key, but converts to a Starlark
	// value (a slice wrapper) that is not hashable
	m := map[[2]string]int{{"a", "b"}: 1}
	if _, err := convert.MakeDict(m); err == nil {
		t.Fatal("expected unhashable-key error from MakeDict, got nil")
	}
	// plain dicts still convert
	v, err := convert.MakeDict(map[string]int{"a": 1})
	if err != nil {
		t.Fatal(err)
	}
	if v.(*starlark.Dict).Len() != 1 {
		t.Fatalf("expected 1 entry, got %v", v)
	}
}

type innerCounter struct {
	N int
}

func (c *innerCounter) Incr() { c.N++ }
func (c innerCounter) Get() int {
	return c.N
}

type outerHolder struct {
	Inner innerCounter
}

// TestPointerMethodsOnAddressableValue verifies pointer-receiver methods
// are reachable on addressable struct values, e.g. fields of a wrapped
// pointer — and that their mutations are visible to the host.
func TestPointerMethodsOnAddressableValue(t *testing.T) {
	o := &outerHolder{}
	globals := map[string]interface{}{"o": o}
	code := []byte(`
o.Inner.Incr()
o.Inner.Incr()
v = o.Inner.Get()
`)
	res, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}
	if o.Inner.N != 2 {
		t.Fatalf("expected mutations visible on host (N=2), got %+v", o)
	}
	if res["v"] != int64(2) {
		t.Fatalf("expected v=2, got %v", res["v"])
	}
}

type loudInt int

func (l loudInt) Doubled() int { return int(l) * 2 }

// TestGoInterfaceDirHonest verifies dir() on a GoInterface lists exactly
// what Attr supports: real methods plus the synthetic to* conversions,
// without duplicates.
func TestGoInterfaceDirHonest(t *testing.T) {
	v, err := convert.ToValue(loudInt(21))
	if err != nil {
		t.Fatal(err)
	}
	gi, ok := v.(*convert.GoInterface)
	if !ok {
		t.Fatalf("expected GoInterface, got %T", v)
	}
	names := gi.AttrNames()
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Fatalf("duplicate attr name %q in %v", n, names)
		}
		seen[n] = true
	}
	for _, want := range []string{"Doubled", "toInt", "toString", "toFloat", "toUint", "toBool"} {
		if !seen[want] {
			t.Fatalf("expected %q in AttrNames, got %v", want, names)
		}
	}
	if !sort.StringsAreSorted(names) {
		t.Fatalf("expected sorted attr names, got %v", names)
	}
	// every listed name must actually resolve through Attr
	for _, n := range names {
		av, err := gi.Attr(n)
		if err != nil || av == nil {
			t.Fatalf("AttrNames lists %q but Attr returns (%v, %v)", n, av, err)
		}
	}
	// and the synthetic conversion works from a script
	globals := map[string]interface{}{"x": v, "assert": nil}
	delete(globals, "assert")
	res, err := starlight.Eval([]byte(`v = x.Doubled()
i = x.toInt()`), globals, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res["v"] != int64(42) || res["i"] != int64(21) {
		t.Fatalf("expected 42/21, got %v", res)
	}
	_ = fmt.Sprint // silence unused import on some build tags
}
