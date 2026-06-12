package convert_test

import (
	"strings"
	"testing"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
)

// Regression tests for self-referential Go values: String() used fmt.Sprint
// with no cycle detection, so a Go map or slice that reaches itself made
// String() recurse forever and killed the process with an unrecoverable
// fatal stack overflow. Cyclic values must format as "<cyclic TYPE>"; plain
// values must keep their exact previous formatting.

// TestGoMapSelfRefString verifies a self-referential map formats safely.
func TestGoMapSelfRefString(t *testing.T) {
	m := map[string]interface{}{}
	m["self"] = m
	got := convert.NewGoMap(m).String()
	if !strings.Contains(got, "cyclic") {
		t.Fatalf("expected cyclic marker, got %q", got)
	}
}

// TestGoSliceSelfRefString verifies a self-referential slice formats safely.
func TestGoSliceSelfRefString(t *testing.T) {
	s := make([]interface{}, 1)
	s[0] = s
	v, err := convert.ToValue(s)
	if err != nil {
		t.Fatal(err)
	}
	got := v.String()
	if !strings.Contains(got, "cyclic") {
		t.Fatalf("expected cyclic marker, got %q", got)
	}
}

// TestGoStructSelfRefString verifies a struct whose field holds a
// self-referential map formats safely.
func TestGoStructSelfRefString(t *testing.T) {
	type node struct {
		M map[string]interface{}
	}
	n := node{M: map[string]interface{}{}}
	n.M["self"] = n.M
	v, err := convert.ToValue(n)
	if err != nil {
		t.Fatal(err)
	}
	got := v.String()
	if !strings.Contains(got, "cyclic") {
		t.Fatalf("expected cyclic marker, got %q", got)
	}
}

// TestScriptStrSelfRef verifies str() in a script cannot crash the host on
// self-referential values. m["self"] unwraps to the GoMap itself (empty
// interfaces are unwrapped to their dynamic value), so both str(m) and
// str(m["self"]) format the cyclic map safely.
func TestScriptStrSelfRef(t *testing.T) {
	m := map[string]interface{}{}
	m["self"] = m
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"m":      m,
	}
	code := []byte(`
s1 = str(m)
s2 = str(m["self"])
assert.Eq("cyclic" in s1, True)
assert.Eq("cyclic" in s2, True)
`)
	if _, err := starlight.Eval(code, globals, nil); err != nil {
		t.Fatal(err)
	}
}

// TestStringFormattingUnchanged pins the exact previous formatting for
// ordinary (acyclic) values.
func TestStringFormattingUnchanged(t *testing.T) {
	if got := convert.NewGoMap(map[string]int{"a": 1}).String(); got != "map[a:1]" {
		t.Fatalf("map formatting changed: %q", got)
	}
	sv, err := convert.ToValue([]int{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if got := sv.String(); got != "[1 2 3]" {
		t.Fatalf("slice formatting changed: %q", got)
	}
	type pair struct {
		A int
		B string
	}
	pv, err := convert.ToValue(pair{A: 1, B: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	if got := pv.String(); got != "{1 bob}" {
		t.Fatalf("struct formatting changed: %q", got)
	}
}

// TestStringDeepNesting verifies that absurdly deep (but finite) values are
// elided rather than allowed to recurse toward a stack overflow.
func TestStringDeepNesting(t *testing.T) {
	var nest interface{} = []interface{}{}
	for i := 0; i < 500; i++ {
		nest = []interface{}{nest}
	}
	v, err := convert.ToValue(nest)
	if err != nil {
		t.Fatal(err)
	}
	got := v.String()
	if !strings.Contains(got, "cyclic") {
		t.Fatalf("expected deep value to be elided, got %d chars", len(got))
	}
}
