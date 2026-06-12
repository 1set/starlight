package convert_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
	"go.starlark.net/starlark"
)

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
