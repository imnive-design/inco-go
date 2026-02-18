package inco

import (
	"os"
	"path/filepath"
	"testing"
)

// setupAuditDir creates a temp directory with Go source files for audit testing.
func setupAuditDir(t *testing.T, files map[string]string) string {
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

func TestAudit_NoFunctions(t *testing.T) {
	dir := setupAuditDir(t, map[string]string{
		"main.go": `package main

var x = 42
`,
	})
	a := NewAuditor(dir)
	report, err := a.Run()
	if err != nil {
		t.Fatal(err)
	}
	s := report.Summarize()
	if s.TotalFuncs != 0 {
		t.Errorf("TotalFuncs = %d, want 0", s.TotalFuncs)
	}
	if s.FuncCoverage() != 100.0 {
		t.Errorf("FuncCoverage = %.1f%%, want 100.0%%", s.FuncCoverage())
	}
}

func TestAudit_FullyCovered(t *testing.T) {
	dir := setupAuditDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func Process(name string, age int) {
	// @require len(name) > 0
	// @require age > 0
	fmt.Println(name, age)
}

func main() {}
`,
	})
	a := NewAuditor(dir)
	report, err := a.Run()
	if err != nil {
		t.Fatal(err)
	}
	s := report.Summarize()
	if s.TotalFuncs != 2 {
		t.Errorf("TotalFuncs = %d, want 2", s.TotalFuncs)
	}
	if s.FuncsWithRequire != 1 {
		t.Errorf("FuncsWithRequire = %d, want 1", s.FuncsWithRequire)
	}
	if s.RequireCount != 2 {
		t.Errorf("RequireCount = %d, want 2", s.RequireCount)
	}
	// main() has no contracts → 1 covered, 1 uncovered
	if s.FuncsWithAny != 1 {
		t.Errorf("FuncsWithAny = %d, want 1", s.FuncsWithAny)
	}
	if len(s.UncoveredFuncs) != 1 {
		t.Errorf("UncoveredFuncs = %d, want 1", len(s.UncoveredFuncs))
	}
}

func TestAudit_AllDirectiveTypes(t *testing.T) {
	dir := setupAuditDir(t, map[string]string{
		"main.go": `package main

import "fmt"

type DB struct{}
func (db *DB) Query(q string) (string, error) { return "", nil }

func Fetch(db *DB) (result string) {
	// @require -nd db
	res, _ := db.Query("SELECT 1") // @must
	result = res
	fmt.Println(result)
	return
}

func main() {}
`,
	})
	a := NewAuditor(dir)
	report, err := a.Run()
	if err != nil {
		t.Fatal(err)
	}
	s := report.Summarize()
	if s.RequireCount != 1 {
		t.Errorf("RequireCount = %d, want 1", s.RequireCount)
	}
	if s.MustCount != 1 {
		t.Errorf("MustCount = %d, want 1", s.MustCount)
	}
	if s.TotalDirectives != 2 {
		t.Errorf("TotalDirectives = %d, want 2", s.TotalDirectives)
	}
	if s.FuncsWithRequire != 1 {
		t.Errorf("FuncsWithRequire = %d, want 1", s.FuncsWithRequire)
	}
}

func TestAudit_ErrorCoverage(t *testing.T) {
	dir := setupAuditDir(t, map[string]string{
		"main.go": `package main

type DB struct{}
func (db *DB) Query(q string) (string, error) { return "", nil }
func (db *DB) Exec(q string) (string, error) { return "", nil }

func Run(db *DB) {
	_, _ = db.Query("q1") // @must
	_, _ = db.Exec("q2")
}

func main() {}
`,
	})
	a := NewAuditor(dir)
	report, err := a.Run()
	if err != nil {
		t.Fatal(err)
	}
	s := report.Summarize()
	if s.TotalErrorAssignments != 2 {
		t.Errorf("TotalErrorAssignments = %d, want 2", s.TotalErrorAssignments)
	}
	if s.GuardedErrorAssignments != 1 {
		t.Errorf("GuardedErrorAssignments = %d, want 1", s.GuardedErrorAssignments)
	}
	if s.ErrorCoverage() != 50.0 {
		t.Errorf("ErrorCoverage = %.1f%%, want 50.0%%", s.ErrorCoverage())
	}
}

func TestAudit_UncoveredFuncs(t *testing.T) {
	dir := setupAuditDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func Covered(x *int) {
	// @require -nd x
	fmt.Println(*x)
}

func Uncovered1() {
	fmt.Println("no contracts")
}

func Uncovered2(n int) {
	fmt.Println(n)
}

func main() {}
`,
	})
	a := NewAuditor(dir)
	report, err := a.Run()
	if err != nil {
		t.Fatal(err)
	}
	s := report.Summarize()
	if s.TotalFuncs != 4 {
		t.Errorf("TotalFuncs = %d, want 4", s.TotalFuncs)
	}
	if s.FuncsWithAny != 1 {
		t.Errorf("FuncsWithAny = %d, want 1", s.FuncsWithAny)
	}
	if len(s.UncoveredFuncs) != 3 {
		t.Errorf("UncoveredFuncs = %d, want 3", len(s.UncoveredFuncs))
	}
	if s.FuncCoverage() != 25.0 {
		t.Errorf("FuncCoverage = %.1f%%, want 25.0%%", s.FuncCoverage())
	}
}

func TestAudit_SkipsHiddenAndVendor(t *testing.T) {
	dir := setupAuditDir(t, map[string]string{
		"main.go": `package main

func main() {}
`,
		".hidden/secret.go": `package hidden

func Secret() {}
`,
		"vendor/lib/lib.go": `package lib

func Vendored() {}
`,
		"testdata/fixture.go": `package testdata

func Fixture() {}
`,
	})
	a := NewAuditor(dir)
	report, err := a.Run()
	if err != nil {
		t.Fatal(err)
	}
	s := report.Summarize()
	// Only main.go should be scanned
	if s.TotalFuncs != 1 {
		t.Errorf("TotalFuncs = %d, want 1 (only main)", s.TotalFuncs)
	}
}

func TestAudit_MultipleFiles(t *testing.T) {
	dir := setupAuditDir(t, map[string]string{
		"a.go": `package main

func A(x *int) {
	// @require -nd x
}
`,
		"b.go": `package main

import "fmt"

func B(y string) string {
	// @require -nd y
	fmt.Println(y)
	return y
}
`,
	})
	a := NewAuditor(dir)
	report, err := a.Run()
	if err != nil {
		t.Fatal(err)
	}
	s := report.Summarize()
	if s.TotalFiles != 2 {
		t.Errorf("TotalFiles = %d, want 2", s.TotalFiles)
	}
	if s.FilesWithContracts != 2 {
		t.Errorf("FilesWithContracts = %d, want 2", s.FilesWithContracts)
	}
	if s.FuncsWithRequire != 2 {
		t.Errorf("FuncsWithRequire = %d, want 2", s.FuncsWithRequire)
	}
}

func TestAudit_BlockMust(t *testing.T) {
	dir := setupAuditDir(t, map[string]string{
		"main.go": `package main

type DB struct{}
func (db *DB) Query(q string) (string, error) { return "", nil }

func Fetch(db *DB) string {
	// @must
	res, _ := db.Query(
		"SELECT * FROM users",
	)
	return res
}

func main() {}
`,
	})
	a := NewAuditor(dir)
	report, err := a.Run()
	if err != nil {
		t.Fatal(err)
	}
	s := report.Summarize()
	if s.MustCount != 1 {
		t.Errorf("MustCount = %d, want 1", s.MustCount)
	}
	if s.TotalErrorAssignments != 1 {
		t.Errorf("TotalErrorAssignments = %d, want 1", s.TotalErrorAssignments)
	}
	if s.GuardedErrorAssignments != 1 {
		t.Errorf("GuardedErrorAssignments = %d, want 1", s.GuardedErrorAssignments)
	}
}

func TestAudit_FuncParams(t *testing.T) {
	dir := setupAuditDir(t, map[string]string{
		"main.go": `package main

func Transfer(from *int, to *int, amount int) (result string) {
	// @require -nd from, to
	return ""
}

func main() {}
`,
	})
	a := NewAuditor(dir)
	report, err := a.Run()
	if err != nil {
		t.Fatal(err)
	}
	// Find Transfer function
	var transfer *FuncAudit
	for _, f := range report.Files {
		for i, fn := range f.Functions {
			if fn.Name == "Transfer" {
				transfer = &f.Functions[i]
			}
		}
	}
	if transfer == nil {
		t.Fatal("Transfer function not found in audit")
	}
	if len(transfer.Params) != 3 {
		t.Errorf("Params count = %d, want 3", len(transfer.Params))
	}
	if len(transfer.Returns) != 1 {
		t.Errorf("Returns count = %d, want 1", len(transfer.Returns))
	}
	if transfer.Returns[0].Name != "result" {
		t.Errorf("Return name = %q, want %q", transfer.Returns[0].Name, "result")
	}
}

func TestAuditSummary_PrintReport(t *testing.T) {
	// Just ensure PrintReport doesn't panic
	s := AuditSummary{
		TotalFiles:              5,
		FilesWithContracts:      3,
		TotalFuncs:              10,
		FuncsWithRequire:        4,
		FuncsWithAny:            5,
		TotalDirectives:         8,
		RequireCount:            5,
		MustCount:               3,
		TotalErrorAssignments:   3,
		GuardedErrorAssignments: 1,
		UncoveredFuncs: []UncoveredFunc{
			{File: "main.go", Name: "main", Line: 1},
		},
	}
	// Should not panic
	s.PrintReport("/test/project")
}

func TestRenderBar(t *testing.T) {
	tests := []struct {
		pct  float64
		want string
	}{
		{0, "[░░░░░░░░░░]"},
		{50, "[█████░░░░░]"},
		{100, "[██████████]"},
		{-10, "[░░░░░░░░░░]"},
		{150, "[██████████]"},
	}
	for _, tt := range tests {
		got := renderBar(tt.pct, 10)
		if got != tt.want {
			t.Errorf("renderBar(%.0f, 10) = %q, want %q", tt.pct, got, tt.want)
		}
	}
}

// --- Additional audit tests ---

func TestAudit_MethodReceiver(t *testing.T) {
	dir := setupAuditDir(t, map[string]string{
		"main.go": `package main

import "fmt"

type Service struct{}

func (s *Service) Process(name string) {
	// @require len(name) > 0
	fmt.Println(name)
}

func main() {}
`,
	})
	a := NewAuditor(dir)
	report, err := a.Run()
	if err != nil {
		t.Fatal(err)
	}
	s := report.Summarize()
	if s.FuncsWithRequire != 1 {
		t.Errorf("FuncsWithRequire = %d, want 1 (method receiver)", s.FuncsWithRequire)
	}
	// Find the Process method
	var found bool
	for _, f := range report.Files {
		for _, fn := range f.Functions {
			if fn.Name == "Process" {
				found = true
				if len(fn.Params) != 1 {
					t.Errorf("Process params = %d, want 1 (name only, receiver excluded)", len(fn.Params))
				}
			}
		}
	}
	if !found {
		t.Error("Process method not found in audit")
	}
}

func TestAudit_UnnamedParams(t *testing.T) {
	dir := setupAuditDir(t, map[string]string{
		"main.go": `package main

func Handler(int, string) {}

func main() {}
`,
	})
	a := NewAuditor(dir)
	report, err := a.Run()
	if err != nil {
		t.Fatal(err)
	}
	var handler *FuncAudit
	for _, f := range report.Files {
		for i, fn := range f.Functions {
			if fn.Name == "Handler" {
				handler = &f.Functions[i]
			}
		}
	}
	if handler == nil {
		t.Fatal("Handler not found")
	}
	if len(handler.Params) != 2 {
		t.Errorf("Params = %d, want 2", len(handler.Params))
	}
	for _, p := range handler.Params {
		if p.Name != "_" {
			t.Errorf("unnamed param should have Name = %q, got %q", "_", p.Name)
		}
	}
}

func TestAudit_NilBodyFunc(t *testing.T) {
	// Interface methods and extern funcs have no body.
	// We simulate this via an interface declaration — auditFunc won't be called
	// for those, but we can test files with only interface methods yield no funcs.
	dir := setupAuditDir(t, map[string]string{
		"main.go": `package main

type Reader interface {
	Read(p []byte) (n int, err error)
}

func main() {}
`,
	})
	a := NewAuditor(dir)
	report, err := a.Run()
	if err != nil {
		t.Fatal(err)
	}
	s := report.Summarize()
	// Only main() should be found (interface methods are not FuncDecl)
	if s.TotalFuncs != 1 {
		t.Errorf("TotalFuncs = %d, want 1", s.TotalFuncs)
	}
}

func TestAudit_SkipsTestFiles(t *testing.T) {
	dir := setupAuditDir(t, map[string]string{
		"main.go": `package main

func Prod() {}

func main() {}
`,
		"main_test.go": `package main

import "testing"

func TestProd(t *testing.T) {}
`,
	})
	a := NewAuditor(dir)
	report, err := a.Run()
	if err != nil {
		t.Fatal(err)
	}
	s := report.Summarize()
	// Only main.go funcs — test file should be skipped
	if s.TotalFuncs != 2 {
		t.Errorf("TotalFuncs = %d, want 2 (Prod + main, test file skipped)", s.TotalFuncs)
	}
}

func TestAudit_SingleValueAssignNotCounted(t *testing.T) {
	// Single-value assignments with _ (e.g. `_ = someFunc()`) should not
	// be counted as error-discarding since len(LHS) < 2.
	dir := setupAuditDir(t, map[string]string{
		"main.go": `package main

func getValue() int { return 42 }

func Run() {
	_ = getValue()
}

func main() {}
`,
	})
	a := NewAuditor(dir)
	report, err := a.Run()
	if err != nil {
		t.Fatal(err)
	}
	s := report.Summarize()
	if s.TotalErrorAssignments != 0 {
		t.Errorf("TotalErrorAssignments = %d, want 0 (single-value _ not counted)", s.TotalErrorAssignments)
	}
}

func TestAuditSummary_ErrorCoverageNoAssignments(t *testing.T) {
	s := AuditSummary{
		TotalErrorAssignments:   0,
		GuardedErrorAssignments: 0,
	}
	if s.ErrorCoverage() != 100.0 {
		t.Errorf("ErrorCoverage with 0 assignments = %.1f%%, want 100.0%%", s.ErrorCoverage())
	}
}

func TestAuditSummary_FuncCoverageNoFuncs(t *testing.T) {
	s := AuditSummary{
		TotalFuncs:   0,
		FuncsWithAny: 0,
	}
	if s.FuncCoverage() != 100.0 {
		t.Errorf("FuncCoverage with 0 funcs = %.1f%%, want 100.0%%", s.FuncCoverage())
	}
}

func TestAudit_NestedDirectory(t *testing.T) {
	dir := setupAuditDir(t, map[string]string{
		"main.go": `package main

func main() {}
`,
		"sub/sub.go": `package sub

func Sub(x *int) {
	// @require -nd x
}
`,
	})
	a := NewAuditor(dir)
	report, err := a.Run()
	if err != nil {
		t.Fatal(err)
	}
	s := report.Summarize()
	if s.TotalFiles != 2 {
		t.Errorf("TotalFiles = %d, want 2", s.TotalFiles)
	}
	if s.FuncsWithRequire != 1 {
		t.Errorf("FuncsWithRequire = %d, want 1", s.FuncsWithRequire)
	}
}

func TestAudit_DirectivesInCorrectFunction(t *testing.T) {
	// Verify that directives are assigned to the correct function,
	// not bleeding across functions.
	dir := setupAuditDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func First(x *int) {
	// @require -nd x
	fmt.Println(*x)
}

func Second(y string) {
	// @require len(y) > 0
	fmt.Println(y)
}

func Third() {
	fmt.Println("no contracts")
}

func main() {}
`,
	})
	a := NewAuditor(dir)
	report, err := a.Run()
	if err != nil {
		t.Fatal(err)
	}

	funcMap := make(map[string]FuncAudit)
	for _, f := range report.Files {
		for _, fn := range f.Functions {
			funcMap[fn.Name] = fn
		}
	}

	if len(funcMap["First"].Directives) != 1 {
		t.Errorf("First should have 1 directive, got %d", len(funcMap["First"].Directives))
	}
	if len(funcMap["Second"].Directives) != 1 {
		t.Errorf("Second should have 1 directive, got %d", len(funcMap["Second"].Directives))
	}
	if len(funcMap["Third"].Directives) != 0 {
		t.Errorf("Third should have 0 directives, got %d", len(funcMap["Third"].Directives))
	}
	if funcMap["First"].HasRequire != true {
		t.Error("First should have HasRequire = true")
	}
	if funcMap["Third"].HasRequire != false {
		t.Error("Third should have HasRequire = false")
	}
}

func TestAudit_RetAndLogCounts(t *testing.T) {
	dir := setupAuditDir(t, map[string]string{
		"main.go": `package main

import "errors"

var ErrNotFound = errors.New("not found")

func Plain(x *int) {
	// @require -nd x
	_ = x
}

func WithRet(x *int) (result *int) {
	// @require -ret -nd x
	return x
}

func WithRetCustom(x *int) (*int, error) {
	// @require -ret(nil, ErrNotFound) -nd x
	return x, nil
}

func WithLog(x *int) (result *int) {
	// @require -log -nd x
	return x
}

func main() {}
`,
	})
	a := NewAuditor(dir)
	report, err := a.Run()
	if err != nil {
		t.Fatal(err)
	}

	s := report.Summarize()
	if s.RequireCount != 4 {
		t.Errorf("RequireCount = %d, want 4", s.RequireCount)
	}
	if s.RetCount != 3 {
		t.Errorf("RetCount = %d, want 3 (WithRet + WithRetCustom + WithLog)", s.RetCount)
	}
	if s.RetCustomCount != 1 {
		t.Errorf("RetCustomCount = %d, want 1 (WithRetCustom)", s.RetCustomCount)
	}
	if s.LogCount != 1 {
		t.Errorf("LogCount = %d, want 1 (WithLog)", s.LogCount)
	}
}
