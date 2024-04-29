# :star2: Starlight Enhanced - Bridging Go and Starlark

[![Go Reference](https://pkg.go.dev/badge/github.com/1set/starlight.svg)](https://pkg.go.dev/github.com/1set/starlight)
[![codecov](https://codecov.io/github/1set/starlight/branch/master/graph/badge.svg?token=yDu7JCcMHv)](https://codecov.io/github/1set/starlight)
[![codacy](https://app.codacy.com/project/badge/Grade/211835be0f0241269e38fd8913648e1e)](https://app.codacy.com/gh/1set/starlight/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_grade)
[![codeclimate](https://api.codeclimate.com/v1/badges/20035dc9703387ad14c6/maintainability)](https://codeclimate.com/github/1set/starlight/maintainability)
[![goreportcard](https://goreportcard.com/badge/github.com/1set/starlight)](https://goreportcard.com/report/github.com/1set/starlight)

Welcome to *Starlight Enhanced*, a sophisticated fork of the original [*Starlight*](https://github.com/starlight-go/starlight) project. Our version builds upon the *Starlight* wrapper for [*Starlark in Go*](https://github.com/google/starlark-go) to facilitate smoother data conversion between Go and Starlark.

Due to the lack of ongoing maintenance of the original [*Starlight*](https://github.com/starlight-go/starlight/commits/master) project, we saw an opportunity to breathe new life into it. We've optimized it to work seamlessly with the latest versions of *Starlark in Go* while addressing and rectifying bugs present in the original repository.

The objectives of this enhanced fork include:

- Identification and resolution of bugs and corner cases that were present in the original repository.
- Extension of the library's capabilities by exposing additional functions, thereby enriching functionality.
- Ensuring compatibility and support for the latest versions of *Starlark in Go*.

We hope that this improved iteration of *Starlight* will contribute to your projects and enhance your experience with Starlark. Your contributions and feedback are always welcome.

## Features

A set of powerful features is provided to facilitate the integration of Starlark scripts into Go applications:

### Seamless Data Conversion

Starlight offers robust support for seamless data conversion between Go and Starlark types. Conversion functions are provided through the [`convert`](https://pkg.go.dev/github.com/1set/starlight/convert) package.

Leveraging the [`convert.ToValue`](https://pkg.go.dev/github.com/1set/starlight/convert#ToValue) and [`convert.FromValue`](https://pkg.go.dev/github.com/1set/starlight/convert#FromValue) utilities, Starlight enables the smooth transition of Go's rich data types and methods into the Starlark scripting environment. This feature supports a wide array of Go types, including structs, slices, maps, and functions, making them readily accessible and manipulable within Starlark scripts. The only exceptions are Go channels and complexes, which are not supported due to their concurrency nature.

### Efficient Caching Mechanism

Starlight introduces an efficient caching mechanism that significantly optimizes script execution performance. By caching scripts upon their first execution, Starlight minimizes the overhead associated with re-reading and re-parsing files in subsequent runs. This caching is compliant with the Starlark `Thread.Load` standards, ensuring that scripts are efficiently loaded and executed while adhering to Starlark's loading semantics. This feature is particularly beneficial for applications that frequently execute Starlark scripts, as it dramatically reduces execution times and resource consumption.

### Simplified Script Execution

The [`Eval`](https://pkg.go.dev/github.com/1set/starlight#Eval) function encapsulates the complexities of setting up and executing Starlark scripts, providing a streamlined interface for developers. This encapsulation allows for easy execution of Starlark scripts with full access to the script's global variables, enhancing script interoperability and flexibility. Additionally, Starlight supports the Starlark `load()` function, enabling scripts to load and execute other scripts seamlessly. This feature simplifies the integration of Starlark scripting into Go applications, reducing the need for repetitive boilerplate code and fostering a more efficient development process.

## Installation

To install *Starlight Enhanced*, use the following Go command under your project directory:

```bash
go get github.com/1set/starlight
```

## Usage

*Starlight* can be used to easily integrate Starlark scripting into your Go applications. Here's a quick example:

```go
import "github.com/1set/starlight"

// Define your Go function
name := "Starlight"
globals := map[string]interface{}{
    "target": name,
    "greet": func(name string) string {
        return fmt.Sprintf("Hello, %s!", name)
    },
}

// Run a Starlark script with the global variables
script := `text = greet(target); print("Starlark:", text)`
res, err := starlight.Eval([]byte(script), globals, nil)

// Check for errors and results
if err != nil {
    fmt.Println("Error executing script:", err)
    return
}
fmt.Println("Go:", res["text"].(string))
```

The `convert` package can be used to convert data between Go and Starlark, making it simpler to pass data and functions back and forth between the two contexts. Here's an example of converting a Go struct to a Starlark value and modifying it in a script:

```go
import (
	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
)

// Define your Go data structure
type Contact struct {
    Name  string
    Email string
    Age   uint
}
contact := Contact{Name: "Bob", Email: "bob@example.com", Age: 30}

// Convert Go data structure to Starlark value
starlarkValue, err := convert.ToValue(&contact)
if err != nil {
    panic(err)
}
globals := map[string]interface{}{
    "candidate": "Leon",
    "contact":   starlarkValue,
}

// Run a Starlark script with the global variables
script := `
contact.Name = "".join(reversed(candidate.codepoints())).title()
contact.Age += 2
summary = "%s [%d] %s" % (contact.Name, contact.Age, contact.Email)
`
res, err := starlight.Eval([]byte(script), globals, nil)

// Check for errors, results and modified data
if err != nil {
    fmt.Println("Error executing script:", err)
    return
}
fmt.Println("Updated:", contact)
fmt.Println("Summary:", res["summary"].(string))
```

## Contributing

We welcome contributions to the *Starlight Enhanced* project. If you encounter any issues or have suggestions for improvements, please feel free to open an issue or submit a pull request. Before undertaking any significant changes, please let us know by filing an issue or claiming an existing one to ensure there is no duplication of effort.

## License

*Starlight Enhanced* is licensed under the [MIT License](LICENSE).

## Credits

This project is a fork of the original [*Starlight*](https://github.com/starlight-go/starlight) project, authored by Nate Finch ([@natefinch](https://github.com/natefinch)). We would like to thank Nate and all the original authors and contributors for laying the foundation upon which this project builds, *Starlight Enhanced* would not have been possible without the original creation and development by Nate Finch 🎉

For historical reference, the original README from the Starlight project is preserved as [README-old.md](README-old.md) in this repository.
