package starlight

import (
	"os"
	"path/filepath"
	"testing"
)

// Root-package features added during hardening.
//
// Sections:
//   1. Dialect: no global resolve.* mutation on import; set/lambda/float/
//      bitwise/nested-def work; cache compile paths
//   2. WithGlobals constructor (load()-module globals)

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
