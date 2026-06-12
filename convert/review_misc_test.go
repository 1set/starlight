package convert_test

import (
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
	startime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
)

// Consolidated small black-box tests from the review round: popitem
// guards, time.Duration symmetry / Int ladder, and the kwargs-message /
// reachable-branch cleanups.

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

// TestDurationSymmetry verifies time.Duration maps bidirectionally to the
// standard Starlark time.duration. It used to convert one way only: into
// an opaque interface wrapper that scripts could not use as a duration,
// while time.duration values coming back were never unwrapped.
func TestDurationSymmetry(t *testing.T) {
	d := 90 * time.Second

	v, err := convert.ToValue(d)
	if err != nil {
		t.Fatal(err)
	}
	sd, ok := v.(startime.Duration)
	if !ok {
		t.Fatalf("expected startime.Duration, got %T", v)
	}
	if time.Duration(sd) != d {
		t.Fatalf("expected %v, got %v", d, time.Duration(sd))
	}

	back := convert.FromValue(v)
	gd, ok := back.(time.Duration)
	if !ok || gd != d {
		t.Fatalf("expected round-trip %v, got %v (%T)", d, back, back)
	}

	// script-visible: it is a real time.duration, with duration semantics
	globals := map[string]interface{}{"d": d}
	res, err := starlight.Eval([]byte(`
t = type(d)
double = d + d
seconds = d.seconds
`), globals, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res["t"] != "time.duration" {
		t.Fatalf("expected time.duration, got %v", res["t"])
	}
	if dd, ok := res["double"].(time.Duration); !ok || dd != 3*time.Minute {
		t.Fatalf("expected 3m, got %v (%T)", res["double"], res["double"])
	}
	if res["seconds"] != float64(90) {
		t.Fatalf("expected 90 seconds, got %v", res["seconds"])
	}

	// duration arguments convert back for Go functions
	var got time.Duration
	globals2 := map[string]interface{}{
		"d":   d,
		"fnD": func(in time.Duration) { got = in },
	}
	if _, err := starlight.Eval([]byte(`fnD(d)`), globals2, nil); err != nil {
		t.Fatal(err)
	}
	if got != d {
		t.Fatalf("expected %v, got %v", d, got)
	}
}

// TestIntLadderContract pins the documented FromValue integer ladder:
// int64 if it fits, else uint64, else *big.Int.
func TestIntLadderContract(t *testing.T) {
	small := convert.FromValue(starlark.MakeInt(42))
	if v, ok := small.(int64); !ok || v != 42 {
		t.Fatalf("expected int64 42, got %v (%T)", small, small)
	}

	negative := convert.FromValue(starlark.MakeInt(-42))
	if v, ok := negative.(int64); !ok || v != -42 {
		t.Fatalf("expected int64 -42, got %v (%T)", negative, negative)
	}

	wide := convert.FromValue(starlark.MakeUint64(10000000000000000000)) // > MaxInt64
	if v, ok := wide.(uint64); !ok || v != 10000000000000000000 {
		t.Fatalf("expected uint64 1e19, got %v (%T)", wide, wide)
	}

	huge, ok := new(big.Int).SetString("36893488147419103232", 10) // 2^65
	if !ok {
		t.Fatal("bad big literal")
	}
	b := convert.FromValue(starlark.MakeBigInt(huge))
	if v, ok := b.(*big.Int); !ok || v.Cmp(huge) != 0 {
		t.Fatalf("expected *big.Int 2^65, got %v (%T)", b, b)
	}
}

// TestKwargsErrorNotDoubleQuoted verifies the unexpected-keyword error names
// the argument once, not double-quoted. kwargs[0][0] is a starlark.String
// whose String() already quotes, so the old %q produced the literal
// "\"name\"".
func TestKwargsErrorNotDoubleQuoted(t *testing.T) {
	globals := map[string]interface{}{
		"quote":  func(s string) string { return s },
		"concat": func(ss ...string) string { return strings.Join(ss, "") },
	}
	for _, code := range []string{`quote("a", bogus=1)`, `concat("a", bogus=1)`} {
		_, err := starlight.Eval([]byte(code), globals, nil)
		if err == nil {
			t.Fatalf("%s: expected error", code)
		}
		msg := err.Error()
		if !strings.Contains(msg, `argument "bogus"`) {
			t.Fatalf("%s: expected single-quoted arg name, got %q", code, msg)
		}
		if strings.Contains(msg, `\"bogus\"`) || strings.Contains(msg, `"\"bogus\""`) {
			t.Fatalf("%s: argument name is double-quoted: %q", code, msg)
		}
	}
}

// reNarrowValue is a custom starlark.Value FromValue leaves as-is, used to
// reach the collection-element starlark.Value re-narrowing branch in
// convertElemValue (the one whose comment used to claim it was unreachable).
type reNarrowValue struct{ n int64 }

func (reNarrowValue) String() string        { return "reNarrowValue" }
func (reNarrowValue) Type() string          { return "reNarrowValue" }
func (reNarrowValue) Freeze()               {}
func (reNarrowValue) Truth() starlark.Bool  { return true }
func (reNarrowValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable") }

// TestConvertElemStarlarkValueBranchReachable proves the branch is live: a
// custom starlark.Value inside a collection passed to a typed Go parameter
// reaches the re-narrowing path and errors cleanly (it is not numeric).
func TestConvertElemStarlarkValueBranchReachable(t *testing.T) {
	fn := convert.MakeStarFn("fn", func(m map[string]int) int { return len(m) })
	globals := starlark.StringDict{"fn": fn, "c": reNarrowValue{n: 5}}
	_, err := starlark.ExecFile(&starlark.Thread{}, "t.star", `x = fn({"a": c})`, globals)
	if err == nil {
		t.Fatal("expected a conversion error for a non-numeric custom value")
	}
	if !strings.Contains(err.Error(), "reNarrowValue") {
		t.Fatalf("expected error to mention the custom type, got %v", err)
	}
}

// TestToValueTypedNilStarlarkPointer: a typed-nil pointer that satisfies the
// starlark.Value interface (e.g. (*starlark.List)(nil)) passed the assertion
// in ToValue and was returned as-is, panicking later when the interpreter
// dereferenced it. It must become None, like other nil-ish inputs.
func TestToValueTypedNilStarlarkPointer(t *testing.T) {
	var l *starlark.List
	var d *starlark.Dict
	for _, v := range []interface{}{l, d} {
		got, err := convert.ToValue(v)
		if err != nil {
			t.Fatalf("ToValue(%T) errored: %v", v, err)
		}
		if got != starlark.None {
			t.Fatalf("ToValue(%T nil) = %v, want None", v, got)
		}
	}
}

type boxNilErr struct{}

func (e *boxNilErr) Error() string { return "should-not-raise" }

// TestMakeOutBoxedNilError: a func declaring (T, error) that returns a
// typed-nil pointer boxed in the error interface (a common Go idiom) must
// not raise — the boxed nil is "no error".
func TestMakeOutBoxedNilError(t *testing.T) {
	globals := map[string]interface{}{
		"fn": func() (int, error) {
			var e *boxNilErr // typed nil
			return 42, e
		},
	}
	res, err := starlight.Eval([]byte(`x = fn()`), globals, nil)
	if err != nil {
		t.Fatalf("boxed typed-nil error must not raise, got %v", err)
	}
	if res["x"] != int64(42) {
		t.Fatalf("expected 42, got %v", res["x"])
	}
	// a real error still raises
	globals["fn2"] = func() (int, error) { return 0, &boxNilErr{} }
	if _, err := starlight.Eval([]byte(`x = fn2()`), globals, nil); err == nil {
		t.Fatal("a non-nil error should still raise")
	}
}

// TestPointerToTimeTime: a *time.Time reached the struct case still as a
// pointer and was wrapped as a GoStruct instead of the Starlark time type.
func TestPointerToTimeTime(t *testing.T) {
	now := time.Date(2026, 6, 12, 1, 2, 3, 0, time.UTC)
	v, err := convert.ToValue(&now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := v.(startime.Time); !ok {
		t.Fatalf("expected startime.Time for *time.Time, got %T", v)
	}
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"t":      &now,
	}
	if _, err := starlight.Eval([]byte(`
assert.Eq(type(t), "time.time")
assert.Eq(t.year, 2026)
`), globals, nil); err != nil {
		t.Fatal(err)
	}
}

// TestNewStructNilMessage: NewStruct/NewStructWithTag formatted their panic
// with val.Interface(), which panics again on a nil (Invalid) arg. The panic
// must carry a clean "<nil>".
func TestNewStructNilMessage(t *testing.T) {
	for _, fn := range []func(){
		func() { convert.NewStruct(nil) },
		func() { convert.NewStructWithTag(nil, "tag") },
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("expected panic for nil arg")
				}
				msg := fmt.Sprint(r)
				if strings.Contains(msg, "zero Value") {
					t.Fatalf("panic-message formatting itself panicked: %v", r)
				}
				if !strings.Contains(msg, "<nil>") {
					t.Fatalf("expected '<nil>' in panic, got %v", r)
				}
			}()
			fn()
		}()
	}
}
