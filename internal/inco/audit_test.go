package inco

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAudit_BasicCounts(t *testing.T) {
	dir := t.TempDir()

	// File with @require, @must, @expect, @ensure and native if.
	writeFile(t, filepath.Join(dir, "main.go"), `package main

import "fmt"

func Guarded(x int, name string) {
	// @require x > 0
	// @require len(name) > 0
	if x > 10 {
		fmt.Println("big")
	}
}

func Unguarded(y int) {
	if y < 0 {
		fmt.Println("neg")
	}
}

type DB struct{}
func (db *DB) Query(q string) (string, error) { return "", nil }

func UseMust(db *DB) {
	_, _ = db.Query("SELECT 1") // @must
	if true {
		fmt.Println("logic")
	}
}

func UseExpect() {
	m := map[string]int{"a": 1}
	_, _ = m["a"] // @expect
}

func UseEnsure(x int) int {
	// @ensure result > 0
	result := x * 2
	return result
}
`)

	result := Audit(dir)

	// Totals.
	if result.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1", result.TotalFiles)
	}
	if result.TotalFuncs != 6 { // Guarded, Unguarded, Query, UseMust, UseExpect, UseEnsure
		t.Errorf("TotalFuncs = %d, want 6", result.TotalFuncs)
	}
	if result.GuardedFuncs != 1 { // only Guarded
		t.Errorf("GuardedFuncs = %d, want 1", result.GuardedFuncs)
	}
	if result.TotalRequires != 2 {
		t.Errorf("TotalRequires = %d, want 2", result.TotalRequires)
	}
	if result.TotalMusts != 1 {
		t.Errorf("TotalMusts = %d, want 1", result.TotalMusts)
	}
	if result.TotalExpects != 1 {
		t.Errorf("TotalExpects = %d, want 1", result.TotalExpects)
	}
	if result.TotalEnsures != 1 {
		t.Errorf("TotalEnsures = %d, want 1", result.TotalEnsures)
	}
	if result.TotalDirectives != 5 {
		t.Errorf("TotalDirectives = %d, want 5", result.TotalDirectives)
	}
	if result.TotalIfs != 3 { // x>10, y<0, true
		t.Errorf("TotalIfs = %d, want 3", result.TotalIfs)
	}
}

func TestAudit_MultipleFiles(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "a.go"), `package main

func A(x int) {
	// @require x > 0
}

func B(y int) {
	if y < 0 {}
}
`)

	writeFile(t, filepath.Join(dir, "b.go"), `package main

func C(z int) {
	// @require z != 0
	if z > 100 {}
}
`)

	result := Audit(dir)

	if result.TotalFiles != 2 {
		t.Errorf("TotalFiles = %d, want 2", result.TotalFiles)
	}
	if result.TotalFuncs != 3 {
		t.Errorf("TotalFuncs = %d, want 3", result.TotalFuncs)
	}
	if result.GuardedFuncs != 2 { // A and C
		t.Errorf("GuardedFuncs = %d, want 2", result.GuardedFuncs)
	}
	if result.TotalRequires != 2 {
		t.Errorf("TotalRequires = %d, want 2", result.TotalRequires)
	}
	if result.TotalIfs != 2 {
		t.Errorf("TotalIfs = %d, want 2", result.TotalIfs)
	}
}

func TestAudit_SkipsHiddenAndTestFiles(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "main.go"), `package main

func X(a int) {
	// @require a > 0
}
`)
	// Test file — should be skipped.
	writeFile(t, filepath.Join(dir, "main_test.go"), `package main

func TestX() {
	// @require true
}
`)
	// Hidden dir — should be skipped.
	hidden := filepath.Join(dir, ".cache")
	os.MkdirAll(hidden, 0o755)
	writeFile(t, filepath.Join(hidden, "cached.go"), `package cache

func Y(b int) {
	// @require b > 0
}
`)

	result := Audit(dir)

	if result.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1", result.TotalFiles)
	}
	if result.TotalRequires != 1 {
		t.Errorf("TotalRequires = %d, want 1", result.TotalRequires)
	}
}

func TestAudit_ClosureCounted(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "main.go"), `package main

func Outer() {
	// @require true
	inner := func(x int) {
		// @require x > 0
	}
	_ = inner
}
`)

	result := Audit(dir)

	if result.TotalFuncs != 2 { // Outer + func literal
		t.Errorf("TotalFuncs = %d, want 2", result.TotalFuncs)
	}
	if result.GuardedFuncs != 2 {
		t.Errorf("GuardedFuncs = %d, want 2", result.GuardedFuncs)
	}
}

func TestAudit_EmptyProject(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "main.go"), `package main

func main() {}
`)

	result := Audit(dir)

	if result.TotalFuncs != 1 {
		t.Errorf("TotalFuncs = %d, want 1", result.TotalFuncs)
	}
	if result.GuardedFuncs != 0 {
		t.Errorf("GuardedFuncs = %d, want 0", result.GuardedFuncs)
	}
	if result.TotalDirectives != 0 {
		t.Errorf("TotalDirectives = %d, want 0", result.TotalDirectives)
	}
}

func TestAudit_MethodReceiver(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "main.go"), `package main

type Svc struct{}

func (s *Svc) Do(x int) {
	// @require x > 0
}
`)

	result := Audit(dir)

	if result.TotalFuncs != 1 {
		t.Errorf("TotalFuncs = %d, want 1", result.TotalFuncs)
	}
	if result.GuardedFuncs != 1 {
		t.Errorf("GuardedFuncs = %d, want 1", result.GuardedFuncs)
	}

	if len(result.Files) != 1 || len(result.Files[0].Funcs) != 1 {
		t.Fatal("unexpected file/func count")
	}
	if result.Files[0].Funcs[0].Name != "Svc.Do" {
		t.Errorf("func name = %q, want %q", result.Files[0].Funcs[0].Name, "Svc.Do")
	}
}

func TestAudit_PrintReport(t *testing.T) {
	r := &AuditResult{
		TotalFiles:      2,
		TotalFuncs:      5,
		GuardedFuncs:    3,
		TotalIfs:        10,
		TotalRequires:   4,
		TotalMusts:      2,
		TotalExpects:    1,
		TotalEnsures:    1,
		TotalDirectives: 8,
		Files: []FileAudit{
			{RelPath: "a.go", RequireCount: 3, MustCount: 1, ExpectCount: 0, IfCount: 6,
				Funcs: []FuncAudit{{Name: "A", Line: 3, RequireCount: 2}, {Name: "B", Line: 8, RequireCount: 1}}},
			{RelPath: "b.go", RequireCount: 1, MustCount: 1, ExpectCount: 1, IfCount: 4,
				Funcs: []FuncAudit{{Name: "C", Line: 3, RequireCount: 0}, {Name: "D", Line: 8, RequireCount: 0}, {Name: "E", Line: 13, RequireCount: 1}}},
		},
	}

	var buf bytes.Buffer
	r.PrintReport(&buf)
	out := buf.String()

	// Check key sections are present.
	for _, want := range []string{
		"contract coverage report",
		"@require coverage:",
		"3 / 5",
		"60.0%",
		"Directive vs if:",
		"@require:",
		"@must:",
		"@expect:",
		"@ensure:",
		"Total directives:",
		"Native if stmts:",
		"0.80",
		"Per-file breakdown:",
		"a.go",
		"b.go",
		"Functions without @require",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q\n\nFull output:\n%s", want, out)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
