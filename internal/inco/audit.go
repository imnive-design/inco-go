package inco

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AuditReport holds the full audit result for a project.
type AuditReport struct {
	Root  string
	Files []FileAudit
}

// FileAudit holds audit information for a single file.
type FileAudit struct {
	Path      string
	RelPath   string
	Functions []FuncAudit
}

// FuncAudit holds contract coverage for a single function.
type FuncAudit struct {
	Name       string
	Line       int
	Params     []ParamInfo   // parameters eligible for contracts
	Returns    []ParamInfo   // named return values
	HasRequire bool          // has at least one @require
	Directives []DirectiveAt // all directives found in this function
	// Derived from analysis
	ErrorAssignments   int // assignments that discard errors (have _ for error)
	GuardedAssignments int // error-discarding assignments covered by @must
}

// ParamInfo describes a function parameter or return value.
type ParamInfo struct {
	Name string
	Type string // human-readable type
}

// DirectiveAt records a directive and its location.
type DirectiveAt struct {
	Kind      Kind
	Line      int
	Text      string // original comment text
	Ret       bool   // has -ret flag
	RetCustom bool   // has -ret(expr, ...) custom expressions
	Log       bool   // has -log flag
}

// AuditSummary is the aggregate statistics across all files.
type AuditSummary struct {
	TotalFiles         int
	FilesWithContracts int

	TotalFuncs       int
	FuncsWithRequire int
	FuncsWithAny     int // has at least one directive of any kind

	TotalDirectives    int
	RequireCount       int
	MustCount          int
	MustRetCount       int // @must with -ret flag
	MustRetCustomCount int // @must with -ret(expr, ...) custom expressions
	MustLogCount       int // @must with -log flag
	RetCount           int // @require with -ret flag
	RetCustomCount     int // @require with -ret(expr, ...) custom expressions
	LogCount           int // @require with -log flag

	TotalErrorAssignments   int
	GuardedErrorAssignments int

	// Per-function detail for uncovered functions
	UncoveredFuncs []UncoveredFunc
}

// UncoveredFunc identifies a function that has no contract coverage.
type UncoveredFunc struct {
	File string
	Name string
	Line int
}

// Summarize computes aggregate statistics from the audit report.
func (r *AuditReport) Summarize() AuditSummary {
	// @require -nd r
	if r == nil {
		return AuditSummary{}
	}
	var s AuditSummary
	s.TotalFiles = len(r.Files)
	for _, f := range r.Files {
		fileHasContracts := false
		for _, fn := range f.Functions {
			s.TotalFuncs++
			hasAny := false
			for _, d := range fn.Directives {
				s.TotalDirectives++
				switch d.Kind {
				case Require:
					s.RequireCount++
					if d.Ret {
						s.RetCount++
						if d.RetCustom {
							s.RetCustomCount++
						}
					}
					if d.Log {
						s.LogCount++
					}
				case Must:
					s.MustCount++
					if d.Ret {
						s.MustRetCount++
						if d.RetCustom {
							s.MustRetCustomCount++
						}
					}
					if d.Log {
						s.MustLogCount++
					}
				}
			}
			if fn.HasRequire {
				s.FuncsWithRequire++
				hasAny = true
			}
			if len(fn.Directives) > 0 {
				hasAny = true
				fileHasContracts = true
			}
			if hasAny {
				s.FuncsWithAny++
			} else if fn.Name != "" {
				s.UncoveredFuncs = append(s.UncoveredFuncs, UncoveredFunc{
					File: f.RelPath,
					Name: fn.Name,
					Line: fn.Line,
				})
			}
			s.TotalErrorAssignments += fn.ErrorAssignments
			s.GuardedErrorAssignments += fn.GuardedAssignments
		}
		if fileHasContracts {
			s.FilesWithContracts++
		}
	}
	return s
}

// FuncCoverage returns the percentage of functions with at least one contract.
func (s *AuditSummary) FuncCoverage() float64 {
	// @require -ret(100.0) s.TotalFuncs > 0
	if s.TotalFuncs == 0 {
		return 100.0
	}
	return float64(s.FuncsWithAny) / float64(s.TotalFuncs) * 100.0
}

// ErrorCoverage returns the percentage of error-discarding assignments guarded by @must.
func (s *AuditSummary) ErrorCoverage() float64 {
	// @require -ret(100.0) s.TotalErrorAssignments > 0
	if s.TotalErrorAssignments == 0 {
		return 100.0
	}
	return float64(s.GuardedErrorAssignments) / float64(s.TotalErrorAssignments) * 100.0
}

// PrintReport writes a human-readable audit report to stdout.
func (s *AuditSummary) PrintReport(root string) {
	// @require len(root) > 0, "root must not be empty"
	fmt.Println("inco audit — contract coverage report")
	fmt.Printf("root: %s\n", root)
	fmt.Println(strings.Repeat("─", 60))

	// Directive counts
	fmt.Printf("\n  %-24s %d\n", "Files scanned:", s.TotalFiles)
	fmt.Printf("  %-24s %d\n", "Files with contracts:", s.FilesWithContracts)
	fmt.Printf("  %-24s %d\n", "Functions found:", s.TotalFuncs)
	fmt.Println()

	fmt.Println("  Directives:")
	fmt.Printf("    @require             %d\n", s.RequireCount)
	if s.RetCount > 0 {
		fmt.Printf("      ├─ -ret            %d\n", s.RetCount)
	}
	if s.RetCustomCount > 0 {
		fmt.Printf("      ├─ -ret(...)       %d\n", s.RetCustomCount)
	}
	if s.LogCount > 0 {
		fmt.Printf("      └─ -log            %d\n", s.LogCount)
	}
	fmt.Printf("    @must                %d\n", s.MustCount)
	if s.MustRetCount > 0 {
		fmt.Printf("      ├─ -ret            %d\n", s.MustRetCount)
	}
	if s.MustRetCustomCount > 0 {
		fmt.Printf("      ├─ -ret(...)       %d\n", s.MustRetCustomCount)
	}
	if s.MustLogCount > 0 {
		fmt.Printf("      └─ -log            %d\n", s.MustLogCount)
	}
	fmt.Printf("    total                %d\n", s.TotalDirectives)
	fmt.Println()

	// Coverage
	fmt.Println("  Coverage:")
	fmt.Printf("    functions w/ contracts   %d / %d  (%.1f%%)\n",
		s.FuncsWithAny, s.TotalFuncs, s.FuncCoverage())
	fmt.Printf("      └─ with @require       %d\n", s.FuncsWithRequire)
	fmt.Printf("    error assigns guarded    %d / %d  (%.1f%%)\n",
		s.GuardedErrorAssignments, s.TotalErrorAssignments, s.ErrorCoverage())
	fmt.Println()

	// Uncovered functions
	if len(s.UncoveredFuncs) > 0 {
		fmt.Println(strings.Repeat("─", 60))
		fmt.Printf("  Uncovered functions (%d):\n", len(s.UncoveredFuncs))
		for _, uf := range s.UncoveredFuncs {
			fmt.Printf("    %s:%d  %s\n", uf.File, uf.Line, uf.Name)
		}
		fmt.Println()
	}

	// Overall bar
	fmt.Println(strings.Repeat("─", 60))
	cov := s.FuncCoverage()
	bar := renderBar(cov, 30)
	fmt.Printf("  contract coverage:  %s  %.1f%%\n", bar, cov)
	fmt.Println()
}

// renderBar renders a simple ASCII progress bar.
func renderBar(pct float64, width int) string {
	// @require width > 0, "width must be positive"
	filled := int(pct / 100.0 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}

// --- Auditor: the analysis engine ---

// commentDirective associates a parsed Directive with its source location.
type commentDirective struct {
	d    *Directive
	pos  token.Pos
	line int
	text string
}

// Auditor walks Go source files and collects contract coverage information.
type Auditor struct {
	Root string
}

// NewAuditor creates a new Auditor for the given project root.
func NewAuditor(root string) (a *Auditor) {
	// @require len(root) > 0, "root must not be empty"
	return &Auditor{Root: root}
}

// Run performs the audit and returns the report.
func (a *Auditor) Run() (report *AuditReport, err error) {
	report = &AuditReport{Root: a.Root}

	// @must -ret
	err = filepath.Walk(a.Root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			base := info.Name()
			if strings.HasPrefix(base, ".") || base == "vendor" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fa, _ := a.auditFile(path) // @must -ret
		if fa != nil {
			report.Files = append(report.Files, *fa)
		}
		return nil
	})

	// Sort files by path for deterministic output
	sort.Slice(report.Files, func(i, j int) bool {
		return report.Files[i].RelPath < report.Files[j].RelPath
	})

	return report, nil
}

// auditFile analyzes a single Go file for contract coverage.
func (a *Auditor) auditFile(path string) (_ *FileAudit, err error) {
	// @require len(path) > 0, "path must not be empty"
	absPath, _ := filepath.Abs(path) // @must -ret
	relPath, relErr := filepath.Rel(a.Root, absPath)
	if relErr != nil {
		relPath = absPath
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, absPath, nil, parser.ParseComments) // @must -ret(nil, fmt.Errorf("inco audit: parse %s: %w", relPath, err))

	fa := &FileAudit{
		Path:    absPath,
		RelPath: relPath,
	}

	// Collect all directives with positions
	var allDirectives []commentDirective
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			d := ParseDirective(c.Text)
			if d != nil {
				allDirectives = append(allDirectives, commentDirective{
					d:    d,
					pos:  c.Pos(),
					line: fset.Position(c.Pos()).Line,
					text: c.Text,
				})
			}
		}
	}

	// Walk all function declarations (including methods)
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			fnAudit := a.auditFunc(node, fset, f, allDirectives)
			fa.Functions = append(fa.Functions, fnAudit)
			return false // don't recurse into function body for FuncDecl; auditFunc handles it
		}
		return true
	})

	// Skip files with no functions
	if len(fa.Functions) == 0 {
		return nil, nil
	}

	return fa, nil
}

// auditFunc analyzes a single function for contract coverage.
func (a *Auditor) auditFunc(fn *ast.FuncDecl, fset *token.FileSet, f *ast.File, allDirectives []commentDirective) FuncAudit {
	// @require -nd fn, fset, f
	pos := fset.Position(fn.Pos())
	audit := FuncAudit{
		Name: fn.Name.Name,
		Line: pos.Line,
	}

	// Collect parameter info
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			typStr := types.ExprString(field.Type)
			for _, name := range field.Names {
				audit.Params = append(audit.Params, ParamInfo{
					Name: name.Name,
					Type: typStr,
				})
			}
			// Unnamed params (e.g., func(int, string))
			if len(field.Names) == 0 {
				audit.Params = append(audit.Params, ParamInfo{
					Name: "_",
					Type: typStr,
				})
			}
		}
	}

	// Collect named return value info
	if fn.Type.Results != nil {
		for _, field := range fn.Type.Results.List {
			typStr := types.ExprString(field.Type)
			for _, name := range field.Names {
				audit.Returns = append(audit.Returns, ParamInfo{
					Name: name.Name,
					Type: typStr,
				})
			}
		}
	}

	// Find directives inside this function's body
	if fn.Body == nil {
		return audit
	}

	bodyStart := fn.Body.Lbrace
	bodyEnd := fn.Body.Rbrace

	for _, cd := range allDirectives {
		if cd.pos > bodyStart && cd.pos < bodyEnd {
			audit.Directives = append(audit.Directives, DirectiveAt{
				Kind:      cd.d.Kind,
				Line:      cd.line,
				Text:      cd.text,
				Ret:       cd.d.Ret,
				RetCustom: cd.d.Ret && len(cd.d.RetExprs) > 0,
				Log:       cd.d.Log,
			})
			switch cd.d.Kind {
			case Require:
				audit.HasRequire = true
			}
		}
	}

	// Count error-discarding assignments and @must coverage
	a.countErrorAssignments(fn.Body, fset, allDirectives, &audit)

	return audit
}

// countErrorAssignments walks a function body and counts assignments where
// an error is discarded with _ and whether they have @must coverage.
func (a *Auditor) countErrorAssignments(body *ast.BlockStmt, fset *token.FileSet, allDirectives []commentDirective, audit *FuncAudit) {
	// @require -nd body, fset, audit
	// Track which @must directives have been consumed (each guards exactly one assignment)
	consumed := make(map[int]bool) // key: directive line number

	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		// Check if any LHS is _ (potential error discard)
		hasBlank := false
		for _, lhs := range assign.Lhs {
			ident, ok := lhs.(*ast.Ident)
			if ok && ident.Name == "_" {
				hasBlank = true
				break
			}
		}
		if !hasBlank {
			return true
		}

		// Heuristic: multi-value assignment with _ on LHS likely discards an error
		if len(assign.Lhs) < 2 {
			return true
		}

		audit.ErrorAssignments++

		// Check if this assignment is guarded by @must
		assignLine := fset.Position(assign.Pos()).Line
		for _, cd := range allDirectives {
			if cd.d.Kind != Must || consumed[cd.line] {
				continue
			}
			// Inline @must: same line as assignment
			if cd.line == assignLine {
				audit.GuardedAssignments++
				consumed[cd.line] = true
				return true
			}
			// Block @must: directive on a preceding line, close to the assignment.
			// For multi-line calls the gap can be a few lines, but each @must
			// guards exactly one assignment (tracked via consumed map).
			if cd.line < assignLine && cd.line >= assignLine-5 {
				audit.GuardedAssignments++
				consumed[cd.line] = true
				return true
			}
		}

		return true
	})
}
