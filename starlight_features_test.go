package starlight

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Root-package features added during hardening.
//
// Sections:
//   1. Dialect: no global resolve.* mutation on import; set/lambda/float/
//      bitwise/nested-def work; cache compile paths
//   2. WithGlobals constructor (load()-module globals)
//   3. Golden end-to-end .star scripts (testdata/golden/*.star) that exercise
//      the conversion behaviors through the real interpreter
//   4. Cache-key isolation by predeclared name set

// Importing starlight (and, transitively, convert) must not mutate any
// process-global state: the dialect is passed explicitly to every
// compile/exec call. On the current go.starlark.net baseline the historic
// resolve.* flags are deprecated constants (the set built-in joined the
// standard dialect), so there is nothing left to assert about them — the
// guarantee is structural (no init functions exist) and the capability
// side is pinned by TestDialectCapabilities and TestCacheDialect below.

// TestDialectCapabilities verifies the starlight entry points still compile
// the full dialect (set built-in plus the standard nested def / lambda /
// float / bitwise features) without the global flags.
func TestDialectCapabilities(t *testing.T) {
	res, err := Eval([]byte(`
s = set([1, 2, 3])
n = len(s)
f = (lambda x: x * 2)(21)
fl = 1.5 * 2
b = 6 & 3

def outer():
    def inner():
        return 1
    return inner()

d = outer()
`), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]interface{}{
		"n":  int64(3),
		"f":  int64(42),
		"fl": float64(3),
		"b":  int64(2),
		"d":  int64(1),
	} {
		if res[k] != want {
			t.Fatalf("expected %s == %v, got %v (%T)", k, want, res[k], res[k])
		}
	}
}

// TestCacheDialect verifies the cache's compile paths — Run's
// SourceProgram compile and load()'s ExecFile — use the same explicit
// dialect (the set built-in must work in both the entry script and a
// loaded module).
func TestCacheDialect(t *testing.T) {
	dir := t.TempDir()
	module := `
loaded = set(["x", "y"])
count = len(loaded)
`
	script := `
load("mod.star", "count")
s = set([1, 2, 3])
total = len(s) + count
`
	if err := os.WriteFile(filepath.Join(dir, "mod.star"), []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.star"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	c := New(dir)
	res, err := c.Run("main.star", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res["total"] != int64(5) {
		t.Fatalf("expected total 5, got %v (%T)", res["total"], res["total"])
	}

	// second run takes the cached-program path
	res, err = c.Run("main.star", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res["total"] != int64(5) {
		t.Fatalf("expected cached total 5, got %v", res["total"])
	}
}

// TestWithGlobals covers the WithGlobals Cache constructor: the only public
// entry point for passing globals into load()-ed modules. It had no test.
func TestWithGlobals(t *testing.T) {
	dir := t.TempDir()
	// the loaded module reads a global supplied via WithGlobals, not via Run
	module := `result = greet(name)`
	main := `
load("mod.star", "result")
out = result
`
	if err := os.WriteFile(filepath.Join(dir, "mod.star"), []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.star"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}

	globals := map[string]interface{}{
		"name":  "world",
		"greet": func(n string) string { return "hello " + n },
	}
	c, err := WithGlobals(globals, dir)
	if err != nil {
		t.Fatal(err)
	}
	// main.star is run with NO per-run globals; name/greet must come from the
	// load()-module globals wired by WithGlobals
	res, err := c.Run("main.star", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res["out"] != "hello world" {
		t.Fatalf("expected 'hello world', got %v", res["out"])
	}
}

// TestWithGlobalsNoDirs verifies the constructor's error path.
func TestWithGlobalsNoDirs(t *testing.T) {
	if _, err := WithGlobals(map[string]interface{}{"a": 1}); err == nil {
		t.Fatal("expected error when no directories are given")
	}
}

// TestWithGlobalsConvertError verifies an unconvertible global surfaces from
// the constructor rather than panicking.
func TestWithGlobalsConvertError(t *testing.T) {
	if _, err := WithGlobals(map[string]interface{}{"bad": make(chan int)}, t.TempDir()); err == nil {
		t.Fatal("expected conversion error for an unsupported global type")
	}
}

// ---- Section 3: golden end-to-end .star scripts ----

// runGolden reads testdata/golden/<file>.star, runs it through Eval with the
// given globals, and returns the resulting global namespace. A script error
// fails the test (the scripts self-check via their out_* globals, asserted
// by the caller).
func runGolden(t *testing.T, file string, globals map[string]interface{}) map[string]interface{} {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", "golden", file))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Eval(src, globals, nil)
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	return res
}

// TestGoldenConversion runs testdata/golden/conversion.star against real Go
// values and asserts the conversion behaviors hold end-to-end (deterministic
// order, empty-interface unwrapping, tuple/big-int keys, str safety).
func TestGoldenConversion(t *testing.T) {
	pairs := map[interface{}]interface{}{}
	globals := map[string]interface{}{
		"nums": map[string]int{"c": 3, "a": 1, "b": 2, "e": 5, "d": 4},
		"mixed": map[string]interface{}{
			"a": 1, "b": 2, "inner": map[string]interface{}{"x": 42},
		},
		"pairs": pairs,
	}
	// seed the wrapped map with a tuple key and a big-int key via a setup script
	setup, err := Eval([]byte(`
p[(1, "a")] = "tuple-value"
p[1 << 70] = "bigint-value"
`), map[string]interface{}{"p": pairs}, nil)
	_ = setup
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	res := runGolden(t, "conversion.star", globals)
	checks := map[string]interface{}{
		"out_keys_sorted":     true,
		"out_keys_match_iter": true,
		"out_items_match":     true,
		"out_sum":             int64(3),
		"out_a_type":          "int",
		"out_eq":              true,
		"out_nested":          int64(42),
		"out_tuple_key":       "tuple-value",
		"out_bigint_key":      "bigint-value",
		"out_str_ok":          true,
	}
	for k, want := range checks {
		if res[k] != want {
			t.Errorf("conversion.star %s = %#v, want %#v", k, res[k], want)
		}
	}
}

// TestGoldenDialect runs testdata/golden/dialect.star to confirm the compiled
// dialect (set/lambda/float/bitwise/nested-def) works through the explicit
// FileOptions, with no globals supplied.
func TestGoldenDialect(t *testing.T) {
	res := runGolden(t, "dialect.star", nil)
	checks := map[string]interface{}{
		"out_set_len": int64(3),
		"out_lambda":  int64(42),
		"out_float":   float64(3),
		"out_bitwise": int64(2),
		"out_nested":  int64(7),
	}
	for k, want := range checks {
		if res[k] != want {
			t.Errorf("dialect.star %s = %#v, want %#v", k, res[k], want)
		}
	}
}

// ---- Section 4: cache-key isolation by predeclared name set ----

// TestCachePredeclaredNameSet pins the compiled-program cache key to the
// predeclared name set, not the filename alone. The same file run first with
// a global and then without it must recompile cleanly instead of reusing a
// program that resolved the name as predeclared — which failed at run time
// with "internal error: predeclared variable x is uninitialized".
func TestCachePredeclaredNameSet(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.star"), []byte("out = x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(dir)

	res, err := c.Run("p.star", map[string]interface{}{"x": 1})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if res["out"] != int64(1) {
		t.Fatalf("first run out = %v (%T), want 1", res["out"], res["out"])
	}

	// Same file, but x is no longer a predeclared name. The old filename-only
	// key reused the stale program and failed with an internal error; now it
	// recompiles and reports the honest resolve error.
	_, err = c.Run("p.star", nil)
	if err == nil {
		t.Fatal("second run with no globals: expected an error, got nil")
	}
	if strings.Contains(err.Error(), "internal error") {
		t.Fatalf("second run leaked a stale-program internal error: %v", err)
	}
	if !strings.Contains(err.Error(), "undefined: x") {
		t.Fatalf("second run error = %q, want an 'undefined: x' resolve error", err)
	}

	// Re-running with the global again must still work (its own cached
	// program for that name set).
	res, err = c.Run("p.star", map[string]interface{}{"x": 5})
	if err != nil {
		t.Fatalf("third run: %v", err)
	}
	if res["out"] != int64(5) {
		t.Fatalf("third run out = %v, want 5", res["out"])
	}
}
