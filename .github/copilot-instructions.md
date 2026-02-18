# Inco DSL — Copilot Instructions

## Overview

Inco is a Design-by-Contract (DbC) toolkit for Go. It uses special comment directives (`@require`, `@ensure`, `@must`) embedded in standard Go comments. These directives are parsed at build time and transformed into runtime assertions via `go build -overlay`. Source files remain untouched — generated shadow files live in `.inco_cache/`.

## Directive Reference

### `@require` — Precondition

Checked at function entry. Place at the top of a function body.

At gen time, if a `@require` expression is a compile-time constant that evaluates to `false`, Inco emits a warning.

#### Non-defaulted mode (`-nd`)

```go
// @require -nd var1, var2
```

- Asserts that each listed variable is **not zero-valued** (type-aware: uses `go/types` to generate the correct check per type).
- Multiple variables are comma-separated.
- Generates one type-appropriate zero-value check per variable:
  - pointer/slice/map/chan/func/interface → `if var == nil`
  - string → `if var == ""`
  - integer → `if var == 0`
  - float → `if var == 0.0`
  - bool → `if !var`
  - comparable struct → `if var == (T{})`
  - comparable array → `if var == ([N]T{})`
  - comparable type param → `if var == *new(T)`
  - non-comparable type param (`any`) → `if reflect.ValueOf(&var).Elem().IsZero()` (auto-imports `reflect`)

#### Expression mode

```go
// @require <expr>
// @require <expr>, "custom message"
```

- Asserts that `<expr>` evaluates to `true`.
- Generates `if !(<expr>) { panic(...) }`.
- An optional quoted string after a comma provides a custom panic message.
- If no message is given, a default message including the expression text is used.

### `@require -ret` — Precondition with Return

Like `@require`, but on violation **returns zero values** instead of panicking. Ideal for replacing `if x == nil { return }` boilerplate.

Supports optional **custom return expressions** via `-ret(expr1, expr2, ...)`:
- `-ret` (no parens): returns zero values (default)
- `-ret(expr1, expr2)`: returns the specified expressions

#### Non-defaulted mode

```go
// @require -ret -nd var1, var2
// @require -ret(nil, ErrNotFound) -nd db
```

- Same zero-value checks as `@require -nd`, but generates `return` instead of `panic`.
- For named returns: `if var == nil { return }` (bare return).
- For unnamed returns: `if var == nil { return nil, 0, "" }` (zero values auto-generated).
- With `-ret(...)`: returns the specified custom expressions instead of zero values.

#### Expression mode

```go
// @require -ret <expr>
// @require -ret <expr>, "custom message"
// @require -ret("", ErrInvalid) <expr>
// @require -ret("", ErrInvalid) <expr>, "custom message"
```

- Generates `if !(<expr>) { return }` (or `return zeroVal1, ...` for unnamed returns).
- With `-ret(...)`: generates `if !(<expr>) { return expr1, expr2, ... }`.
- The message is stored but not used in the generated code (use `-log` to output it).

### `@require -log` — Precondition with Log + Return

Like `-ret`, but also logs the violation message before returning. Implies `-ret`. Auto-imports `log`.

#### Non-defaulted mode

```go
// @require -log -nd var1, var2
```

- Generates `if var == nil { log.Println("..."); return }`.

#### Expression mode

```go
// @require -log <expr>
// @require -log <expr>, "custom message"
```

- Generates `if !(<expr>) { log.Println("custom message at file.go:LINE"); return }`.
- Uses the custom message if provided; otherwise a default violation message.

### Flag Order

Flags (`-nd`, `-ret`, `-log`) are order-insensitive:

```go
// @require -ret -nd x       // OK
// @require -nd -ret x       // OK (same effect)
// @require -log -nd x       // OK (-log implies -ret)
// @require -ret -log -nd x  // OK (explicit -ret + -log)
```

### `@ensure` — Postcondition

Checked at function exit via `defer`. Typically used with named return values.

#### Non-defaulted mode (`-nd`)

```go
// @ensure -nd result
```

- Wraps the check in `defer func() { if result == nil { panic(...) } }()`.
- The named return variable must be declared in the function signature.

#### Expression mode

```go
// @ensure <expr>
// @ensure <expr>, "custom message"
```

- Same as `@require` expression mode, but wrapped in a `defer`.

### `@must` — Error assertion

Asserts that an error returned from a call is nil. Supports two placement styles and optional `-ret` flag.

#### Inline mode (same line as assignment)

```go
res, _ := db.Exec(query) // @must
```

- The `_` (blank identifier) is replaced with a generated error variable `_inco_err_<line>`.
- Generates `if _inco_err_<line> != nil { panic("inco // must violation at <loc>: " + _inco_err_<line>.Error()) }` immediately after the assignment.
- If the LHS has an explicit `err` variable instead of `_`, the check uses `err` directly.

#### Block mode (directive on its own line, applies to the next statement)

```go
// @must
res, _ := db.Query(
    "SELECT * FROM users WHERE id = ?",
)
```

- The directive on its own line applies to the **next assignment statement**.
- Useful for multi-line function calls.

#### `-ret` flag (return error instead of panicking)

```go
res, _ := db.Query("SELECT 1") // @must -ret
```

- Instead of panicking, generates `if err != nil { return zeroVal1, ..., err }` — propagates the error.
- Non-error return positions get zero values; the error position gets the actual error variable.
- Ideal for replacing `if err != nil { return err }` boilerplate.

#### `-ret(expr1, expr2, ...)` (return custom expressions on error)

```go
res, _ := db.Query("SELECT 1") // @must -ret("", ErrNotFound)
```

- Returns the specified custom expressions instead of auto-generated zero values + error.

## Syntax Rules

1. Directives must appear as **Go line comments** (`// @directive`) or block comments (`/* @directive */`).
2. The `@` prefix is mandatory and must immediately follow the comment marker (after optional whitespace).
3. `@require` and `@ensure` directives are standalone comments on their own line, placed inside a function body.
4. `@must` can be either inline (trailing comment on an assignment) or standalone (preceding line).
5. Flags (`-nd`, `-ret`, `-log`) appear after the directive keyword, before any variable list or expression. Order-insensitive.
6. In expression mode, the custom message must be a **double-quoted string** and must be the last comma-separated token.
7. Directives work inside closures/anonymous functions and nested scopes.

## Placement Rules

| Directive | Where to place | Scope |
|-----------|----------------|-------|
| `@require -nd` | Top of function body, before logic | Function entry |
| `@require <expr>` | Top of function body, before logic | Function entry |
| `@require -ret -nd` | Top of function body, before logic | Function entry (returns on violation) |
| `@require -ret <expr>` | Top of function body, before logic | Function entry (returns on violation) |
| `@require -ret(...) -nd` | Top of function body, before logic | Function entry (returns custom exprs) |
| `@require -ret(...) <expr>` | Top of function body, before logic | Function entry (returns custom exprs) |
| `@require -log -nd` | Top of function body, before logic | Function entry (logs + returns) |
| `@require -log <expr>` | Top of function body, before logic | Function entry (logs + returns) |
| `@ensure -nd` | Inside function body (generates `defer`) | Function exit |
| `@ensure <expr>` | Inside function body (generates `defer`) | Function exit |
| `@must` (inline) | Trailing comment on assignment with `_` or `err` | Immediately after assignment |
| `@must` (block) | Own line, before an assignment statement | Immediately after next assignment |
| `@must -ret` (inline) | Trailing comment on assignment | Returns error instead of panicking |
| `@must -ret` (block) | Own line, before an assignment statement | Returns error instead of panicking |
| `@must -ret(...)` (inline) | Trailing comment on assignment | Returns custom expressions on error |
| `@must -ret(...)` (block) | Own line, before an assignment statement | Returns custom expressions on error |

## Generated Code Patterns

### `@require -nd var`

```go
// Type-aware: the check depends on the resolved type of var.
// Examples for different types:

// pointer/interface/slice/map/chan:
if var == nil {
    panic("inco // require -nd violation: [var] is defaulted (nil) at file.go:LINE")
}

// string:
if var == "" {
    panic("inco // require -nd violation: [var] is defaulted (empty string) at file.go:LINE")
}

// integer:
if var == 0 {
    panic("inco // require -nd violation: [var] is defaulted (zero) at file.go:LINE")
}

// bool:
if !var {
    panic("inco // require -nd violation: [var] is defaulted (false) at file.go:LINE")
}

// comparable type param T:
if var == *new(T) {
    panic("inco // require -nd violation: [var] is defaulted (zero value of type param T) at file.go:LINE")
}

// non-comparable type param T (any):
if reflect.ValueOf(&var).Elem().IsZero() {
    panic("inco // require -nd violation: [var] is defaulted (zero value of type param T (reflect)) at file.go:LINE")
}
```

### `@require expr, "msg"`

```go
if !(expr) {
    panic("msg at file.go:LINE")
}
```

### `@require -ret -nd var` (return on violation)

```go
// Named returns → bare return:
if var == nil {
    return
}

// Unnamed returns → zero values:
if var == nil {
    return nil, 0, ""
}
```

### `@require -ret(expr1, expr2) -nd var` (return custom expressions)

```go
if var == nil {
    return expr1, expr2
}
```

### `@require -ret expr` (return on violation)

```go
// Named returns:
if !(expr) {
    return
}

// Unnamed returns:
if !(expr) {
    return nil, ""
}
```

### `@require -ret(expr1, expr2) expr` (return custom expressions)

```go
if !(expr) {
    return expr1, expr2
}
```

### `@require -log -nd var` (log + return)

```go
if var == nil {
    log.Println("inco // require -ret -nd violation: [var] is defaulted (nil) at file.go:LINE")
    return
}
```

### `@require -log expr, "msg"` (log + return)

```go
if !(expr) {
    log.Println("msg at file.go:LINE")
    return
}
```

### `@ensure -nd result`

```go
// Same type-aware checks as @require -nd, wrapped in defer:
defer func() {
    if result == nil {  // or type-appropriate zero check
        panic("inco // ensure -nd violation: [result] is defaulted (nil) at file.go:LINE")
    }
}()
```

### `res, _ := fn() // @must`

```go
res, _inco_err_LINE := fn()
if _inco_err_LINE != nil {
    panic("inco // must violation at file.go:LINE: " + _inco_err_LINE.Error())
}
```

### `res, _ := fn() // @must -ret`

```go
res, _inco_err_LINE := fn()
if _inco_err_LINE != nil {
    return zeroVal1, _inco_err_LINE
}
```

### `res, _ := fn() // @must -ret("", ErrCustom)`

```go
res, _inco_err_LINE := fn()
if _inco_err_LINE != nil {
    return "", ErrCustom
}
```

## Code Generation Guidelines for Copilot

When writing Go code in this project:

1. **Use `@require -nd` for nil-guard preconditions** on pointer, interface, slice, map, and channel parameters instead of manual `if x == nil` checks.
2. **Use `@require <expr>` for value-range preconditions** (e.g., `amount > 0`, `len(name) > 0`) instead of inline validation boilerplate.
3. **Use `@require -ret -nd` for early-return nil guards** when the function should return zero values instead of panicking (replaces `if x == nil { return }` boilerplate).
4. **Use `@require -ret(expr1, expr2) -nd` for early-return with specific error values** (e.g., `@require -ret(nil, ErrNotFound) -nd db`).
5. **Use `@require -log` for guarded returns with logging** when you need a trace of violated preconditions without crashing (replaces `if x == nil { log.Println(...); return }` boilerplate).
6. **Use `@ensure -nd` for postcondition guarantees** on named return values that must not be nil at function exit.
7. **Use `// @must` on assignments that discard errors with `_`** when the error should never occur (replacing silent drops with fail-fast panics).
8. **Use `// @must -ret` on error-returning calls** to replace `if err != nil { return err }` boilerplate with a single-line directive.
9. **Use `// @must -ret(expr1, expr2)` on error-returning calls** when you need custom return values on error instead of auto-generated zero values.
10. **Keep function bodies clean** — defensive checks belong in directives, not in business logic.
11. **Do not manually write assertion code** that Inco generates; use directives instead.
12. **Never modify files in `.inco_cache/`** — they are auto-generated.
13. **Named return values are required** for `@ensure` directives to reference.
12. The project is self-bootstrapping: source files in `internal/inco/` and `cmd/inco/` use Inco directives themselves.

## CLI Commands

```bash
inco gen [dir]    # Scan .go files, generate shadow files + overlay.json (default: .)
inco build ./...  # Build with contracts enforced (go build -overlay)
inco test ./...   # Test with contracts enforced
inco run .        # Run with contracts enforced
inco audit [dir]  # Contract coverage report (default: .)
inco clean [dir]  # Remove .inco_cache (default: .)
```

## Project Structure

```
cmd/inco/           CLI entry point (exec.go, main.go)
internal/inco/      Core engine:
  audit.go            Contract coverage auditing
  contract.go         Directive parsing (ParseDirective)
  engine.go           AST injection, overlay generation, //line mapping
  typecheck.go        Type resolution, zero-value check generation, generics support
example/            Demo files:
  demo.go             Basic directives (require, ensure, must)
  transfer.go         Full directive set showcase
  edge_cases.go       Closures, multi-line @must, -ret/-ret(...)/-log
  generics.go         Type parameters: comparable, any, mixed, expression mode
.inco_cache/        Generated shadow files + overlay.json (git-ignored)
```

## Examples

### Full directive set

```go
func Transfer(from *Account, to *Account, amount int) {
    // @require -nd from, to
    // @require amount > 0, "amount must be positive"

    res, _ := db.Exec(query) // @must

    // @ensure -nd res

    fmt.Printf("Transfer %d\n", amount)
}
```

### Closure with directives

```go
func ProcessWithCallback(db *DB) {
    handler := func(u *User) {
        // @require -nd u
        fmt.Println(u.Name)
    }
    u, _ := db.Query("SELECT 1")
    handler(u)
}
```

### Multi-line @must

```go
func FetchMultiLine(db *DB) *User {
    // @must
    res, _ := db.Query(
        "SELECT * FROM users WHERE id = ?",
    )
    return res
}
```

### Generics with directives

```go
// comparable type param → *new(T) zero check
func FirstNonZero[T comparable](items []T) (result T) {
    // @ensure -nd result
    for _, v := range items {
        return v
    }
    return
}

// any type param → reflect.ValueOf zero check (import auto-added)
func MustNotBeZero[T any](v T) T {
    // @require -nd v
    return v
}

// Expression mode works with type params
func Clamp[N Number](val, lo, hi N) N {
    // @require lo <= hi, "lo must not exceed hi"
    if val < lo { return lo }
    if val > hi { return hi }
    return val
}
```

### Return on violation (-ret)

```go
// Silent return with zero values when precondition fails
func SafeGetUser(db *DB, id string) (*User, error) {
    // @require -ret -nd db
    // @require -ret len(id) > 0
    return db.Query("SELECT * FROM users WHERE id = ?")
}
```

### Return custom expressions (-ret(...))

```go
// Return specific error values on violation
func FindUser(db *DB, id string) (*User, error) {
    // @require -ret(nil, ErrNotFound) -nd db
    // @require -ret(nil, fmt.Errorf("invalid id: %s", id)) len(id) > 0
    return db.Query("SELECT * FROM users WHERE id = ?")
}
```

### Log and return (-log)

```go
// Log the violation and return zero values (auto-imports "log")
func ProcessOrder(order *Order, amount int) (result *Receipt) {
    // @require -log -nd order
    // @require -log amount > 0, "amount must be positive"
    // ... business logic ...
    return
}
```

### @must -ret (error propagation)

```go
// Replace `if err != nil { return err }` boilerplate
func ProcessFile(path string) error {
    f, _ := os.Open(path) // @must -ret
    defer f.Close()
    // ...
    return nil
}
```

### @must -ret(...) (custom error return)

```go
// Return custom expressions on error
func Fetch(db *DB) (string, error) {
    res, _ := db.Query("SELECT 1") // @must -ret("", ErrNotFound)
    return res, nil
}
```

## Shadow File Features

### `//line` Directives

Generated shadow files include `//line` directives so that panic stack traces and compiler errors point back to the original source file and line numbers. This ensures a seamless debugging experience.

### Static Expression Warnings

During `inco gen`, if a `@require` expression is a compile-time constant that evaluates to `false`, Inco emits a warning to stderr:
```
inco: expression "1 > 2" is always false (compile-time constant) at main.go:10
```

### Auto-Import

When generated code requires additional imports (e.g., `reflect` for non-comparable type params, `log` for `-log` mode, or cross-package types in struct/array composite literals), the imports are automatically added to the shadow file.
