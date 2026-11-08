# :star2: Starlight Enhanced - Bridging Go and Starlark

[![Go Reference](https://pkg.go.dev/badge/github.com/1set/starlight.svg)](https://pkg.go.dev/github.com/1set/starlight)
[![codecov](https://codecov.io/github/1set/starlight/branch/master/graph/badge.svg?token=yDu7JCcMHv)](https://codecov.io/github/1set/starlight)
[![codacy](https://app.codacy.com/project/badge/Grade/211835be0f0241269e38fd8913648e1e)](https://app.codacy.com/gh/1set/starlight/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_grade)
[![codeclimate](https://api.codeclimate.com/v1/badges/20035dc9703387ad14c6/maintainability)](https://codeclimate.com/github/1set/starlight/maintainability)
[![goreportcard](https://goreportcard.com/badge/github.com/1set/starlight)](https://goreportcard.com/report/github.com/1set/starlight)

Starlight runs [Starlark](https://github.com/google/starlark-go) scripts from Go
and bridges Go values and functions into them. Use `Eval` for one-off scripts,
`Cache` for file-backed scripts and modules, or the
[`convert`](https://pkg.go.dev/github.com/1set/starlight/convert) package directly.

## Installation

```sh
go get github.com/1set/starlight
```

The module's Go floor is **1.19**. Its interpreter requirement is
`go.starlark.net v0.0.0-20260324133313-ffb3f39dd27a`. Another dependency can select
a newer interpreter; check your application's selection with
`go list -m go.starlark.net`. Build production applications with a supported Go
toolchain containing current security fixes.

This README and [usage examples](USAGE.md) describe this checkout. The
[published API reference](https://pkg.go.dev/github.com/1set/starlight) may show a
different released version; the current [runner](starlight.go) and
[conversion](convert/conv.go) APIs are in this source tree. In your application
module, inspect the selected version and its local documentation with:

```sh
go list -m github.com/1set/starlight
go doc github.com/1set/starlight
go doc github.com/1set/starlight/convert
```

Use host-selected, reviewed source and modules. The retained interpreter has a
known parser recursion limitation, and this package does not provide process or
resource isolation. See [SECURITY.md](SECURITY.md) for the deployment boundary.

## Quick start

This complete program calls a Go function and updates an exported Go struct
field through a pointer:

```go
package main

import (
	"fmt"

	"github.com/1set/starlight"
)

type Contact struct {
	Name string
}

func main() {
	contact := Contact{Name: "Ada"}
	globals := map[string]interface{}{
		"contact": &contact,
		"greet": func(name string) string {
			return "Hello, " + name + "!"
		},
	}
	result, err := starlight.Eval([]byte(`
contact.Name = "Lin"
text = greet(contact.Name)
count = 2 + 3
`), globals, nil)
	if err != nil {
		fmt.Println("script error:", err)
		return
	}
	fmt.Println(result["text"]) // Hello, Lin!
	fmt.Println(contact.Name)   // Lin
	count := result["count"]
	fmt.Printf("%v (%T)\n", count, count) // 5 (int64)
}
```

## Script inputs and results

[`Eval(src, globals, load)`](https://pkg.go.dev/github.com/1set/starlight#Eval)
executes a whole script, including statements and function definitions.

| Argument | Meaning |
| --- | --- |
| `src` as `string` | A **filename**, not source text. |
| `src` as `[]byte` or `io.Reader` | Script source text. |
| `globals` | Predeclared input names and Go values; `nil` means none. |
| `load` | Resolver for `load()`; `nil` leaves module loading unavailable. |

The returned `map[string]interface{}` contains bindings defined by the script,
converted through `convert.FromValue`. Input names are not automatically copied
into the result. Functions and custom values without a native Go conversion
remain `starlark.Value` objects. Conversion, parsing, loading and execution
failures return an `error`; host changes made before an error are not rolled
back. `Eval` compiles on every call and does not use `Cache` automatically.

## Files, modules and caching

```go
cache := starlight.New("./scripts")
result, err := cache.Run("main.star", map[string]interface{}{"name": "Ada"})
```

`New` searches directories in order and panics if none are supplied. `Run`
caches compiled scripts by filename and input-name set, then executes the
script on each call with the current input values. Its `load()` calls use the
same directory search; loaded modules cache their initialized exports or errors.
Editing files does not automatically invalidate either cache.

`cache.Forget("main.star")` invalidates that filename's script/module entries.
It does not invalidate modules that already loaded it; use `cache.Reset()` to
refresh the whole cache. For one-off execution with the same loader, pass
`cache.Load` as the third argument to `Eval`.

[`WithGlobals(moduleGlobals, dirs...)`](https://pkg.go.dev/github.com/1set/starlight#WithGlobals)
returns a cache with predeclared values for **loaded modules**. They are separate
from the per-call globals supplied to `Run`; pass values to both when needed.
Unlike `New`, it returns an error for an empty directory list. See the complete
[file and module example](USAGE.md#files-and-modules), including both script files.

## Conversion and host data

`convert.ToValue` returns `(starlark.Value, error)`;
`convert.ToValueWithTag` also selects the struct-field tag name.
The [direct conversion example](USAGE.md#direct-conversion) shows custom field
tags and a callable Go method. `convert.FromValue` returns the corresponding Go
value:

| Starlark value | Go result |
| --- | --- |
| `None`, bool, float, string | `nil`, `bool`, `float64`, `string` |
| Integer | `int64`, then `uint64`, then `*big.Int`, by magnitude |
| List / tuple | `[]interface{}` |
| Bytes | `[]byte` |
| Dict | `map[interface{}]interface{}` |
| Set | `map[interface{}]bool` (`FromSetToSlice` returns an insertion-ordered slice) |
| `time.time` / `time.duration` | `time.Time` / `time.Duration` |
| Go wrapper | Its underlying Go value |

Conversion is not a JSON encoding or a guarantee of identical round-trip types.
Go maps, slices and structs use live wrappers: scripts can change shared Go
data where it is writable. Pass a struct pointer to allow exported-field writes;
Go arrays are copied to slices. A Go `[]byte` is a slice wrapper, not native
Starlark `bytes`. Wrapper `Freeze` is shallow; it neither copies host data nor
synchronizes host access. The host must coordinate concurrent access.

Exported Go methods are callable from scripts. Field tags rename or hide fields;
they do not hide methods. Neither tags nor wrapper `Freeze` restrict the effects
of a Go method, which may still change host state.

Channels, complex numbers, `unsafe.Pointer` and `uintptr` have no direct
conversion. Statically unsupported collection element types are rejected;
unsupported dynamic values inside interface collections can remain opaque
wrappers. Existing `starlark.Value` objects pass through, except typed-nil
pointers become `None`.

Wrapped Go functions accept **positional arguments only**, including variadics.
A non-nil final `error` becomes a script error; after removing that error result,
zero, one or multiple return values become `None`, one value or a tuple.

## Contributing and license

Open an issue before substantial changes to avoid duplicate work. Contributions
are welcome under the [MIT License](LICENSE).

This is a fork of [Starlight](https://github.com/starlight-go/starlight), created
by Nate Finch and its contributors. [README-old.md](README-old.md) preserves the
upstream documentation as historical reference.
