# CLAUDE.md

Guidance for Claude Code (claude.ai/code) working in this repository.

## What this is

`starlight` is a Go⇄Starlark value bridge: it wraps Go values so [Starlark](https://github.com/google/starlark-go) scripts can use them, and converts script results back to Go. Pure library, no main. Module floor is **Go 1.19**.

## Commands

```bash
go test ./...                                 # all tests
go test -race -count=2 ./...                  # the real bar (race + repeat)
go test ./convert/ -run TestGoMapBigIntKey    # a single test
go test ./convert/ -run xxx -bench . -count 3 # benchmarks (convert/bench_test.go)
go vet ./... && gofmt -l . convert/           # must be clean before commit

# Verify on the Go floor: the local toolchain is newer than go 1.19, and the
# pinned go.starlark.net uses maphash.String (needs >=1.19), so check behavior
# in a container before trusting it.
docker run --rm -v "$PWD":/src -v "$HOME/go/pkg/mod":/go/pkg/mod -w /src golang:1.19 go test -race -count=1 ./...
```

CI (`.github/workflows/build.yml`): Go `1.19.x`/`1.25.x` × ubuntu-22.04 / macos-14 / windows-2022, plus CodeQL and the Codecov/Codacy coverage gate. The gate is the `codecov/project`+`codecov/patch` commit statuses, not the upload step (its `continue-on-error` only tolerates upload outages).

## Downstream compatibility — a release gate

starlight is the L1 foundation; `starlet` (via `dataconv`), `starbox`, and the `starpkg/*` modules all depend on it. A change can be locally green yet break a consumer. **Before tagging a release, run the two-leg matrix** — for each downstream (at least `starlet`, then `starbox` / `starpkg/base` / `starpkg/sqlite`):

```bash
# clone the downstream fresh to /tmp, then:
#  baseline leg — its existing pins:
docker run --rm -v <dst>:/src -v $HOME/go/pkg/mod:/go/pkg/mod -w /src golang:1.19 sh -c 'go build ./... && go test ./...'
#  upgrade leg — point it at this starlight and re-tidy:
( cd <dst> && go mod edit -replace github.com/1set/starlight=<this-repo> )
docker run --rm -v <dst>:/src -v <this-repo>:/starlight-new -v $HOME/go/pkg/mod:/go/pkg/mod -w /src golang:1.19 \
  sh -c 'go mod tidy && go build ./... && go test ./...'
```

Only failures that appear in the **upgrade leg but not the baseline** are regressions. Known pre-existing (not regressions): `starlet` `lib/file`/`lib/path` (need `/etc/sudoers` / a permission barrier absent under Docker-root); `starcli` build (goldmark needs go1.22). The replace also bumps `go.starlark.net` transitively via MVS — L0-caused breaks count too, and predict what the downstream's own pin-upgrade will face. `dataconv`/`dataconv/types` are the most load-bearing consumers — they must stay green.

The `.star` module-test corpus lives at `starpkg/test/` (run via each module's harness, not from here).

## Golden end-to-end tests

`testdata/golden/*.star` are classic scripts run through the real interpreter (see `starlight_features_test.go` section 3): they exercise deterministic order, empty-interface unwrapping, tuple/big-int keys, `str()` safety, and the compiled dialect. Add a script there (and a `checks` map) when a fix has script-observable behavior — it's the cheapest end-to-end regression layer.

## Architecture

Everything is the `convert` package's bidirectional bridge:

- **`ToValue` (Go→Starlark)** — `conv.go:toValue` is a `reflect.Kind` switch. Composite Go values become lazy wrappers: `GoMap` (`map.go`), `GoSlice` (`slice.go`), `GoStruct` (`struct.go`), `GoInterface` (`interface.go`), each implementing the matching `starlark.Value` interfaces.
- **`FromValue` (Starlark→Go)** — `conv.go:fromValue` is a `starlark.Value` type switch (the inverse).
- `common.go` holds shared helpers: deterministic key sorting, the bounded type caches, cycle detection.

The root package (`starlight.go`, `cache.go`) is a thin layer: `Eval` (one-shot) and `Cache`/`New`/`WithGlobals` (file-backed scripts with `load()`). Scripts compile with an explicit `syntax.FileOptions{Set: true}` passed to every `ExecFileOptions`/`SourceProgramOptions` — never by mutating process-global `resolve.*` flags.

## Invariants — do not silently break these

Most of the work in this repo enforces properties that are easy to regress. When editing `convert/`, preserve all of:

1. **No host panics from script input.** Methods that can't return errors (`GoMap.Items/Keys/Iterate`, `GoSlice.Index`, iterators) must never reach `panic`. Unsupported *dynamic* values degrade to an opaque `GoInterface` wrapper; unsupported *static* element types are rejected up front by `checkCollectionElemTypes`.
2. **Deterministic order.** Go map iteration is randomized — route every script-visible materialization through `common.go:sortedMapKeys`, never raw `reflect.Value.MapKeys()`.
3. **Checked conversions.** Numeric overflow/narrowing, `int→string` (codepoint), and float-truncation must error, not silently corrupt (`conv.go:checkedConvert`). `None` is valid only for nullable target kinds.
4. **Comparable-by-value map keys.** Keys must compare by value: tuple→`[N]interface{}`, bytes→`[N]byte`, large int (`*big.Int`, comparable only by pointer identity)→`bigIntKey` — see `hashableGoValue`.
5. **Bounded caches.** Per-`reflect.Type` caches (`elemTypeCheckCache`, `typeCanCycleCache`) use `boundedTypeCache` with a cap, because a script can mint unbounded distinct array types (`[N]interface{}`). Never make them unbounded `sync.Map`s.
6. **Cycle/recursion safety.** `safeGoString` guards `String()` against self-referential values; `fromValue` threads a per-call `visited` set (not a package global) so concurrent conversions don't interfere.

## Type mapping — the highest-risk surface

Type-detection sites are where bugs hide (a missed `*big.Int` made large-int map keys silently unretrievable). Each conversion function **categorizes by type**, so the mapping must be complete in both directions. The tables below are the contract — keep them and the code in sync.

**Go → Starlark** (`ToValue` / `toValue`, a `reflect.Kind` switch):

| Go type | Starlark result |
|---|---|
| `bool` | `Bool` |
| `int`/`int8…64`, `uint`/`uint8…64` | `Int` |
| `float32/64` | `Float` |
| `string` | `String` |
| `map` | `*GoMap` (lazy wrapper) |
| `slice`, `array` | `*GoSlice` (array is copied to a slice) |
| `struct` | `*GoStruct` |
| `func` | `*starlark.Builtin` (callable) |
| `time.Time` / `time.Duration` | Starlark `time` / `duration` |
| pointer to basic type | dereferenced to the above |
| pointer to struct (non-nil / nil) | `*GoStruct` / `*GoInterface` |
| named scalar / non-struct pointer **with methods** | `*GoInterface` (exposes the methods; structs always use `*GoStruct`) |
| non-nil `interface{}` | unwrapped to its dynamic value |
| nil `interface{}`, invalid | `None` |
| **`chan`, `complex`, `unsafepointer`, `uintptr`** | **error — unsupported** |

Note the asymmetry: Go `[]byte` is a `uint8` slice → `*GoSlice`, **not** Starlark `Bytes`.

**Starlark → Go** (`FromValue` / `fromValue`, a type switch):

| Starlark | Go result |
|---|---|
| `None` | `nil` |
| `Bool` / `Float` / `String` / `Bytes` | `bool` / `float64` / `string` / `[]byte` |
| `Int` | `int64` → `uint64` → `*big.Int` (by magnitude; never assume it fits a machine int) |
| `List` / `Tuple` | `[]interface{}` |
| `Dict` | `map[interface{}]interface{}` |
| `Set` | `map[interface{}]bool` (or `FromSetToSlice` → ordered `[]interface{}`) |
| `time`/`duration` | `time.Time` / `time.Duration` |
| a `Go*` wrapper | its underlying Go value |
| `Function`, `Builtin`, custom value | returned as-is (the `starlark.Value`) |

**Map keys** (`hashableGoValue`) must be **comparable by value**, which is stricter than `reflect.Type.Comparable()` (pointers pass that but compare by identity): `Tuple`→`[N]interface{}`, `Bytes`→`[N]byte`, large `Int`→`bigIntKey`; other hashable values use their `FromValue` form if comparable-by-value, else **error**. `dict`/`list`/`set` keys are unhashable → error.

When you add or edit any type switch: walk it against **both tables above plus the full `reflect.Kind` set**, and for every unhandled type decide its behavior — error, correct fall-through, or the bug: **silently wrong / panic**.

## Contribution standard

- **Test-first, one fix per PR.** Write the failing/repro test, then the fix; keep them in the same PR.
- **Pass the full bar before commit:** `go test -race -count=2 ./...`, `go vet`, `gofmt -l` clean, and the Docker go1.19 run above.
- **Touching a hot path?** Run `convert/bench_test.go` and confirm no regression.
- **Changing observable behavior?** Update the test that pins the old behavior and say so in the commit; document any host-visible semantic change in the relevant godoc.
- Keep godoc accurate — comments here state *why* and the *boundary/fall-through behavior* of a type switch, not what the next line does.

## Test organization — do NOT create one file per feature

Keep the test-file count small. **Do not add a new `*_test.go` file per bugfix, feature, or section** — that is the failure mode to avoid.

- The test files that mirror a Go source file are the canonical homes — `conv_test.go`, `map_test.go`, `slice_test.go`, `struct_test.go`, `interface_test.go`, `value_test.go`, `call_test.go`, `func_test.go`, `extern_test.go`, `util_test.go`. Leave these as-is; add source-shaped tests to the matching one.
- All other (feature/regression) tests live in a **few** thematic files, grouped by functional goal, each opened with a commented list of its sections:
  - `keys_test.go` — map-key conversion + deterministic ordering
  - `conversion_test.go` — value conversion correctness/safety (checked numeric, None, unwrapping, type-mapping edges, typed-nil)
  - `robustness_test.go` — host never panics + bridge contracts + freeze/concurrency/cycles
  - `internal_test.go` — white-box (`package convert`) tests of internal helpers (caches, `comparableByValue`, `stableKeyString`, panic sentinel)
  - `bench_test.go` — benchmarks
  - root package: `starlight_test.go` (original) and `starlight_features_test.go` (dialect, `WithGlobals`)
- A new test belongs in the file whose theme it matches — add a **section** (with a header comment), not a new file. Prefer a longer, well-sectioned file over another tiny one. Only create a new test file if a genuinely new functional theme appears.

## Reply marker

End every reply with the 🌟 emoji to confirm this file was read and is being followed.
