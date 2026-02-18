package inco

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// Overlay represents the go build -overlay JSON format.
type Overlay struct {
	Replace map[string]string `json:"Replace"`
}

// Engine is the core processor that scans Go source files,
// parses contract directives, injects assertion code, and
// produces overlay mappings for `go build -overlay`.
type Engine struct {
	Root      string // project root directory
	CacheDir  string // .inco_cache directory path
	Overlay   Overlay
	typeCache map[string]*packageCache
}

type packageCache struct {
	fset     *token.FileSet
	files    map[string]*ast.File
	resolver *TypeResolver
}

// NewEngine creates a new Engine rooted at the given directory.
func NewEngine(root string) (e *Engine) {
	// @require len(root) > 0, "root must not be empty"
	cache := filepath.Join(root, ".inco_cache")
	return &Engine{
		Root:      root,
		CacheDir:  cache,
		Overlay:   Overlay{Replace: make(map[string]string)},
		typeCache: make(map[string]*packageCache),
	}
}

// Run executes the full pipeline: scan -> parse -> inject -> write overlay.
func (e *Engine) Run() (err error) {
	err = os.MkdirAll(e.CacheDir, 0o755) // @must -ret(fmt.Errorf("inco: create cache dir: %w", err))

	// @must -ret
	err = filepath.Walk(e.Root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Skip hidden dirs, vendor, testdata, and cache itself
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
		return e.processFile(path)
	})

	return e.writeOverlay()
}

// processFile scans a single Go file for contract directives.
// If any are found, it generates a shadow file and registers it in the overlay.
func (e *Engine) processFile(path string) (err error) {
	// @require len(path) > 0, "path must not be empty"
	absPath, err := filepath.Abs(path) // @must -ret(fmt.Errorf("inco: abs path %s: %w", path, err))

	f, fset, resolver, err := e.loadFileWithTypes(absPath) // @must -ret

	directives := e.collectDirectives(f, fset)
	if len(directives) == 0 {
		return nil // nothing to do
	}

	// Read original source lines for //line mapping
	origLines, err := readLines(absPath) // @must -ret(fmt.Errorf("inco: read original %s: %w", path, err))

	// Inject assertions into AST
	e.injectAssertions(f, fset, directives, resolver)

	// Strip all comments to prevent go/printer from displacing them
	// into injected code. The shadow file is for compilation only.
	f.Comments = nil

	// Generate shadow file content
	var buf strings.Builder
	cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}
	err = cfg.Fprint(&buf, fset, f) // @must -ret(fmt.Errorf("inco: print shadow for %s: %w", path, err))

	// Post-process: inject //line directives to map back to original source
	shadowContent := injectLineDirectives(buf.String(), origLines, absPath)

	// Compute content hash for stable cache filenames
	hash := contentHash(shadowContent)
	base := strings.TrimSuffix(filepath.Base(path), ".go")
	shadowName := fmt.Sprintf("%s_%s.go", base, hash[:12])
	shadowPath := filepath.Join(e.CacheDir, shadowName)

	err = os.WriteFile(shadowPath, []byte(shadowContent), 0o644) // @must -ret(fmt.Errorf("inco: write shadow %s: %w", shadowPath, err))

	e.Overlay.Replace[absPath] = shadowPath
	return nil
}

func (e *Engine) loadFileWithTypes(path string) (f *ast.File, fset *token.FileSet, resolver *TypeResolver, err error) {
	// @require len(path) > 0, "path must not be empty"
	dir := filepath.Dir(path)
	cache, ok := e.typeCache[dir]
	if !ok {
		cache, err = e.loadPackage(dir) // @must -ret
		e.typeCache[dir] = cache
	}

	f = cache.files[path]
	// @require -ret(nil, nil, nil, fmt.Errorf("inco: file not found in package: %s", path)) f != nil

	return f, cache.fset, cache.resolver, nil
}

func (e *Engine) loadPackage(dir string) (*packageCache, error) {
	// @require len(dir) > 0, "dir must not be empty"
	fset := token.NewFileSet()
	// @must -ret(nil, fmt.Errorf("inco: parse dir %s: %w", dir, err))
	pkgs, err := parser.ParseDir(fset, dir, func(info os.FileInfo) bool {
		name := info.Name()
		return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	}, parser.ParseComments)

	var pkg *ast.Package
	for _, p := range pkgs {
		pkg = p
		break
	}
	// @require -ret(nil, fmt.Errorf("inco: no package in %s", dir)) pkg != nil

	fileMap := make(map[string]*ast.File)
	var files []*ast.File
	for _, f := range pkg.Files {
		filename := fset.Position(f.Pos()).Filename
		abs, _ := filepath.Abs(filename) // @must -ret
		fileMap[abs] = f
		files = append(files, f)
	}

	info := &types.Info{
		Types:  make(map[ast.Expr]types.TypeAndValue),
		Defs:   make(map[*ast.Ident]types.Object),
		Uses:   make(map[*ast.Ident]types.Object),
		Scopes: make(map[ast.Node]*types.Scope),
	}
	conf := &types.Config{
		Importer: importer.ForCompiler(fset, "source", nil),
		Error:    func(err error) {}, // collect but don't abort on first error
	}
	pkgTypes, err := conf.Check(pkg.Name, fset, files, info)
	if err != nil {
		// Typecheck warnings are diagnostic — code generation continues regardless.
		fmt.Fprintf(os.Stderr, "inco: typecheck warning in %s: %v\n", dir, err)
	}

	resolver := &TypeResolver{Info: info, Fset: fset, Pkg: pkgTypes}
	return &packageCache{fset: fset, files: fileMap, resolver: resolver}, nil
}

// directiveInfo associates a parsed Directive with its position in the AST.
type directiveInfo struct {
	Directive *Directive
	Pos       token.Pos
	Comment   *ast.Comment
}

// collectDirectives walks the AST comment map and extracts all contract directives.
func (e *Engine) collectDirectives(f *ast.File, fset *token.FileSet) []directiveInfo {
	// @require -nd f, fset
	var result []directiveInfo
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			d := ParseDirective(c.Text)
			if d != nil {
				result = append(result, directiveInfo{
					Directive: d,
					Pos:       c.Pos(),
					Comment:   c,
				})
			}
		}
	}
	return result
}

// injectAssertions modifies the AST by inserting assertion statements
// after each contract directive comment.
func (e *Engine) injectAssertions(f *ast.File, fset *token.FileSet, directives []directiveInfo, resolver *TypeResolver) {
	// @require -nd f, fset
	// Build a position -> directive lookup
	dirMap := make(map[token.Pos]*directiveInfo)
	for i := range directives {
		dirMap[directives[i].Pos] = &directives[i]
	}

	var importsToAdd []string

	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BlockStmt:
			if node != nil {
				var added []string
				node.List, added = e.processStmtList(node.List, node.Lbrace, fset, dirMap, resolver, f)
				importsToAdd = append(importsToAdd, added...)
			}
		case *ast.CaseClause:
			if node != nil {
				var added []string
				node.Body, added = e.processStmtList(node.Body, node.Colon, fset, dirMap, resolver, f)
				importsToAdd = append(importsToAdd, added...)
			}
		case *ast.CommClause:
			if node != nil {
				var added []string
				node.Body, added = e.processStmtList(node.Body, node.Colon, fset, dirMap, resolver, f)
				importsToAdd = append(importsToAdd, added...)
			}
		}
		return true
	})

	if len(importsToAdd) > 0 {
		for _, path := range uniqStrings(importsToAdd) {
			astutil.AddImport(fset, f, path)
		}
	}
}

// processStmtList inspects a statement list and injects assertions where directives are found.
// startPos is the position of the opening brace or colon that precedes the first statement.
func (e *Engine) processStmtList(stmts []ast.Stmt, startPos token.Pos, fset *token.FileSet, dirMap map[token.Pos]*directiveInfo, resolver *TypeResolver, f *ast.File) ([]ast.Stmt, []string) {
	// @require -nd fset, dirMap, f
	var newList []ast.Stmt
	var importsToAdd []string

	for i, stmt := range stmts {
		// Check if any directive is associated with this statement
		// by looking at comments that appear before this statement's position
		// and after the previous statement (or block/clause start).
		var prevEnd token.Pos
		if i > 0 {
			prevEnd = stmts[i-1].End()
		} else {
			prevEnd = startPos
		}

		// Collect directives that appear between prevEnd and this statement.
		// Sort by position to ensure deterministic injection order.
		var between []token.Pos
		for pos := range dirMap {
			if pos > prevEnd && pos < stmt.Pos() {
				between = append(between, pos)
			}
		}
		sort.Slice(between, func(a, b int) bool { return between[a] < between[b] })

		var pendingMust *directiveInfo
		var pendingMustPos token.Pos
		for _, pos := range between {
			di := dirMap[pos]
			if di.Directive.Kind == Must {
				// Block-mode @must: directive on its own line, applies to next statement
				pendingMust = di
				pendingMustPos = pos
			} else {
				generated, added := e.generateAssertion(di, fset, resolver, f)
				newList = append(newList, generated...)
				importsToAdd = append(importsToAdd, added...)
				delete(dirMap, pos) // consumed
			}
		}

		// Handle block-mode @must: applies to the next assignment statement
		if pendingMust != nil {
			if assign, ok := stmt.(*ast.AssignStmt); ok {
				mustStmts, mustImports := e.generateMustForAssign(assign, fset, pendingMust, resolver, f)
				newList = append(newList, stmt)
				newList = append(newList, mustStmts...)
				importsToAdd = append(importsToAdd, mustImports...)
				stmt = nil                     // mark as handled
				delete(dirMap, pendingMustPos) // consumed
			}
		}

		// Handle inline // @must on assignment statements (same line)
		if stmt != nil {
			if assign, ok := stmt.(*ast.AssignStmt); ok {
				for pos, di := range dirMap {
					stmtLine := fset.Position(stmt.Pos()).Line
					commentLine := fset.Position(pos).Line
					if di.Directive.Kind == Must && commentLine == stmtLine {
						mustStmts, mustImports := e.generateMustForAssign(assign, fset, di, resolver, f)
						newList = append(newList, stmt)
						newList = append(newList, mustStmts...)
						importsToAdd = append(importsToAdd, mustImports...)
						stmt = nil          // mark as handled
						delete(dirMap, pos) // consumed
						break
					}
				}
			}
		}

		if stmt != nil {
			newList = append(newList, stmt)
		}
	}

	return newList, importsToAdd
}

// generateAssertion creates assertion statements from a directive.
func (e *Engine) generateAssertion(di *directiveInfo, fset *token.FileSet, resolver *TypeResolver, f *ast.File) ([]ast.Stmt, []string) {
	// @require -nd di, fset, f
	pos := fset.Position(di.Pos)
	loc := fmt.Sprintf("%s:%d", pos.Filename, pos.Line)

	switch di.Directive.Kind {
	case Require:
		return e.generateRequire(di.Directive, loc, resolver, di.Pos, f)
	default:
		return nil, nil
	}
}

// generateRequire generates assertion statements for require directives.
// In default mode, generates `if <cond> { panic(...) }`.
// With -ret flag, generates `if <cond> { return }` (or return with zero values).
// With -log flag, generates `if <cond> { log.Println(...); return }` (auto-imports log).
func (e *Engine) generateRequire(d *Directive, loc string, resolver *TypeResolver, pos token.Pos, f *ast.File) ([]ast.Stmt, []string) {
	// @require -nd d
	// @require len(loc) > 0, "loc must not be empty"
	if d.Ret || d.Log {
		return e.generateRequireRet(d, loc, resolver, pos, f)
	}
	if d.ND {
		return e.generateNDChecks(d.Vars, loc, "require", resolver, pos, f)
	}
	if d.Expr != "" {
		msg := d.Message
		if msg == "" {
			msg = fmt.Sprintf("inco // require violation: %s", d.Expr)
		}
		if resolver != nil {
			if warn := resolver.EvalRequireExpr(pos, d.Expr); warn != "" {
				fmt.Fprintf(os.Stderr, "inco: %s at %s\n", warn, loc)
			}
		}
		expr, _ := parser.ParseExpr(d.Expr) // @must -log
		cond := &ast.UnaryExpr{Op: token.NOT, X: &ast.ParenExpr{X: expr}}
		return []ast.Stmt{makeIfPanicStmt(cond, fmt.Sprintf("%s at %s", msg, loc))}, nil
	}
	return nil, nil
}

// generateRequireRet generates require checks that return instead of panicking.
func (e *Engine) generateRequireRet(d *Directive, loc string, resolver *TypeResolver, pos token.Pos, f *ast.File) ([]ast.Stmt, []string) {
	// @require -nd d
	// @require len(loc) > 0, "loc must not be empty"

	var retStmt *ast.ReturnStmt
	var retImports []string

	// Use custom return expressions if provided, otherwise build zero-value return
	if len(d.RetExprs) > 0 {
		var retErr error
		retStmt, retErr = buildCustomReturnStmt(d.RetExprs) // @must -log
		_ = retErr
	} else {
		retStmt, retImports = e.buildReturnStmt(f, pos, resolver)
	}

	var importsToAdd []string
	importsToAdd = append(importsToAdd, retImports...)

	if d.Log {
		importsToAdd = append(importsToAdd, "log")
	}

	if d.ND {
		stmts, ndImports := e.generateNDRetChecks(d.Vars, loc, resolver, pos, f, retStmt, d.Log)
		importsToAdd = append(importsToAdd, ndImports...)
		return stmts, importsToAdd
	}

	if d.Expr != "" {
		msg := d.Message
		if msg == "" {
			msg = fmt.Sprintf("inco // require -ret violation: %s", d.Expr)
		}
		if resolver != nil {
			if warn := resolver.EvalRequireExpr(pos, d.Expr); warn != "" {
				fmt.Fprintf(os.Stderr, "inco: %s at %s\n", warn, loc)
			}
		}
		expr, _ := parser.ParseExpr(d.Expr) // @must -log
		cond := &ast.UnaryExpr{Op: token.NOT, X: &ast.ParenExpr{X: expr}}

		var body []ast.Stmt
		if d.Log {
			body = append(body, makeLogStmt(fmt.Sprintf("%s at %s", msg, loc)))
		}
		body = append(body, retStmt)

		stmt := &ast.IfStmt{
			Cond: cond,
			Body: &ast.BlockStmt{List: body},
		}
		return []ast.Stmt{stmt}, importsToAdd
	}

	return nil, nil
}

// generateNDRetChecks generates non-defaulted checks that return instead of panicking.
func (e *Engine) generateNDRetChecks(vars []string, loc string, resolver *TypeResolver, pos token.Pos, f *ast.File, retStmt *ast.ReturnStmt, doLog bool) ([]ast.Stmt, []string) {
	// @require len(vars) > 0, "vars must not be empty"
	// @require len(loc) > 0, "loc must not be empty"
	// @require -nd resolver
	var stmts []ast.Stmt
	var importsToAdd []string

	funcType := findEnclosingFuncType(f, pos)
	for _, v := range vars {
		typ := resolver.ResolveVarType(funcType, v)
		currentPkg := resolver.Pkg
		zeroExpr := ZeroCheckExpr(v, typ, currentPkg)
		if zeroExpr == nil {
			zeroExpr = &ast.BinaryExpr{X: ast.NewIdent(v), Op: token.EQL, Y: ast.NewIdent("nil")}
		}

		desc := ZeroValueDesc(typ)
		msg := fmt.Sprintf("inco // require -ret -nd violation: [%s] is defaulted (%s) at %s", v, desc, loc)

		var body []ast.Stmt
		if doLog {
			body = append(body, makeLogStmt(msg))
		}
		body = append(body, retStmt)

		stmts = append(stmts, &ast.IfStmt{
			Cond: zeroExpr,
			Body: &ast.BlockStmt{List: body},
		})

		if imp := NeedsImport(typ, resolver.Pkg); imp != "" {
			importsToAdd = append(importsToAdd, imp)
		}
	}

	return stmts, importsToAdd
}

// generateNDChecks generates non-defaulted zero-value panic checks with type awareness.
func (e *Engine) generateNDChecks(vars []string, loc string, protocol string, resolver *TypeResolver, pos token.Pos, f *ast.File) ([]ast.Stmt, []string) {
	// @require len(vars) > 0, "vars must not be empty"
	// @require len(loc) > 0, "loc must not be empty"
	// @require -nd resolver
	var stmts []ast.Stmt
	var importsToAdd []string

	funcType := findEnclosingFuncType(f, pos)
	for _, v := range vars {
		typ := resolver.ResolveVarType(funcType, v)
		currentPkg := resolver.Pkg
		zeroExpr := ZeroCheckExpr(v, typ, currentPkg)
		if zeroExpr == nil {
			zeroExpr = &ast.BinaryExpr{X: ast.NewIdent(v), Op: token.EQL, Y: ast.NewIdent("nil")}
		}

		desc := ZeroValueDesc(typ)
		msg := fmt.Sprintf("inco // %s -nd violation: [%s] is defaulted (%s) at %s", protocol, v, desc, loc)
		stmts = append(stmts, makeIfPanicStmt(zeroExpr, msg))

		if imp := NeedsImport(typ, resolver.Pkg); imp != "" {
			importsToAdd = append(importsToAdd, imp)
		}
	}

	return stmts, importsToAdd
}

// generateMustForAssign injects error checking after an assignment that uses _ for the error.
// It attempts to use type information to identify the error variable. If type info is unavailable,
// it falls back to heuristics (last blank identifier or explicit "err" variable).
func (e *Engine) generateMustForAssign(assign *ast.AssignStmt, fset *token.FileSet, di *directiveInfo, resolver *TypeResolver, f *ast.File) ([]ast.Stmt, []string) {
	// @require -nd assign, fset, di
	pos := fset.Position(di.Pos)
	loc := fmt.Sprintf("%s:%d", pos.Filename, pos.Line)

	var errorIdent *ast.Ident

	// Strategy 1: Type-based resolution
	if resolver != nil && resolver.Info != nil {
		if len(assign.Rhs) == 1 {
			// Function call returning multiple values OR single value
			if tv, ok := resolver.Info.Types[assign.Rhs[0]]; ok {
				if tuple, ok := tv.Type.(*types.Tuple); ok {
					// Multi-value return: (T, error)
					for i := 0; i < tuple.Len() && i < len(assign.Lhs); i++ {
						if IsErrorType(tuple.At(i).Type()) {
							if ident, ok := assign.Lhs[i].(*ast.Ident); ok {
								errorIdent = ident
							}
						}
					}
				} else if IsErrorType(tv.Type) {
					// Single value return: error
					if len(assign.Lhs) == 1 {
						if ident, ok := assign.Lhs[0].(*ast.Ident); ok {
							errorIdent = ident
						}
					}
				}
			}
		} else if len(assign.Lhs) == len(assign.Rhs) {
			// Multiple assignments: a, err = x, y
			for i, rhs := range assign.Rhs {
				if i < len(assign.Lhs) {
					if tv, ok := resolver.Info.Types[rhs]; ok {
						if IsErrorType(tv.Type) {
							if ident, ok := assign.Lhs[i].(*ast.Ident); ok {
								errorIdent = ident
							}
						}
					}
				}
			}
		}
	}

	// Strategy 2: Heuristic fallback (if type resolution failed or didn't find an error)
	if errorIdent == nil {
		// Find the LAST _ (blank identifier) in LHS — that's the error position by Go convention.
		for _, lhs := range assign.Lhs {
			ident, ok := lhs.(*ast.Ident)
			if ok && ident.Name == "_" {
				errorIdent = ident
			}
		}

		// If still no blank identifier found, check for explicit err variable
		if errorIdent == nil {
			for _, lhs := range assign.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if ok && ident.Name == "err" {
					errorIdent = ident
					break
				}
			}
		}
	}

	if errorIdent != nil {
		targetName := errorIdent.Name
		if targetName == "_" {
			// Replace _ with generated variable
			targetName = fmt.Sprintf("_inco_err_%d", pos.Line)
			errorIdent.Name = targetName

			// Ensure it's a short variable declaration so the new name is declared
			if assign.Tok == token.ASSIGN {
				assign.Tok = token.DEFINE
			}
		}

		// -log mode: log error and return (implies -ret)
		// -ret mode: return error instead of panicking
		if di.Directive.Ret {
			return e.generateMustRetStmts(targetName, di, resolver, f)
		}

		msg := fmt.Sprintf("inco // must violation at %s", loc)
		return []ast.Stmt{makeIfPanicErrStmt(targetName, msg)}, nil
	}

	return nil, nil
}

// generateMustRetStmts generates return-on-error statements for @must -ret / @must -log.
// Instead of panicking, it returns the error (and zero values for other returns).
// With -log, it also logs the error before returning.
func (e *Engine) generateMustRetStmts(errVar string, di *directiveInfo, resolver *TypeResolver, f *ast.File) ([]ast.Stmt, []string) {
	// @require len(errVar) > 0, "errVar must not be empty"
	// @require -nd di

	var retStmt *ast.ReturnStmt
	var retImports []string
	var importsToAdd []string

	if len(di.Directive.RetExprs) > 0 {
		// Custom return expressions: @must -ret(expr1, expr2)
		var retErr error
		retStmt, retErr = buildCustomReturnStmt(di.Directive.RetExprs) // @must -ret
		_ = retErr
	} else {
		// Auto-build return: zero values for non-error returns, errVar for error return
		retStmt, retImports = e.buildMustReturnStmt(f, di.Pos, resolver, errVar)
		importsToAdd = append(importsToAdd, retImports...)
	}

	// -log mode: wrap return in log + return block
	if di.Directive.Log {
		importsToAdd = append(importsToAdd, "log")
		pos := di.Pos
		loc := fmt.Sprintf("%s:%d", resolver.Fset.Position(pos).Filename, resolver.Fset.Position(pos).Line)
		logMsg := fmt.Sprintf("inco // must -log violation at %s: ", loc)
		logStmt := &ast.ExprStmt{
			X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("log"),
					Sel: ast.NewIdent("Println"),
				},
				Args: []ast.Expr{
					&ast.BinaryExpr{
						X:  &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(logMsg)},
						Op: token.ADD,
						Y: &ast.CallExpr{
							Fun: &ast.SelectorExpr{
								X:   ast.NewIdent(errVar),
								Sel: ast.NewIdent("Error"),
							},
						},
					},
				},
			},
		}
		return []ast.Stmt{makeIfLogReturnErrStmt(errVar, logStmt, retStmt)}, importsToAdd
	}

	return []ast.Stmt{makeIfReturnErrStmt(errVar, retStmt)}, importsToAdd
}

// buildMustReturnStmt creates a return statement for @must -ret.
// Non-error return positions get zero values; the error position gets errVar.
func (e *Engine) buildMustReturnStmt(f *ast.File, pos token.Pos, resolver *TypeResolver, errVar string) (*ast.ReturnStmt, []string) {
	// @require -nd f, resolver
	// @require len(errVar) > 0, "errVar must not be empty"

	funcType := findEnclosingFuncType(f, pos)
	if funcType == nil || funcType.Results == nil || funcType.Results.NumFields() == 0 {
		// No return values — just return the error variable (shouldn't normally happen)
		return &ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(errVar)}}, nil
	}

	// Build explicit return: zero values for non-error, errVar for error positions
	var results []ast.Expr
	var importsToAdd []string

	for _, field := range funcType.Results.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}

		// Determine if this return type is error
		isErr := false
		if tv, ok := resolver.Info.Types[field.Type]; ok {
			isErr = IsErrorType(tv.Type)
		}
		if !isErr {
			// AST heuristic fallback
			if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == "error" {
				isErr = true
			}
		}

		for i := 0; i < count; i++ {
			if isErr {
				results = append(results, ast.NewIdent(errVar))
			} else {
				var zeroExpr ast.Expr
				if tv, ok := resolver.Info.Types[field.Type]; ok {
					zeroExpr = ZeroValueLiteral(tv.Type, resolver.Pkg)
				}
				if zeroExpr == nil {
					zeroExpr = zeroValueFromASTType(field.Type)
				}
				results = append(results, zeroExpr)
			}
		}
	}

	return &ast.ReturnStmt{Results: results}, importsToAdd
}

// makeIfReturnErrStmt builds: if <errVar> != nil { return ... }
func makeIfReturnErrStmt(errVar string, retStmt *ast.ReturnStmt) *ast.IfStmt {
	// @require len(errVar) > 0, "errVar must not be empty"
	// @require -nd retStmt
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{
			X:  ast.NewIdent(errVar),
			Op: token.NEQ,
			Y:  ast.NewIdent("nil"),
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{retStmt},
		},
	}
}

// makeIfLogReturnErrStmt builds: if <errVar> != nil { <logStmt>; return ... }
func makeIfLogReturnErrStmt(errVar string, logStmt ast.Stmt, retStmt *ast.ReturnStmt) *ast.IfStmt {
	// @require len(errVar) > 0, "errVar must not be empty"
	// @require -nd logStmt, retStmt
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{
			X:  ast.NewIdent(errVar),
			Op: token.NEQ,
			Y:  ast.NewIdent("nil"),
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{logStmt, retStmt},
		},
	}
}

// writeOverlay writes the overlay.json file to the cache directory.
func (e *Engine) writeOverlay() (err error) {
	if len(e.Overlay.Replace) == 0 {
		return nil
	}

	data, err := json.MarshalIndent(e.Overlay, "", "  ") // @must -ret(fmt.Errorf("inco: marshal overlay: %w", err))

	path := filepath.Join(e.CacheDir, "overlay.json")
	err = os.WriteFile(path, data, 0o644) // @must -ret(fmt.Errorf("inco: write overlay.json: %w", err))

	fmt.Printf("inco: overlay written to %s (%d file(s) mapped)\n", path, len(e.Overlay.Replace))
	return nil
}

// --- AST construction helpers ---

// makeIfPanicStmt builds: if <cond> { panic("<msg>") }
func makeIfPanicStmt(cond ast.Expr, msg string) *ast.IfStmt {
	// @require -nd cond
	// @require len(msg) > 0, "msg must not be empty"
	return &ast.IfStmt{
		Cond: cond,
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ExprStmt{
					X: &ast.CallExpr{
						Fun:  ast.NewIdent("panic"),
						Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(msg)}},
					},
				},
			},
		},
	}
}

// makeIfPanicErrStmt builds: if <errVar> != nil { panic("<msg>: " + <errVar>.Error()) }
func makeIfPanicErrStmt(errVar string, msg string) *ast.IfStmt {
	// @require len(errVar) > 0, "errVar must not be empty"
	// @require len(msg) > 0, "msg must not be empty"
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{
			X:  ast.NewIdent(errVar),
			Op: token.NEQ,
			Y:  ast.NewIdent("nil"),
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ExprStmt{
					X: &ast.CallExpr{
						Fun: ast.NewIdent("panic"),
						Args: []ast.Expr{
							&ast.BinaryExpr{
								X:  &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(msg + ": ")},
								Op: token.ADD,
								Y: &ast.CallExpr{
									Fun: &ast.SelectorExpr{
										X:   ast.NewIdent(errVar),
										Sel: ast.NewIdent("Error"),
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// makeLogStmt builds: log.Println("<msg>")
func makeLogStmt(msg string) ast.Stmt {
	// @require len(msg) > 0, "msg must not be empty"
	return &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent("log"),
				Sel: ast.NewIdent("Println"),
			},
			Args: []ast.Expr{
				&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(msg)},
			},
		},
	}
}

// buildReturnStmt creates a return statement with appropriate zero values for
// the enclosing function's return types.
//
// For named returns: bare return (no values).
// For unnamed returns: return with zero-value literals for each return type.
// For void functions: bare return.
func (e *Engine) buildReturnStmt(f *ast.File, pos token.Pos, resolver *TypeResolver) (*ast.ReturnStmt, []string) {
	// @require -nd f
	funcType := findEnclosingFuncType(f, pos)
	if funcType == nil || funcType.Results == nil || funcType.Results.NumFields() == 0 {
		return &ast.ReturnStmt{}, nil
	}

	// Check if all return values are named → bare return
	allNamed := true
	for _, field := range funcType.Results.List {
		if len(field.Names) == 0 {
			allNamed = false
			break
		}
	}
	if allNamed {
		return &ast.ReturnStmt{}, nil
	}

	// Generate zero-value expressions for unnamed returns
	var results []ast.Expr
	var importsToAdd []string
	for _, field := range funcType.Results.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}

		var zeroExpr ast.Expr
		if resolver != nil {
			if tv, ok := resolver.Info.Types[field.Type]; ok {
				zeroExpr = ZeroValueLiteral(tv.Type, resolver.Pkg)
			}
		}
		if zeroExpr == nil {
			zeroExpr = zeroValueFromASTType(field.Type)
		}

		for i := 0; i < count; i++ {
			results = append(results, zeroExpr)
		}
	}

	return &ast.ReturnStmt{Results: results}, importsToAdd
}

// buildCustomReturnStmt parses custom return expression strings and builds
// a return statement with those expressions.
func buildCustomReturnStmt(exprs []string) (_ *ast.ReturnStmt, err error) {
	// @require len(exprs) > 0, "exprs must not be empty"
	var results []ast.Expr
	var expr ast.Expr
	for _, raw := range exprs {
		expr, err = parser.ParseExpr(raw) // @must -ret(nil, fmt.Errorf("invalid return expression %q: %w", raw, err))
		results = append(results, expr)
	}
	return &ast.ReturnStmt{Results: results}, nil
}

func uniqStrings(items []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

// contentHash returns a hex-encoded SHA-256 hash of the content.
func contentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// readLines reads a file and returns its lines (without newlines).
func readLines(path string) ([]string, error) {
	// @require len(path) > 0, "path must not be empty"
	file, _ := os.Open(path) // @must -ret
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// injectLineDirectives compares the shadow output with the original source lines
// and inserts `//line` directives after injected blocks to restore correct line mapping.
//
// Strategy: walk shadow lines and original lines together. When a shadow line matches
// the next expected original line, they are "in sync". When shadow lines don't match
// (i.e. they are injected code), we let them pass. Once we re-sync, we emit a
// `//line original.go:N` directive to snap the compiler's line counter back.
func injectLineDirectives(shadow string, origLines []string, absPath string) string {
	// @require len(absPath) > 0, "absPath must not be empty"
	shadowLines := strings.Split(shadow, "\n")

	origIdx := 0 // pointer into original lines
	var result []string
	needsLineDirective := false

	for _, sLine := range shadowLines {
		trimmed := strings.TrimSpace(sLine)

		// Try to match against the current original line
		if origIdx < len(origLines) {
			origTrimmed := strings.TrimSpace(origLines[origIdx])

			if trimmed == origTrimmed {
				// Lines match — we are in sync
				if needsLineDirective {
					// Emit //line to snap back to the correct original line number
					// (origIdx is 0-based, line numbers are 1-based)
					result = append(result, fmt.Sprintf("//line %s:%d", absPath, origIdx+1))
					needsLineDirective = false
				}
				result = append(result, sLine)
				origIdx++
				continue
			}

			// Skip consecutive contract comment lines in the original source.
			// These were stripped from the AST and replaced with injected code.
			skipped := false
			for origIdx < len(origLines) && isContractComment(strings.TrimSpace(origLines[origIdx])) {
				origIdx++
				skipped = true
			}
			if skipped && origIdx < len(origLines) {
				origTrimmed = strings.TrimSpace(origLines[origIdx])
				if trimmed == origTrimmed {
					if needsLineDirective {
						result = append(result, fmt.Sprintf("//line %s:%d", absPath, origIdx+1))
						needsLineDirective = false
					}
					result = append(result, sLine)
					origIdx++
					continue
				}
			}
		}

		// This shadow line is injected code (no match in original)
		result = append(result, sLine)
		needsLineDirective = true
	}

	return strings.Join(result, "\n")
}

// isContractComment checks if a line is an inco contract comment that was stripped.
func isContractComment(line string) bool {
	s := strings.TrimSpace(line)
	if !strings.HasPrefix(s, "//") {
		return false
	}
	s = strings.TrimSpace(s[2:])
	return strings.HasPrefix(s, "@require") ||
		strings.HasPrefix(s, "@must")
}
