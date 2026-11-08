# Usage examples

These examples use the API in this checkout. For a separate application, create
an empty directory, run `go mod init example.com/starlight-demo`, then add the
Starlight version you intend to use with `go get github.com/1set/starlight@VERSION`.
Replace `VERSION` with a release or commit; `go list -m github.com/1set/starlight`
shows the selected version. Run each example in its own directory.

## Direct conversion

Save this complete program as `main.go` and run `go run .`:

```go
package main

import (
	"fmt"
	"log"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
)

type Contact struct {
	Name     string `json:"name"`
	Internal string `json:"-"`
}

func (c *Contact) Greet() string {
	return "Hello, " + c.Name + "!"
}

func main() {
	contact := Contact{Name: "Ada", Internal: "host-only field"}
	value, err := convert.ToValueWithTag(&contact, "json")
	if err != nil {
		log.Fatal(err)
	}
	result, err := starlight.Eval([]byte(`
contact.name = "Lin"
message = contact.Greet()
`), map[string]interface{}{"contact": value}, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result["message"])
	fmt.Println(contact.Name)
}
```

Output:

```text
Hello, Lin!
Lin
```

`Eval` accepts the already converted `starlark.Value` without wrapping it again.
The pointer makes the field write visible to Go. Use `convert.ToValue(&contact)`
when no custom field-tag name is needed. The `json` tag selects `name`
and hides `Internal`; without a custom tag name, the default is `starlark`.
Tags affect fields, while exported methods such as `Greet` remain callable.
A method can have host effects even when its wrapper has been frozen. Pass only
host data and methods that the script is allowed to use.

## Files and modules

Create these three files in the example directory:

`main.go`:

```go
package main

import (
	"fmt"
	"log"

	"github.com/1set/starlight"
)

func main() {
	cache, err := starlight.WithGlobals(map[string]interface{}{
		"prefix": "Hello, ",
	}, "scripts")
	if err != nil {
		log.Fatal(err)
	}
	for _, name := range []string{"Ada", "Lin"} {
		result, err := cache.Run("main.star", map[string]interface{}{"name": name})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(result["message"])
	}
}
```

`scripts/main.star`:

```python
load("greetings.star", "greet")
message = greet(name)
```

`scripts/greetings.star`:

```python
def greet(name):
    return prefix + name + "!"
```

Run `go run .` from the example directory. Output:

```text
Hello, Ada!
Hello, Lin!
```

`prefix` is a predeclared name in loaded modules because it was supplied to
`WithGlobals`; `name` is a per-call input to `main.star` from `Run`. A `Run` input
is not automatically available inside a loaded module. Both calls reuse the
compiled main script and initialized module but use the current `name` value.

After editing script files, call `cache.Reset()` before the next `Run` to reload
all scripts and modules. `cache.Forget("greetings.star")` only removes that
filename's entries; it does not refresh other modules that already loaded it.
Use `New("scripts")` when loaded modules need no extra predeclared names.

See the [README](README.md#script-inputs-and-results) for source input forms,
return values, error behavior and the host-data boundary.
