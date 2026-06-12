package convert_test

import (
	"strings"
	"testing"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
	"go.starlark.net/starlark"
)

// Regression tests for dict.popitem's mutation guards. Every other GoMap
// mutation entry point (SetKey, Clear, Delete/pop) rejects writes on a
// frozen map and during iteration; popitem called the internal unguarded
// delete, so it silently mutated frozen maps and mutated maps mid-iteration
// where its sibling pop() correctly errors.

// TestPopitemFrozen verifies popitem on a frozen GoMap errors instead of
// mutating, matching pop().
func TestPopitemFrozen(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	g := convert.NewGoMap(m)
	g.Freeze()
	globals := map[string]interface{}{"m": g}

	_, err := starlight.Eval([]byte(`x = m.popitem()`), globals, nil)
	if err == nil || !strings.Contains(err.Error(), "frozen") {
		t.Fatalf("expected frozen error, got %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("expected frozen map unchanged, got %v", m)
	}
}

// TestPopitemDuringIteration verifies popitem during an active iteration of
// the same map errors, matching pop(). Driven at the API level because a
// Starlark for-loop must live in a function.
func TestPopitemDuringIteration(t *testing.T) {
	g := convert.NewGoMap(map[string]int{"a": 1, "b": 2, "c": 3})
	it := g.Iterate()
	defer it.Done()
	var k starlark.Value
	it.Next(&k) // open the iteration (numIt > 0)

	piFn, err := g.Attr("popitem")
	if err != nil || piFn == nil {
		t.Fatal("no popitem attr")
	}
	_, piErr := starlark.Call(&starlark.Thread{}, piFn.(*starlark.Builtin), nil, nil)
	if piErr == nil || !strings.Contains(piErr.Error(), "during iteration") {
		t.Fatalf("expected during-iteration error, got %v", piErr)
	}

	// pop() agrees
	popFn, _ := g.Attr("pop")
	_, popErr := starlark.Call(&starlark.Thread{}, popFn.(*starlark.Builtin), starlark.Tuple{starlark.String("a")}, nil)
	if popErr == nil || !strings.Contains(popErr.Error(), "during iteration") {
		t.Fatalf("pop should also error during iteration, got %v", popErr)
	}
}

// TestPopitemGuardsMatchPop directly compares popitem and pop guard
// behavior at the API level to ensure they agree.
func TestPopitemGuardsMatchPop(t *testing.T) {
	makeFrozen := func() *convert.GoMap {
		g := convert.NewGoMap(map[string]int{"a": 1})
		g.Freeze()
		return g
	}
	// pop on frozen
	gp := makeFrozen()
	popFn, err := gp.Attr("pop")
	if err != nil || popFn == nil {
		t.Fatal("no pop attr")
	}
	_, popErr := starlark.Call(&starlark.Thread{}, popFn.(*starlark.Builtin), starlark.Tuple{starlark.String("a")}, nil)
	// popitem on frozen
	gpi := makeFrozen()
	piFn, err := gpi.Attr("popitem")
	if err != nil || piFn == nil {
		t.Fatal("no popitem attr")
	}
	_, piErr := starlark.Call(&starlark.Thread{}, piFn.(*starlark.Builtin), nil, nil)

	if popErr == nil || piErr == nil {
		t.Fatalf("both must error on frozen: pop=%v popitem=%v", popErr, piErr)
	}
	if !strings.Contains(piErr.Error(), "frozen") {
		t.Fatalf("popitem frozen error should mention frozen, got %v", piErr)
	}
}

// TestPopitemStillWorks pins the happy path: popitem on a mutable,
// non-iterating map still pops the deterministic smallest key.
func TestPopitemStillWorks(t *testing.T) {
	m := map[string]int{"c": 3, "a": 1, "b": 2}
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"m":      m,
	}
	code := []byte(`
assert.Eq(m.popitem(), ("a", 1))
assert.Eq(m.popitem(), ("b", 2))
assert.Eq(len(m), 1)
`)
	if _, err := starlight.Eval(code, globals, nil); err != nil {
		t.Fatal(err)
	}
}
