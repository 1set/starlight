// Package starlight provides a convenience wrapper around go.starlark.net/starlark.
package starlight

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/1set/starlight/convert"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// dialectOptions is the Starlark dialect starlight compiles with: the
// standard language plus the 'set' built-in. It is passed explicitly to
// every compile/exec call instead of mutating the process-global resolve
// flags, so importing this package has no side effects on other Starlark
// users in the same process. (Nested def, lambda, float, and bitwise
// operations are part of the standard dialect already.)
var dialectOptions = &syntax.FileOptions{Set: true}

// LoadFunc is a function that tells starlark how to find and load other scripts
// using the load() function.  If you don't use load() in your scripts, you can pass in nil.
type LoadFunc func(thread *starlark.Thread, module string) (starlark.StringDict, error)

// Eval evaluates the starlark source with the given global variables. The type
// of the argument for the src parameter must be string (filename), []byte, or io.Reader.
func Eval(src interface{}, globals map[string]interface{}, load LoadFunc) (map[string]interface{}, error) {
	dict, err := convert.MakeStringDict(globals)
	if err != nil {
		return nil, err
	}
	thread := &starlark.Thread{
		Load: load,
	}
	filename, ok := src.(string)
	if ok {
		dict, err = starlark.ExecFileOptions(dialectOptions, thread, filename, nil, dict)
	} else {
		dict, err = execNonFileSource(thread, src, dict)
	}
	if err != nil {
		return nil, err
	}
	return convert.FromStringDict(dict), nil
}

// execNonFileSource runs a non-filename source ([]byte or io.Reader). It
// recovers panics from the interpreter's source reader — e.g. a host passing
// a typed-nil io.Reader — and returns them as a clean error.
func execNonFileSource(thread *starlark.Thread, src interface{}, dict starlark.StringDict) (out starlark.StringDict, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = nil, fmt.Errorf("starlight: cannot read source: %v", r)
		}
	}()
	return starlark.ExecFileOptions(dialectOptions, thread, "eval.sky", src, dict)
}

// Cache is a cache of scripts to avoid re-reading files and re-parsing them.
type Cache struct {
	_       convert.DoNotCompare
	dirs    []string
	cache   *cache
	mu      sync.Mutex
	scripts map[string]*starlark.Program
}

func run(p *starlark.Program, globals map[string]interface{}, load LoadFunc) (map[string]interface{}, error) {
	g, err := convert.MakeStringDict(globals)
	if err != nil {
		return nil, err
	}
	ret, err := p.Init(&starlark.Thread{Load: load}, g)
	if err != nil {
		return nil, err
	}
	return convert.FromStringDict(ret), nil
}

// New returns a Starlight Cache that looks in the given directories for plugin
// files to run.  The directories are searched in order for files when Run is
// called.  Calls to the script function load() will also look in these
// directories. This function will panic if you give it no directories.
func New(dirs ...string) *Cache {
	if len(dirs) == 0 {
		panic(fmt.Errorf("no directories given"))
	}
	return newCache(dirs, nil)
}

// WithGlobals returns a new Starlight cache that passes the listed global
// values to scripts loaded with the load() script function.  Note that these
// globals will *not* be passed to individual scripts you run unless you
// explicitly pass them in the Run call.
func WithGlobals(globals map[string]interface{}, dirs ...string) (*Cache, error) {
	if len(dirs) == 0 {
		return nil, fmt.Errorf("no directories given")
	}
	g, err := convert.MakeStringDict(globals)
	if err != nil {
		return nil, err
	}
	return newCache(dirs, g), nil
}

func newCache(dirs []string, globals starlark.StringDict) *Cache {
	c := &Cache{
		dirs:    dirs,
		scripts: map[string]*starlark.Program{},
	}
	c.cache = &cache{
		cache:    make(map[string]*entry),
		readFile: c.readFile,
		globals:  globals,
	}
	return c
}

// Run looks for a file with the given filename, and runs it with the given globals
// passed to the script's global namespace. The return value is all convertible
// global variables from the script, which may include the passed-in globals.
func (c *Cache) Run(filename string, globals map[string]interface{}) (map[string]interface{}, error) {
	dict, err := convert.MakeStringDict(globals)
	if err != nil {
		return nil, err
	}
	key := scriptCacheKey(filename, dict)
	c.mu.Lock()
	if p, ok := c.scripts[key]; ok {
		c.mu.Unlock()
		return run(p, globals, c.Load)
	}
	c.mu.Unlock()

	b, err := c.readFile(filename)
	if err != nil {
		return nil, err
	}
	_, p, err := starlark.SourceProgramOptions(dialectOptions, filename, b, dict.Has)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.scripts[key] = p
	c.mu.Unlock()
	return run(p, globals, c.Load)
}

// scriptCacheKey composes the key under which a compiled program is cached.
// Compilation resolves each identifier as predeclared vs global via the
// predeclared name set (dict.Has), so the same file compiled under a
// different set of global names is a different program: keying on the
// filename alone returned a stale program that failed at run time with
// "internal error: predeclared variable X is uninitialized" — or silently
// reused a program resolved under the wrong name set. The dialect is fixed
// (dialectOptions) for every compile here, so only the filename and the
// sorted predeclared names need to participate.
func scriptCacheKey(filename string, dict starlark.StringDict) string {
	names := make([]string, 0, len(dict))
	for n := range dict {
		names = append(names, n)
	}
	sort.Strings(names)
	return filename + "\x00" + strings.Join(names, "\x00")
}

// Load loads a module using the cache's configured directories.
func (c *Cache) Load(_ *starlark.Thread, module string) (starlark.StringDict, error) {
	return c.cache.Load(module)
}

func (c *Cache) readFile(filename string) ([]byte, error) {
	var err error
	var b []byte
	for _, d := range c.dirs {
		full := filepath.Join(d, filename)
		// Containment: filepath.Join cleans embedded ".." segments, so a
		// script-controlled name like "../secret.star" (Run and the load()
		// path both reach here) can resolve above d and read a sibling or
		// parent file. Skip any candidate that escapes d; a name that stays
		// within a *different* configured dir is still served from that one,
		// and only a name that escapes every dir falls through to not-found.
		// This is defense in depth — confinement is not a guarantee of New()
		// (real sandboxing belongs to the host layer), but the search scope
		// should not silently reach outside the directories it was given.
		if !withinDir(d, full) {
			continue
		}
		b, err = ioutil.ReadFile(full)
		if err == nil {
			return b, nil
		}
	}
	// guaranteed to have at least one directory, so there should be at least
	// not found error here.
	return nil, fmt.Errorf("cannot find file %q in any of the configured directories %q", filename, c.dirs)
}

// withinDir reports whether the cleaned path full is dir itself or lives
// under it. A path that climbs out of dir via ".." (e.g. dir/../secret) is
// rejected. Both paths are cleaned before comparison.
func withinDir(dir, full string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(full))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Reset clears all cached scripts.
func (c *Cache) Reset() {
	c.mu.Lock()
	c.scripts = map[string]*starlark.Program{}
	c.cache.reset()
	c.mu.Unlock()
}

// Forget clears the cached script for the given filename.
func (c *Cache) Forget(filename string) {
	c.mu.Lock()
	c.cache.remove(filename)
	// Run keys c.scripts by filename + predeclared name set (see
	// scriptCacheKey), so a single file may have several entries — one per
	// distinct global-name set it was run under. Every such key begins with
	// "filename\x00"; drop them all.
	prefix := filename + "\x00"
	for k := range c.scripts {
		if strings.HasPrefix(k, prefix) {
			delete(c.scripts, k)
		}
	}
	c.mu.Unlock()
}
