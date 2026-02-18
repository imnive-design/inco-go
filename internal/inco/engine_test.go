package inco

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Engine integration tests ---

// setupTestDir creates a temp directory with Go source files, returns the path.
func setupTestDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestEngine_NoDirectives(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

func main() {}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}
	if len(e.Overlay.Replace) != 0 {
		t.Errorf("expected 0 overlay entries, got %d", len(e.Overlay.Replace))
	}
}

func TestEngine_RequireND_Pointer(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

type User struct{ Name string }

func Greet(u *User) {
	// @require -nd u
	fmt.Println(u.Name)
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}
	if len(e.Overlay.Replace) != 1 {
		t.Fatalf("expected 1 overlay entry, got %d", len(e.Overlay.Replace))
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "u == nil") {
		t.Error("shadow should contain 'u == nil' check for pointer type")
	}
	if !strings.Contains(shadow, "panic(") {
		t.Error("shadow should contain panic call")
	}
}

func TestEngine_RequireND_String(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func Hello(name string) {
	// @require -nd name
	fmt.Println(name)
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, `name == ""`) {
		t.Errorf("shadow should contain string zero check, got:\n%s", shadow)
	}
}

func TestEngine_RequireND_Int(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func Process(count int) {
	// @require -nd count
	fmt.Println(count)
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "count == 0") {
		t.Errorf("shadow should contain int zero check, got:\n%s", shadow)
	}
}

func TestEngine_RequireND_Bool(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func Check(ok bool) {
	// @require -nd ok
	fmt.Println(ok)
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "!ok") {
		t.Errorf("shadow should contain bool zero check '!ok', got:\n%s", shadow)
	}
}

func TestEngine_RequireExpr(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func Create(name string, age int) {
	// @require len(name) > 0, "name required"
	// @require age > 0
	fmt.Println(name, age)
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "!(len(name) > 0)") {
		t.Error("shadow should contain negated expression for len(name) > 0")
	}
	if !strings.Contains(shadow, "!(age > 0)") {
		t.Error("shadow should contain negated expression for age > 0")
	}
	if !strings.Contains(shadow, "name required") {
		t.Error("shadow should contain custom message 'name required'")
	}
}

func TestEngine_EnsureND_Removed(t *testing.T) {
	// @ensure was removed — directive should be ignored (no overlay generated)
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

type Item struct{ ID int }

func Find(id int) (result *Item) {
	// @ensure -nd result
	return &Item{ID: id}
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}
	if len(e.Overlay.Replace) != 0 {
		t.Errorf("expected 0 overlay entries for removed @ensure, got %d", len(e.Overlay.Replace))
	}
}

func TestEngine_Must_Inline(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

type DB struct{}
func (db *DB) Query(q string) (string, error) { return "ok", nil }

func Fetch(db *DB) {
	res, _ := db.Query("SELECT 1") // @must
	fmt.Println(res)
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "_inco_err_") {
		t.Error("shadow should contain generated _inco_err_ variable")
	}
	if !strings.Contains(shadow, ".Error()") {
		t.Error("shadow should contain .Error() call")
	}
}

func TestEngine_Must_Block(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

type DB struct{}
func (db *DB) Query(q string) (string, error) { return "ok", nil }

func FetchBlock(db *DB) {
	// @must
	res, _ := db.Query(
		"SELECT 1",
	)
	fmt.Println(res)
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "_inco_err_") {
		t.Error("shadow should contain generated _inco_err_ variable for block @must")
	}
}

func TestEngine_Generics_Comparable(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

func First[T comparable](items []T) T {
	// @require -nd items
	return items[0]
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "items == nil") {
		t.Errorf("shadow should contain 'items == nil' for slice param, got:\n%s", shadow)
	}
}

func TestEngine_Generics_Any(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

func Check[T any](v T) T {
	// @require -nd v
	return v
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "reflect") {
		t.Errorf("shadow should use reflect for non-comparable type param, got:\n%s", shadow)
	}
	if !strings.Contains(shadow, "IsZero") {
		t.Errorf("shadow should contain IsZero for non-comparable type param, got:\n%s", shadow)
	}
}

func TestEngine_Generics_ReflectImport(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

func Validate[T any](v T) T {
	// @require -nd v
	return v
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	// The import "reflect" should be auto-added
	if !strings.Contains(shadow, `"reflect"`) {
		t.Errorf("shadow should auto-import reflect for any type param, got:\n%s", shadow)
	}
}

func TestEngine_OverlayJSON(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

func Do(x *int) {
	// @require -nd x
	_ = *x
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	overlayPath := filepath.Join(dir, ".inco_cache", "overlay.json")
	data, err := os.ReadFile(overlayPath)
	if err != nil {
		t.Fatalf("overlay.json not found: %v", err)
	}

	var overlay Overlay
	if err := json.Unmarshal(data, &overlay); err != nil {
		t.Fatalf("invalid overlay JSON: %v", err)
	}

	if len(overlay.Replace) != 1 {
		t.Errorf("overlay has %d entries, want 1", len(overlay.Replace))
	}

	// Check that the shadow file exists
	for _, shadowPath := range overlay.Replace {
		if _, err := os.Stat(shadowPath); err != nil {
			t.Errorf("shadow file does not exist: %s", shadowPath)
		}
	}
}

func TestEngine_SkipsHiddenDirs(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		".hidden/main.go": `package hidden

func X(p *int) {
	// @require -nd p
	_ = *p
}
`,
		"main.go": `package main

func main() {}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}
	if len(e.Overlay.Replace) != 0 {
		t.Errorf("should skip hidden dirs, but got %d overlay entries", len(e.Overlay.Replace))
	}
}

func TestEngine_MultipleVarsND(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func Multi(a *int, b string, c float64) {
	// @require -nd a, b, c
	fmt.Println(a, b, c)
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "a == nil") {
		t.Error("should check a == nil (pointer)")
	}
	if !strings.Contains(shadow, `b == ""`) {
		t.Error(`should check b == "" (string)`)
	}
	if !strings.Contains(shadow, "c == 0.0") {
		t.Error("should check c == 0.0 (float64)")
	}
}

func TestEngine_Closure(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func Outer() {
	f := func(x *int) {
		// @require -nd x
		fmt.Println(*x)
	}
	v := 42
	f(&v)
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "x == nil") {
		t.Error("should contain nil check for closure param")
	}
}

func TestEngine_LineDirectives(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func Hello(name string) {
	// @require len(name) > 0
	fmt.Println(name)
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "//line") {
		t.Error("shadow should contain //line directives for source mapping")
	}
}

// readShadow reads the first shadow file content from the engine's overlay.
func readShadow(t *testing.T, e *Engine) string {
	t.Helper()
	for _, path := range e.Overlay.Replace {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read shadow: %v", err)
		}
		return string(data)
	}
	t.Fatal("no shadow files in overlay")
	return ""
}

// --- Bug fix regression tests ---

// TestEngine_Must_ReplacesLastBlank verifies that @must replaces the LAST blank
// identifier (error position), not the first one.
func TestEngine_Must_ReplacesLastBlank(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"go.mod": "module testmod\n\ngo 1.21\n",
		"main.go": `package main

type Result struct{}

type DB struct{}

func (db *DB) Exec(q string) (*Result, error) {
	return &Result{}, nil
}

func main() {
	db := &DB{}
	_, _ = db.Exec("INSERT INTO t VALUES (1)") // @must
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}
	shadow := readShadow(t, e)
	// The LAST _ should be replaced (error position), not the first
	if !strings.Contains(shadow, "_inco_err_") {
		t.Fatal("expected _inco_err_ in shadow")
	}
	// The first _ should remain as _ (it's the non-error result)
	if strings.Contains(shadow, "_inco_err_") && !strings.Contains(shadow, "_, _inco_err_") {
		// If the first _ was replaced, it would be "_inco_err_, _" which is wrong
		if strings.Contains(shadow, "_inco_err_") {
			// Check that the pattern is "_, _inco_err_LINE" not "_inco_err_LINE, _"
			lines := strings.Split(shadow, "\n")
			for _, line := range lines {
				if strings.Contains(line, "_inco_err_") && strings.Contains(line, "db.Exec") {
					trimmed := strings.TrimSpace(line)
					if strings.HasPrefix(trimmed, "_inco_err_") {
						t.Errorf("@must replaced the FIRST _ instead of the LAST: %s", trimmed)
					}
				}
			}
		}
	}
}

// TestEngine_DirectiveOrder verifies that multiple directives between two
// statements are injected in source order (not random map iteration order).
func TestEngine_DirectiveOrder(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"go.mod": "module testmod\n\ngo 1.21\n",
		"main.go": `package main

import "fmt"

func Process(name string, age int, score float64) {
	// @require len(name) > 0, "name required"
	// @require age > 0, "age must be positive"
	// @require score >= 0, "score must be non-negative"
	fmt.Println(name, age, score)
}

func main() {}
`,
	})

	// Run multiple times to check for non-determinism
	for i := 0; i < 5; i++ {
		e := NewEngine(dir)
		if err := e.Run(); err != nil {
			t.Fatal(err)
		}
		shadow := readShadow(t, e)
		nameIdx := strings.Index(shadow, "name required")
		ageIdx := strings.Index(shadow, "age must be positive")
		scoreIdx := strings.Index(shadow, "score must be non-negative")
		if nameIdx < 0 || ageIdx < 0 || scoreIdx < 0 {
			t.Fatal("expected all three require panic messages in shadow")
		}
		if !(nameIdx < ageIdx && ageIdx < scoreIdx) {
			t.Errorf("iteration %d: directives not in source order: name@%d age@%d score@%d",
				i, nameIdx, ageIdx, scoreIdx)
		}
	}
}

// TestEngine_ConsecutiveContractComments verifies that //line directives are
// correctly emitted when multiple consecutive contract comments exist.
func TestEngine_ConsecutiveContractComments(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"go.mod": "module testmod\n\ngo 1.21\n",
		"main.go": `package main

import "fmt"

func Foo(a *int, b *int, c *int) {
	// @require -nd a
	// @require -nd b
	// @require -nd c
	fmt.Println(*a, *b, *c)
}

func main() {}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}
	shadow := readShadow(t, e)
	// After the injected checks, there should be a //line directive that
	// maps back to the fmt.Println line (line 9 in original)
	if !strings.Contains(shadow, "//line") {
		t.Error("expected //line directive in shadow with consecutive contract comments")
	}
	// Verify the line directive points to the correct line
	if !strings.Contains(shadow, ":9") && !strings.Contains(shadow, ":10") {
		t.Logf("shadow content:\n%s", shadow)
		t.Error("//line directive does not restore correct original line number")
	}
}

// --- Additional engine tests ---

func TestEngine_EnsureExpr_Removed(t *testing.T) {
	// @ensure was removed — directive should be ignored
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

func Compute(x int) (result int) {
	// @ensure result > 0, "result must be positive"
	result = x * 2
	return
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}
	if len(e.Overlay.Replace) != 0 {
		t.Errorf("expected 0 overlay entries for removed @ensure, got %d", len(e.Overlay.Replace))
	}
}

func TestEngine_EnsureExpr_DefaultMessage_Removed(t *testing.T) {
	// @ensure was removed — directive should be ignored
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

func Compute(x int) (result int) {
	// @ensure result >= 0
	result = x * 2
	return
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}
	if len(e.Overlay.Replace) != 0 {
		t.Errorf("expected 0 overlay entries for removed @ensure, got %d", len(e.Overlay.Replace))
	}
}

func TestEngine_Must_ExplicitErr(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

type DB struct{}
func (db *DB) Query(q string) (string, error) { return "ok", nil }

func Fetch(db *DB) {
	res, err := db.Query("SELECT 1") // @must
	fmt.Println(res, err)
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	// With explicit err, should use err directly, not _inco_err_
	if !strings.Contains(shadow, "err") {
		t.Error("shadow should contain err check for explicit err variable")
	}
	if !strings.Contains(shadow, "must violation") {
		t.Error("shadow should contain must violation panic")
	}
}

func TestEngine_ContentHashStable(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func Hello(name string) {
	// @require len(name) > 0
	fmt.Println(name)
}
`,
	})

	// Run twice and verify same shadow file is produced
	e1 := NewEngine(dir)
	if err := e1.Run(); err != nil {
		t.Fatal(err)
	}
	var shadow1Path string
	for _, p := range e1.Overlay.Replace {
		shadow1Path = p
	}

	e2 := NewEngine(dir)
	if err := e2.Run(); err != nil {
		t.Fatal(err)
	}
	var shadow2Path string
	for _, p := range e2.Overlay.Replace {
		shadow2Path = p
	}

	if filepath.Base(shadow1Path) != filepath.Base(shadow2Path) {
		t.Errorf("shadow filenames differ across runs: %s vs %s",
			filepath.Base(shadow1Path), filepath.Base(shadow2Path))
	}
}

func TestEngine_RequireND_FuncType(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

func Apply(fn func(int) int) {
	// @require -nd fn
	_ = fn(1)
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "fn == nil") {
		t.Errorf("func type should produce nil check, got:\n%s", shadow)
	}
}

func TestEngine_RequireND_Map(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func Lookup(m map[string]int) {
	// @require -nd m
	fmt.Println(m)
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "m == nil") {
		t.Errorf("map type should produce nil check, got:\n%s", shadow)
	}
}

func TestEngine_RequireND_Chan(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

func Wait(ch chan int) {
	// @require -nd ch
	<-ch
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "ch == nil") {
		t.Errorf("chan type should produce nil check, got:\n%s", shadow)
	}
}

func TestEngine_NoOverlayWhenNoDirectives(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

func main() {
	// just a regular comment
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}
	overlayPath := filepath.Join(dir, ".inco_cache", "overlay.json")
	if _, err := os.Stat(overlayPath); !os.IsNotExist(err) {
		t.Error("overlay.json should not exist when there are no directives")
	}
}

func TestEngine_Must_TypeAware(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

type MyError struct{}
func (e *MyError) Error() string { return "fail" }

func run() (int, *MyError) {
	return 42, nil
}

func main() {
	// Should pick "e" as the error variable because of its type (*MyError implements error)
	// even if not named "err" or "_"
	val, e := run() // @must
	_ = val
	_ = e
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)

	// We expect check on 'e'
	if !strings.Contains(shadow, "if e != nil {") {
		t.Errorf("shadow should check e != nil, got:\n%s", shadow)
	}
}

// --- Tests for -ret and -log flags ---

func TestEngine_RequireRet_ND_NamedReturns(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

type User struct{ Name string }

func FindUser(u *User) (result *User, ok bool) {
	// @require -ret -nd u
	return u, true
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	// Should contain nil check
	if !strings.Contains(shadow, "u == nil") {
		t.Errorf("shadow should contain 'u == nil' check, got:\n%s", shadow)
	}
	// Should contain return (not panic)
	if strings.Contains(shadow, "panic(") {
		t.Error("shadow should NOT contain panic for -ret mode")
	}
	// Should have bare return (named returns)
	if !strings.Contains(shadow, "return") {
		t.Error("shadow should contain return statement")
	}
}

func TestEngine_RequireRet_ND_UnnamedReturns(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

type User struct{ Name string }

func GetUser(u *User) (*User, error) {
	// @require -ret -nd u
	return u, nil
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "u == nil") {
		t.Errorf("shadow should contain 'u == nil', got:\n%s", shadow)
	}
	if strings.Contains(shadow, "panic(") {
		t.Error("shadow should NOT contain panic for -ret mode")
	}
	// Should have return with zero values for unnamed returns
	if !strings.Contains(shadow, "return nil, nil") {
		t.Errorf("shadow should contain 'return nil, nil' for unnamed returns, got:\n%s", shadow)
	}
}

func TestEngine_RequireRet_Expr(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func Process(amount int) (result string) {
	// @require -ret amount > 0
	result = fmt.Sprintf("amount: %d", amount)
	return
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "!(amount > 0)") {
		t.Errorf("shadow should contain negated expression, got:\n%s", shadow)
	}
	if strings.Contains(shadow, "panic(") {
		t.Error("shadow should NOT contain panic for -ret mode")
	}
	if !strings.Contains(shadow, "return") {
		t.Error("shadow should contain return for -ret mode")
	}
}

func TestEngine_RequireLog_ND(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

type User struct{ Name string }

func FindUser(u *User) (result *User) {
	// @require -log -nd u
	return u
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "u == nil") {
		t.Errorf("shadow should contain 'u == nil', got:\n%s", shadow)
	}
	if strings.Contains(shadow, "panic(") {
		t.Error("shadow should NOT contain panic for -log mode")
	}
	// Should have log.Println
	if !strings.Contains(shadow, "log.Println") {
		t.Errorf("shadow should contain log.Println, got:\n%s", shadow)
	}
	// Should auto-import log
	if !strings.Contains(shadow, `"log"`) {
		t.Errorf("shadow should auto-import log, got:\n%s", shadow)
	}
	// Should also return
	if !strings.Contains(shadow, "return") {
		t.Error("shadow should contain return for -log mode")
	}
}

func TestEngine_RequireLog_Expr(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func Transfer(amount int) string {
	// @require -log amount > 0, "amount must be positive"
	return fmt.Sprintf("transferred: %d", amount)
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "!(amount > 0)") {
		t.Errorf("shadow should contain negated expression, got:\n%s", shadow)
	}
	if !strings.Contains(shadow, "log.Println") {
		t.Errorf("shadow should contain log.Println, got:\n%s", shadow)
	}
	if !strings.Contains(shadow, "amount must be positive") {
		t.Errorf("shadow should contain custom message, got:\n%s", shadow)
	}
	if strings.Contains(shadow, "panic(") {
		t.Error("shadow should NOT contain panic for -log mode")
	}
}

func TestEngine_RequireRet_VoidFunc(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func Greet(name string) {
	// @require -ret -nd name
	fmt.Println("Hello", name)
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, `name == ""`) {
		t.Errorf("shadow should contain string zero check, got:\n%s", shadow)
	}
	if strings.Contains(shadow, "panic(") {
		t.Error("shadow should NOT contain panic for -ret mode")
	}
}

func TestEngine_RequireRet_MultipleVars(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func Multi(a *int, b string) (ok bool) {
	// @require -ret -nd a, b
	fmt.Println(a, b)
	return true
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "a == nil") {
		t.Error("should check a == nil (pointer)")
	}
	if !strings.Contains(shadow, `b == ""`) {
		t.Error(`should check b == "" (string)`)
	}
	if strings.Contains(shadow, "panic(") {
		t.Error("should NOT contain panic for -ret mode")
	}
}

func TestEngine_RequireLog_AutoImport(t *testing.T) {
	// Verifies that "log" is auto-imported even when the original file doesn't use it
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

func Check(x *int) (result int) {
	// @require -log -nd x
	return *x
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, `"log"`) {
		t.Errorf("shadow must auto-import log package, got:\n%s", shadow)
	}
}

func TestEngine_RequireRet_Closure(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func Outer() {
	f := func(x *int) int {
		// @require -ret -nd x
		return *x
	}
	fmt.Println(f(nil))
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "x == nil") {
		t.Error("should contain nil check for closure param with -ret")
	}
	if strings.Contains(shadow, "panic(") {
		t.Error("should NOT contain panic for -ret in closure")
	}
}

// --- Tests for -ret(expr, ...) custom return expressions ---

func TestEngine_RetExprs_ND(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "errors"

var ErrNotFound = errors.New("not found")

type DB struct{}

func (db *DB) FindUser(id string) (*DB, error) {
	// @require -ret(nil, ErrNotFound) -nd id
	return db, nil
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, `id == ""`) {
		t.Errorf("shadow should contain string zero check, got:\n%s", shadow)
	}
	if !strings.Contains(shadow, "ErrNotFound") {
		t.Errorf("shadow should contain custom return expr 'ErrNotFound', got:\n%s", shadow)
	}
	if strings.Contains(shadow, "panic(") {
		t.Error("shadow should NOT contain panic for -ret mode")
	}
}

func TestEngine_RetExprs_Expr(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "errors"

var ErrInvalid = errors.New("invalid")

func Process(amount int) (string, error) {
	// @require -ret("", ErrInvalid) amount > 0
	return "ok", nil
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "!(amount > 0)") {
		t.Errorf("shadow should contain negated expression, got:\n%s", shadow)
	}
	if !strings.Contains(shadow, "ErrInvalid") {
		t.Errorf("shadow should contain 'ErrInvalid' return expr, got:\n%s", shadow)
	}
	if strings.Contains(shadow, "panic(") {
		t.Error("should NOT contain panic")
	}
}

func TestEngine_RetExprs_NestedCall(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func Fetch(id string) (string, error) {
	// @require -ret("", fmt.Errorf("bad id: %s", id)) len(id) > 0
	return id, nil
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "fmt.Errorf") {
		t.Errorf("shadow should contain fmt.Errorf call, got:\n%s", shadow)
	}
	if !strings.Contains(shadow, "!(len(id) > 0)") {
		t.Errorf("shadow should contain negated expression, got:\n%s", shadow)
	}
}

func TestEngine_RetExprs_WithLog(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "errors"

var ErrBadAmount = errors.New("bad amount")

func Transfer(amount int) (string, error) {
	// @require -log -ret("", ErrBadAmount) amount > 0, "amount must be positive"
	return "ok", nil
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "log.Println") {
		t.Errorf("shadow should contain log.Println, got:\n%s", shadow)
	}
	if !strings.Contains(shadow, "ErrBadAmount") {
		t.Errorf("shadow should contain custom return expr, got:\n%s", shadow)
	}
	if !strings.Contains(shadow, "amount must be positive") {
		t.Errorf("shadow should contain custom message, got:\n%s", shadow)
	}
	if strings.Contains(shadow, "panic(") {
		t.Error("should NOT contain panic")
	}
}

func TestEngine_RetExprs_SingleValue(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

func GetDefault(x *int) int {
	// @require -ret(-1) -nd x
	return *x
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "x == nil") {
		t.Errorf("shadow should contain nil check, got:\n%s", shadow)
	}
	if !strings.Contains(shadow, "-1") {
		t.Errorf("shadow should contain '-1' as custom return, got:\n%s", shadow)
	}
	if strings.Contains(shadow, "panic(") {
		t.Error("should NOT contain panic")
	}
}

func TestEngine_MustRet_Inline_SingleError(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "os"

func Run() error {
	err := os.MkdirAll("/tmp/test", 0o755) // @must -ret
	_ = err
	return nil
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "err != nil") {
		t.Errorf("shadow should contain 'err != nil' check, got:\n%s", shadow)
	}
	if !strings.Contains(shadow, "return err") {
		t.Errorf("shadow should contain 'return err', got:\n%s", shadow)
	}
	if strings.Contains(shadow, "panic(") {
		t.Error("should NOT contain panic for @must -ret")
	}
}

func TestEngine_MustRet_Inline_MultiReturn(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

type DB struct{}
func (db *DB) Query(q string) (string, error) { return "ok", nil }

func Fetch(db *DB) (string, error) {
	res, _ := db.Query("SELECT 1") // @must -ret
	return res, nil
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "_inco_err_") {
		t.Error("shadow should contain generated _inco_err_ variable")
	}
	if !strings.Contains(shadow, "!= nil") {
		t.Errorf("shadow should contain '!= nil' check, got:\n%s", shadow)
	}
	// Should return zero string + error, not panic
	if strings.Contains(shadow, "panic(") {
		t.Error("should NOT contain panic for @must -ret")
	}
	if !strings.Contains(shadow, "return") {
		t.Errorf("shadow should contain return statement, got:\n%s", shadow)
	}
}

func TestEngine_MustRet_Block(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

type DB struct{}
func (db *DB) Query(q string) (string, error) { return "ok", nil }

func FetchBlock(db *DB) (string, error) {
	// @must -ret
	res, _ := db.Query(
		"SELECT 1",
	)
	return res, nil
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "_inco_err_") {
		t.Error("shadow should contain generated _inco_err_ variable")
	}
	if strings.Contains(shadow, "panic(") {
		t.Error("should NOT contain panic for @must -ret block mode")
	}
}

func TestEngine_MustRet_ExplicitErr(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

type DB struct{}
func (db *DB) Query(q string) (string, error) { return "ok", nil }

func FetchExplicit(db *DB) (string, error) {
	res, err := db.Query("SELECT 1") // @must -ret
	return res, err
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "err != nil") {
		t.Errorf("shadow should contain 'err != nil', got:\n%s", shadow)
	}
	if strings.Contains(shadow, "panic(") {
		t.Error("should NOT contain panic")
	}
}

func TestEngine_MustRetExprs_Custom(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "errors"

var ErrNotFound = errors.New("not found")

type DB struct{}
func (db *DB) Query(q string) (string, error) { return "ok", nil }

func FetchCustom(db *DB) (string, error) {
	res, _ := db.Query("SELECT 1") // @must -ret("", ErrNotFound)
	return res, nil
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "ErrNotFound") {
		t.Errorf("shadow should contain custom return expression 'ErrNotFound', got:\n%s", shadow)
	}
	if strings.Contains(shadow, "panic(") {
		t.Error("should NOT contain panic")
	}
}

// --- Tests for @must -log ---

func TestEngine_MustLog_Inline(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

type DB struct{}
func (db *DB) Query(q string) (string, error) { return "ok", nil }

func Fetch(db *DB) (string, error) {
	res, _ := db.Query("SELECT 1") // @must -log
	return res, nil
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "_inco_err_") {
		t.Error("shadow should contain generated _inco_err_ variable")
	}
	if !strings.Contains(shadow, "log.Println") {
		t.Errorf("shadow should contain log.Println for @must -log, got:\n%s", shadow)
	}
	if !strings.Contains(shadow, `"log"`) {
		t.Errorf("shadow should auto-import log, got:\n%s", shadow)
	}
	if strings.Contains(shadow, "panic(") {
		t.Error("should NOT contain panic for @must -log")
	}
	if !strings.Contains(shadow, "return") {
		t.Error("shadow should contain return statement")
	}
}

func TestEngine_MustLog_Block(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": `package main

type DB struct{}
func (db *DB) Query(q string) (string, error) { return "ok", nil }

func FetchBlock(db *DB) (string, error) {
	// @must -log
	res, _ := db.Query(
		"SELECT 1",
	)
	return res, nil
}
`,
	})
	e := NewEngine(dir)
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}

	shadow := readShadow(t, e)
	if !strings.Contains(shadow, "log.Println") {
		t.Errorf("shadow should contain log.Println for @must -log block, got:\n%s", shadow)
	}
	if strings.Contains(shadow, "panic(") {
		t.Error("should NOT contain panic for @must -log block mode")
	}
}
