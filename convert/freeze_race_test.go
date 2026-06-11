package convert_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
	"go.starlark.net/starlark"
)

// Regression tests for freeze semantics and conversion concurrency:
//   - GoStruct.Freeze was an empty function, so scripts kept mutating host
//     structs through frozen values;
//   - the package-level recursion detector was shared across goroutines by
//     pointer, so concurrent conversions of the same list/dict spuriously
//     saw "already visited" and silently returned nil (data loss), and the
//     iteration counters raced.

type frozenTarget struct {
	Name string
	Num  int
}

// TestFreezeGoStruct verifies that a frozen GoStruct rejects writes through
// both of its write paths (attribute assignment and index assignment).
func TestFreezeGoStruct(t *testing.T) {
	s := &frozenTarget{Name: "a", Num: 1}
	v, err := convert.ToValue(s)
	if err != nil {
		t.Fatal(err)
	}
	v.Freeze()
	globals := map[string]interface{}{"s": v}

	for _, code := range []string{`s.Name = "b"`, `s["Num"] = 2`} {
		_, err = starlight.Eval([]byte(code), globals, nil)
		if err == nil || !strings.Contains(err.Error(), "frozen") {
			t.Fatalf("%s: expected frozen error, got %v", code, err)
		}
	}
	if s.Name != "a" || s.Num != 1 {
		t.Fatalf("expected struct unchanged, got %+v", s)
	}

	// unfrozen wrappers still accept writes
	s2 := &frozenTarget{Name: "a", Num: 1}
	globals2 := map[string]interface{}{"s": s2}
	if _, err := starlight.Eval([]byte(`s.Name = "b"`), globals2, nil); err != nil {
		t.Fatal(err)
	}
	if s2.Name != "b" {
		t.Fatalf("expected write to unfrozen struct to work, got %+v", s2)
	}
}

// TestConcurrentFromValue verifies that concurrent conversions of the same
// Starlark list/dict are complete and race-free: with the shared
// package-level recursion detector, goroutines spuriously saw each other's
// in-progress markers and silently got nil back.
func TestConcurrentFromValue(t *testing.T) {
	l := starlark.NewList(nil)
	for i := 0; i < 10; i++ {
		if err := l.Append(starlark.MakeInt(i)); err != nil {
			t.Fatal(err)
		}
	}
	d := starlark.NewDict(10)
	for i := 0; i < 10; i++ {
		if err := d.SetKey(starlark.String(fmt.Sprintf("k%d", i)), starlark.MakeInt(i)); err != nil {
			t.Fatal(err)
		}
	}
	// per the Starlark contract, values shared across threads must be
	// frozen first (unfrozen values race on L0's own iteration counters)
	l.Freeze()
	d.Freeze()

	const goroutines = 8
	const rounds = 20
	var wg sync.WaitGroup
	errs := make(chan error, goroutines*rounds*2)
	for gi := 0; gi < goroutines; gi++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				if got := convert.FromList(l); len(got) != 10 {
					errs <- fmt.Errorf("FromList returned %d elements, want 10", len(got))
					return
				}
				if got := convert.FromDict(d); len(got) != 10 {
					errs <- fmt.Errorf("FromDict returned %d entries, want 10", len(got))
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// TestConcurrentIterate verifies concurrent read-only iteration over the
// same wrapped Go map and slice is race-free (the iteration counters used
// to be plain ints).
func TestConcurrentIterate(t *testing.T) {
	gm := convert.NewGoMap(map[string]int{"a": 1, "b": 2, "c": 3})
	gs := convert.NewGoSlice([]int{1, 2, 3})

	const goroutines = 8
	var wg sync.WaitGroup
	for gi := 0; gi < goroutines; gi++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 0; r < 50; r++ {
				var v starlark.Value
				it := gm.Iterate()
				for it.Next(&v) {
				}
				it.Done()
				it = gs.Iterate()
				for it.Next(&v) {
				}
				it.Done()
			}
		}()
	}
	wg.Wait()
}

// TestFromListCycle pins the documented cycle behavior: a list reaching
// itself converts with nil in place of the cyclic reference, instead of
// recursing forever.
func TestFromListCycle(t *testing.T) {
	l := starlark.NewList(nil)
	if err := l.Append(starlark.MakeInt(1)); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(l); err != nil {
		t.Fatal(err)
	}
	got := convert.FromList(l)
	if len(got) != 2 {
		t.Fatalf("expected 2 elements, got %#v", got)
	}
	if got[0] != int64(1) {
		t.Fatalf("expected first element 1, got %#v", got[0])
	}
	if s, ok := got[1].([]interface{}); !ok || s != nil {
		t.Fatalf("expected cyclic reference to convert to a nil slice, got %#v", got[1])
	}
}
