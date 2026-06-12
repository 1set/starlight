package starlight

import (
	"os"
	"path/filepath"
	"testing"
)

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
