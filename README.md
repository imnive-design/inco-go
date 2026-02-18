# Inco

Invisible constraints. Invincible code.

Inco is a Design-by-Contract (DbC) toolkit for Go. Define execution protocols using incognito comments; they are transformed into runtime assertions at build time via `-overlay`.

## Philosophy

Business logic should be pure. Defensive noise (`if x == nil`, `if err != nil`) belongs in the shadow, not in your source.

Write the intent; Inco generates the shield.

### `if` is for logic, not for guarding

In an Inco codebase, `if` statements should express **logic flow** — branching on business conditions, selecting behavior, iterating exit criteria. They should *not* be used for:

- **Nil/zero guards** → use `@require -nd` or `@require -ret -nd`
- **Value-range validation** → use `@require <expr>`
- **Error propagation** (`if err != nil { return err }`) → use `// @must -ret`
- **Error wrapping** (`if err != nil { return fmt.Errorf(...) }`) → use `// @must -ret(fmt.Errorf(...))`
- **Silent error drops** (`if err != nil { return }`) → use `@require -log` or `// @must -ret`

When every defensive check is a directive, the remaining `if` statements in your code carry **real** semantic weight — they represent genuine decisions, not boilerplate.

## Directives

```go
func Transfer(from *Account, to *Account, amount int) {
    // @require -nd from, to
    // @require amount > 0, "amount must be positive"
    
    res, _ := db.Exec(query) // @must
    
    // @ensure -nd res
}
```

| Directive | Meaning |
|-----------|---------|
| `// @require -nd var1, var2` | Precondition: variables must not be zero-valued (type-aware) |
| `// @require <expr>, "msg"` | Precondition: expression must be true |
| `// @require -ret -nd var` | Precondition: return zero values on violation (instead of panic) |
| `// @require -ret(e1, e2) -nd var` | Precondition: return custom expressions on violation |
| `// @require -log -nd var` | Precondition: log message and return on violation |
| `// @ensure -nd var`         | Postcondition (via `defer`): must not be zero-valued at return |
| `// @must`                   | Execution assert: error must be nil, panic otherwise |\n| `// @must -ret`              | Error propagation: return error instead of panicking |\n| `// @must -ret(e1, e2)`      | Error handling: return custom expressions on error |

After running `inco gen`, the above is transformed into a shadow file in `.inco_cache/`:

```go
func Transfer(from *Account, to *Account, amount int) {
    // @require -nd from, to  →
    if from == nil {
        panic("inco // require -nd violation: [from] is defaulted (nil) at transfer.go:24")
    }
    if to == nil {
        panic("inco // require -nd violation: [to] is defaulted (nil) at transfer.go:24")
    }

    // @require amount > 0, "amount must be positive"  →
    if !(amount > 0) {
        panic("amount must be positive at transfer.go:25")
    }

    // res, _ := db.Exec(query) // @must  →
    res, _inco_err_28 := db.Exec(query)
    if _inco_err_28 != nil {
        panic("inco // must violation at transfer.go:28: " + _inco_err_28.Error())
    }

    // @ensure -nd res  →
    defer func() {
        if res == nil {
            panic("inco // ensure -nd violation: [res] is defaulted (nil) at transfer.go:30")
        }
    }()

    fmt.Printf("Transfer %d from %s to %s, affected %d rows\n", amount, from.ID, to.ID, res.RowsAffected)
}
```

Your source stays clean — the shadow files live in `.inco_cache/` and are wired in via `go build -overlay`.

## Soft Contracts: `-ret` and `-ret(...)`

Not every violation warrants a panic. Use `-ret` to return on violation instead:

```go
// Return zero values on violation (silent guard)
func SafeGet(db *DB, id string) (*User, error) {
    // @require -ret -nd db
    // @require -ret len(id) > 0
    return db.Query("SELECT * FROM users WHERE id = ?")
}

// Return custom expressions on violation
func FindUser(db *DB, id string) (*User, error) {
    // @require -ret(nil, ErrNotFound) -nd db
    // @require -ret(nil, fmt.Errorf("invalid id: %s", id)) len(id) > 0
    return db.Query("SELECT * FROM users WHERE id = ?")
}

// Log + return on violation (auto-imports "log")
func Process(order *Order) (result *Receipt) {
    // @require -log -nd order
    return
}
```

`@must` also supports `-ret` for error propagation:

```go
// Replace `if err != nil { return err }` boilerplate
func ProcessFile(path string) error {
    f, _ := os.Open(path) // @must -ret
    defer f.Close()
    return nil
}

// Custom return expressions on error
func Fetch(db *DB) (string, error) {
    res, _ := db.Query("SELECT 1") // @must -ret("", ErrNotFound)
    return res, nil
}
```

Flags are order-insensitive and composable:
```go
// @require -ret -nd x              // return zero values if x is defaulted
// @require -ret(nil, err) -nd x    // return custom expressions if x is defaulted
// @require -log -nd x              // log + return (implies -ret)
// @require -log -ret("", err) expr // log + return custom expressions
```

## Type-Aware Zero-Value Checks

`-nd` generates **type-specific** zero-value assertions using `go/types` analysis:

| Type | Generated condition |
|------|--------------------|
| pointer / slice / map / chan / func / interface | `var == nil` |
| string | `var == ""` |
| integer | `var == 0` |
| float | `var == 0.0` |
| bool | `!var` |
| comparable struct | `var == (T{})` |
| comparable array | `var == ([N]T{})` |
| comparable type param | `var == *new(T)` |
| non-comparable type param (`any`) | `reflect.ValueOf(&var).Elem().IsZero()` |

Panic messages include the zero-value description for easier debugging:
```
inco // require -nd violation: [name] is defaulted (empty string) at main.go:10
inco // require -nd violation: [count] is defaulted (zero) at main.go:11
```

## Generics Support

Inco fully supports generic functions and types. The zero-value check strategy is chosen based on the type parameter's constraint:

```go
// comparable constraint → generates: result == *new(T)
func FirstNonZero[T comparable](items []T) (result T) {
    // @ensure -nd result
    for _, v := range items {
        return v
    }
    return
}

// any constraint → generates: reflect.ValueOf(&v).Elem().IsZero()
// (import "reflect" is auto-added)
func MustNotBeZero[T any](v T) T {
    // @require -nd v
    return v
}

// Expression mode works with type params too
func Clamp[N Number](val, lo, hi N) N {
    // @require lo <= hi, "lo must not exceed hi"
    ...
}
```

## Usage

```bash
# Install
go install github.com/incognito-design/inco/cmd/inco@latest

# Generate overlay
inco gen

# Build / Test / Run with contracts enforced
inco build ./...
inco test ./...
inco run .

# Contract coverage audit
inco audit .
```

## Build from source

Inco is self-bootstrapping — it uses its own contract directives in its source code.

```bash
# Two-stage build:
make build
# Stage 0: plain go build → bin/inco-bootstrap (no contracts)
# Stage 1: bootstrap generates overlay → compiles with contracts → bin/inco

# Other targets:
make test       # Run tests with contracts enforced
make clean      # Remove .inco_cache/ and bin/
make install    # Install to $GOPATH/bin
```

## How it works

1. `inco gen` scans all `.go` files for `@require`, `@ensure`, `@must` comments
2. Type-checks each package via `go/types` for type-aware assertion generation
3. Generates shadow files with injected assertions in `.inco_cache/`
4. Injects `//line` directives so panic stack traces point back to original source lines
5. Produces `overlay.json` for `go build -overlay`
6. Source files remain untouched — zero invasion

At gen time, constant `@require` expressions that evaluate to `false` are detected and warnings are emitted.

## Project structure

```
cmd/inco/           CLI entry point
internal/inco/      Core engine:
  audit.go            Contract coverage auditing
  contract.go         Directive parsing
  engine.go           AST injection, overlay generation, //line mapping
  typecheck.go        Type resolution, zero-value checks, generics support
example/            Demo files:
  demo.go             Basic directives
  transfer.go         Full directive set
  edge_cases.go       Closures, multi-line @must, -ret/-ret(...)/-log, @must -ret
  generics.go         Type parameters: comparable, any, mixed, expression mode
```

## Design

- **Zero-invasive**: No custom extensions, no broken IDE support
- **Zero-overhead option**: Strip via `-tags` in production, or keep for fail-fast
- **Self-bootstrapping**: Inco is built with Inco — its own source uses `@require` and `@must`
- **Cache-friendly**: Content-hash based filenames for stable build cache
- **Type-aware**: Full `go/types` integration — smart zero-value checks for all Go types including generics
- **Source-mapped**: `//line` directives in shadow files preserve original file:line in stack traces

## License

MIT
