package inco

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

// exprToString prints an ast.Expr to a string for assertion comparisons.
func exprToString(expr ast.Expr) string {
	var buf strings.Builder
	fset := token.NewFileSet()
	printer.Fprint(&buf, fset, expr)
	return buf.String()
}

// --- ZeroCheckExpr: concrete types ---

func TestZeroCheckExpr_Nil(t *testing.T) {
	expr := ZeroCheckExpr("x", nil, nil)
	got := exprToString(expr)
	if got != "x == nil" {
		t.Errorf("nil type → %q, want %q", got, "x == nil")
	}
}

func TestZeroCheckExpr_Pointer(t *testing.T) {
	typ := types.NewPointer(types.Typ[types.Int])
	expr := ZeroCheckExpr("p", typ, nil)
	got := exprToString(expr)
	if got != "p == nil" {
		t.Errorf("pointer → %q, want %q", got, "p == nil")
	}
}

func TestZeroCheckExpr_Slice(t *testing.T) {
	typ := types.NewSlice(types.Typ[types.Byte])
	expr := ZeroCheckExpr("s", typ, nil)
	got := exprToString(expr)
	if got != "s == nil" {
		t.Errorf("slice → %q, want %q", got, "s == nil")
	}
}

func TestZeroCheckExpr_Map(t *testing.T) {
	typ := types.NewMap(types.Typ[types.String], types.Typ[types.Int])
	expr := ZeroCheckExpr("m", typ, nil)
	got := exprToString(expr)
	if got != "m == nil" {
		t.Errorf("map → %q, want %q", got, "m == nil")
	}
}

func TestZeroCheckExpr_Chan(t *testing.T) {
	typ := types.NewChan(types.SendRecv, types.Typ[types.Int])
	expr := ZeroCheckExpr("ch", typ, nil)
	got := exprToString(expr)
	if got != "ch == nil" {
		t.Errorf("chan → %q, want %q", got, "ch == nil")
	}
}

func TestZeroCheckExpr_Interface(t *testing.T) {
	typ := types.NewInterfaceType(nil, nil)
	typ.Complete()
	expr := ZeroCheckExpr("iface", typ, nil)
	got := exprToString(expr)
	if got != "iface == nil" {
		t.Errorf("interface → %q, want %q", got, "iface == nil")
	}
}

func TestZeroCheckExpr_String(t *testing.T) {
	typ := types.Typ[types.String]
	expr := ZeroCheckExpr("name", typ, nil)
	got := exprToString(expr)
	if got != `name == ""` {
		t.Errorf("string → %q, want %q", got, `name == ""`)
	}
}

func TestZeroCheckExpr_Int(t *testing.T) {
	typ := types.Typ[types.Int]
	expr := ZeroCheckExpr("n", typ, nil)
	got := exprToString(expr)
	if got != "n == 0" {
		t.Errorf("int → %q, want %q", got, "n == 0")
	}
}

func TestZeroCheckExpr_Float64(t *testing.T) {
	typ := types.Typ[types.Float64]
	expr := ZeroCheckExpr("f", typ, nil)
	got := exprToString(expr)
	if got != "f == 0.0" {
		t.Errorf("float64 → %q, want %q", got, "f == 0.0")
	}
}

func TestZeroCheckExpr_Complex128(t *testing.T) {
	typ := types.Typ[types.Complex128]
	expr := ZeroCheckExpr("c", typ, nil)
	got := exprToString(expr)
	if got != "c == 0" {
		t.Errorf("complex128 → %q, want %q", got, "c == 0")
	}
}

func TestZeroCheckExpr_Bool(t *testing.T) {
	typ := types.Typ[types.Bool]
	expr := ZeroCheckExpr("ok", typ, nil)
	got := exprToString(expr)
	if got != "!ok" {
		t.Errorf("bool → %q, want %q", got, "!ok")
	}
}

func TestZeroCheckExpr_Func(t *testing.T) {
	sig := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	expr := ZeroCheckExpr("fn", sig, nil)
	got := exprToString(expr)
	if got != "fn == nil" {
		t.Errorf("func → %q, want %q", got, "fn == nil")
	}
}

// --- ZeroCheckExpr: generics ---

func TestZeroCheckExpr_ComparableTypeParam(t *testing.T) {
	// Build a comparable type parameter T
	tp := makeTypeParam("T", true)
	expr := ZeroCheckExpr("v", tp, nil)
	got := exprToString(expr)
	if got != "v == *new(T)" {
		t.Errorf("comparable TypeParam → %q, want %q", got, "v == *new(T)")
	}
}

func TestZeroCheckExpr_AnyTypeParam(t *testing.T) {
	// Build a non-comparable type parameter T (any)
	tp := makeTypeParam("T", false)
	expr := ZeroCheckExpr("v", tp, nil)
	got := exprToString(expr)
	if !strings.Contains(got, "reflect") || !strings.Contains(got, "IsZero") {
		t.Errorf("any TypeParam → %q, want reflect-based IsZero check", got)
	}
}

// --- ZeroValueDesc ---

func TestZeroValueDesc(t *testing.T) {
	tests := []struct {
		typ  types.Type
		want string
	}{
		{nil, "nil"},
		{types.Typ[types.String], "empty string"},
		{types.Typ[types.Int], "zero"},
		{types.Typ[types.Float64], "zero"},
		{types.Typ[types.Bool], "false"},
		{types.NewPointer(types.Typ[types.Int]), "nil"},
		{types.NewSlice(types.Typ[types.Int]), "nil"},
		{types.NewMap(types.Typ[types.String], types.Typ[types.Int]), "nil"},
	}
	for _, tt := range tests {
		got := ZeroValueDesc(tt.typ)
		if got != tt.want {
			name := "nil"
			if tt.typ != nil {
				name = tt.typ.String()
			}
			t.Errorf("ZeroValueDesc(%s) = %q, want %q", name, got, tt.want)
		}
	}
}

func TestZeroValueDesc_TypeParam(t *testing.T) {
	tp := makeTypeParam("T", true)
	got := ZeroValueDesc(tp)
	if !strings.Contains(got, "type param T") {
		t.Errorf("ZeroValueDesc(comparable TypeParam) = %q, want to contain 'type param T'", got)
	}

	tp2 := makeTypeParam("V", false)
	got2 := ZeroValueDesc(tp2)
	if !strings.Contains(got2, "reflect") {
		t.Errorf("ZeroValueDesc(any TypeParam) = %q, want to contain 'reflect'", got2)
	}
}

// --- NeedsImport ---

func TestNeedsImport_BasicTypes(t *testing.T) {
	for _, typ := range []types.Type{
		nil,
		types.Typ[types.Int],
		types.Typ[types.String],
		types.Typ[types.Bool],
		types.NewPointer(types.Typ[types.Int]),
		types.NewSlice(types.Typ[types.Int]),
	} {
		got := NeedsImport(typ, nil)
		if got != "" {
			t.Errorf("NeedsImport(%v) = %q, want empty", typ, got)
		}
	}
}

func TestNeedsImport_AnyTypeParam(t *testing.T) {
	tp := makeTypeParam("T", false)
	got := NeedsImport(tp, nil)
	if got != "reflect" {
		t.Errorf("NeedsImport(any TypeParam) = %q, want %q", got, "reflect")
	}
}

func TestNeedsImport_ComparableTypeParam(t *testing.T) {
	tp := makeTypeParam("T", true)
	got := NeedsImport(tp, nil)
	if got != "" {
		t.Errorf("NeedsImport(comparable TypeParam) = %q, want empty", got)
	}
}

// --- typeToASTExpr ---

func TestTypeToASTExpr_Basic(t *testing.T) {
	expr := typeToASTExpr(types.Typ[types.Int], nil)
	got := exprToString(expr)
	if got != "int" {
		t.Errorf("typeToASTExpr(int) = %q, want %q", got, "int")
	}
}

func TestTypeToASTExpr_Pointer(t *testing.T) {
	typ := types.NewPointer(types.Typ[types.String])
	expr := typeToASTExpr(typ, nil)
	got := exprToString(expr)
	if got != "*string" {
		t.Errorf("typeToASTExpr(*string) = %q, want %q", got, "*string")
	}
}

func TestTypeToASTExpr_Slice(t *testing.T) {
	typ := types.NewSlice(types.Typ[types.Byte])
	expr := typeToASTExpr(typ, nil)
	got := exprToString(expr)
	// types.Typ[types.Byte] is uint8 under the hood
	if got != "[]uint8" {
		t.Errorf("typeToASTExpr([]byte) = %q, want %q", got, "[]uint8")
	}
}

func TestTypeToASTExpr_Array(t *testing.T) {
	typ := types.NewArray(types.Typ[types.Int], 5)
	expr := typeToASTExpr(typ, nil)
	got := exprToString(expr)
	if got != "[5]int" {
		t.Errorf("typeToASTExpr([5]int) = %q, want %q", got, "[5]int")
	}
}

func TestTypeToASTExpr_Map(t *testing.T) {
	typ := types.NewMap(types.Typ[types.String], types.Typ[types.Int])
	expr := typeToASTExpr(typ, nil)
	got := exprToString(expr)
	if got != "map[string]int" {
		t.Errorf("typeToASTExpr(map[string]int) = %q, want %q", got, "map[string]int")
	}
}

func TestTypeToASTExpr_TypeParam(t *testing.T) {
	tp := makeTypeParam("T", true)
	expr := typeToASTExpr(tp, nil)
	got := exprToString(expr)
	if got != "T" {
		t.Errorf("typeToASTExpr(TypeParam T) = %q, want %q", got, "T")
	}
}

// --- findEnclosingFuncType ---

func TestFindEnclosingFuncType(t *testing.T) {
	src := `package test
func Foo(x int) {
	_ = x
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	// Position inside the function body (the _ = x statement)
	var stmtPos token.Pos
	ast.Inspect(f, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok {
			stmtPos = assign.Pos()
			return false
		}
		return true
	})

	ft := findEnclosingFuncType(f, stmtPos)
	if ft == nil {
		t.Fatal("findEnclosingFuncType returned nil")
	}
	if ft.Params == nil || len(ft.Params.List) != 1 {
		t.Fatalf("expected 1 param, got %v", ft.Params)
	}
}

func TestFindEnclosingFuncType_Closure(t *testing.T) {
	src := `package test
func Outer() {
	f := func(y string) {
		_ = y
	}
	_ = f
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	// Find the _ = y statement inside the closure
	var innerPos token.Pos
	ast.Inspect(f, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok {
			if ident, ok := assign.Rhs[0].(*ast.Ident); ok && ident.Name == "y" {
				innerPos = assign.Pos()
				return false
			}
		}
		return true
	})

	ft := findEnclosingFuncType(f, innerPos)
	if ft == nil {
		t.Fatal("findEnclosingFuncType returned nil for closure")
	}
	// The closure has `y string` as param
	if ft.Params == nil || len(ft.Params.List) != 1 {
		t.Fatalf("expected 1 param for closure, got %v", ft.Params)
	}
	name := ft.Params.List[0].Names[0].Name
	if name != "y" {
		t.Errorf("closure param = %q, want %q", name, "y")
	}
}

// --- AST helpers (makeIfPanicStmt, makeDeferStmt) ---

func TestMakeIfPanicStmt(t *testing.T) {
	cond := &ast.BinaryExpr{
		X:  ast.NewIdent("x"),
		Op: token.EQL,
		Y:  ast.NewIdent("nil"),
	}
	stmt := makeIfPanicStmt(cond, "test message")

	if stmt.Cond == nil {
		t.Fatal("Cond is nil")
	}
	if len(stmt.Body.List) != 1 {
		t.Fatalf("Body has %d stmts, want 1", len(stmt.Body.List))
	}
	exprStmt, ok := stmt.Body.List[0].(*ast.ExprStmt)
	if !ok {
		t.Fatal("body stmt is not ExprStmt")
	}
	call, ok := exprStmt.X.(*ast.CallExpr)
	if !ok {
		t.Fatal("expr is not CallExpr")
	}
	if ident, ok := call.Fun.(*ast.Ident); !ok || ident.Name != "panic" {
		t.Error("function is not panic")
	}
}

func TestMakeIfPanicErrStmt(t *testing.T) {
	stmt := makeIfPanicErrStmt("err", "something failed")
	bin, ok := stmt.Cond.(*ast.BinaryExpr)
	if !ok {
		t.Fatal("Cond is not BinaryExpr")
	}
	if bin.Op != token.NEQ {
		t.Errorf("Op = %v, want NEQ", bin.Op)
	}
	x, ok := bin.X.(*ast.Ident)
	if !ok || x.Name != "err" {
		t.Errorf("X = %v, want ident 'err'", bin.X)
	}
}

// --- uniqStrings ---

func TestUniqStrings(t *testing.T) {
	tests := []struct {
		input []string
		want  []string
	}{
		{nil, nil},
		{[]string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
		{[]string{"x"}, []string{"x"}},
		{[]string{"reflect", "reflect", "fmt"}, []string{"reflect", "fmt"}},
	}
	for _, tt := range tests {
		got := uniqStrings(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("uniqStrings(%v) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("uniqStrings(%v)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

// --- isContractComment ---

func TestIsContractComment(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"// @require -nd x", true},
		{"  // @require -ret -nd x", true},
		{"// @must", true},
		{"// @ensure -nd result", false}, // @ensure removed
		{"// regular comment", false},
		{"not a comment", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isContractComment(tt.line)
		if got != tt.want {
			t.Errorf("isContractComment(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

// --- helpers ---

// makeTypeParam creates a synthetic types.TypeParam for testing.
// If comparable is true, the constraint is the built-in `comparable`;
// otherwise, it is `any` (empty interface).
func makeTypeParam(name string, comparable bool) *types.TypeParam {
	var constraint *types.Interface
	if comparable {
		// comparable is a built-in interface that marks types as comparable
		constraint = types.NewInterfaceType(nil, nil)
		constraint.MarkImplicit()
		constraint.Complete()
		// Use the universe's comparable type
		obj := types.Universe.Lookup("comparable")
		if obj != nil {
			if named, ok := obj.Type().(*types.Named); ok {
				if iface, ok := named.Underlying().(*types.Interface); ok {
					constraint = iface
				}
			}
		}
	} else {
		// any = empty interface
		constraint = types.NewInterfaceType(nil, nil)
		constraint.Complete()
	}
	tparam := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, name, nil), constraint)
	return tparam
}

// --- Additional typecheck tests ---

func TestZeroCheckExpr_ComparableStruct(t *testing.T) {
	// Create a named struct type with comparable fields
	pkg := types.NewPackage("test/pkg", "pkg")
	fields := []*types.Var{
		types.NewField(token.NoPos, pkg, "X", types.Typ[types.Int], false),
		types.NewField(token.NoPos, pkg, "Y", types.Typ[types.String], false),
	}
	structType := types.NewStruct(fields, nil)
	named := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Point", nil), structType, nil)

	expr := ZeroCheckExpr("p", named, pkg)
	got := exprToString(expr)
	if !strings.Contains(got, "p ==") || !strings.Contains(got, "Point{}") {
		t.Errorf("comparable struct → %q, want 'p == (Point{})' pattern", got)
	}
}

func TestZeroCheckExpr_ComparableArray(t *testing.T) {
	typ := types.NewArray(types.Typ[types.Int], 3)
	expr := ZeroCheckExpr("arr", typ, nil)
	got := exprToString(expr)
	if !strings.Contains(got, "arr ==") || !strings.Contains(got, "[3]int{}") {
		t.Errorf("comparable array → %q, want 'arr == ([3]int{})' pattern", got)
	}
}

func TestZeroValueDesc_Struct(t *testing.T) {
	fields := []*types.Var{
		types.NewField(token.NoPos, nil, "X", types.Typ[types.Int], false),
	}
	structType := types.NewStruct(fields, nil)
	got := ZeroValueDesc(structType)
	if got != "zero-valued struct" {
		t.Errorf("ZeroValueDesc(struct) = %q, want %q", got, "zero-valued struct")
	}
}

func TestZeroValueDesc_Array(t *testing.T) {
	typ := types.NewArray(types.Typ[types.Int], 5)
	got := ZeroValueDesc(typ)
	if got != "zero-valued array" {
		t.Errorf("ZeroValueDesc(array) = %q, want %q", got, "zero-valued array")
	}
}

func TestZeroValueDesc_Chan(t *testing.T) {
	typ := types.NewChan(types.SendRecv, types.Typ[types.Int])
	got := ZeroValueDesc(typ)
	if got != "nil" {
		t.Errorf("ZeroValueDesc(chan) = %q, want %q", got, "nil")
	}
}

func TestZeroValueDesc_Func(t *testing.T) {
	sig := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	got := ZeroValueDesc(sig)
	if got != "nil" {
		t.Errorf("ZeroValueDesc(func) = %q, want %q", got, "nil")
	}
}

func TestEvalRequireExpr_AlwaysFalse(t *testing.T) {
	fset := token.NewFileSet()
	src := `package test
const x = true
`
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:  make(map[ast.Expr]types.TypeAndValue),
		Defs:   make(map[*ast.Ident]types.Object),
		Uses:   make(map[*ast.Ident]types.Object),
		Scopes: make(map[ast.Node]*types.Scope),
	}
	conf := &types.Config{}
	pkg, err := conf.Check("test", fset, []*ast.File{f}, info)
	if err != nil {
		t.Fatal(err)
	}
	tr := &TypeResolver{Info: info, Fset: fset, Pkg: pkg}

	// 1 > 2 is always false
	warn := tr.EvalRequireExpr(f.Pos(), "1 > 2")
	if warn == "" {
		t.Error("expected warning for always-false expression '1 > 2'")
	}
	if !strings.Contains(warn, "always false") {
		t.Errorf("warning = %q, want to contain 'always false'", warn)
	}
}

func TestEvalRequireExpr_AlwaysTrue(t *testing.T) {
	fset := token.NewFileSet()
	src := `package test
`
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:  make(map[ast.Expr]types.TypeAndValue),
		Defs:   make(map[*ast.Ident]types.Object),
		Uses:   make(map[*ast.Ident]types.Object),
		Scopes: make(map[ast.Node]*types.Scope),
	}
	conf := &types.Config{}
	pkg, err := conf.Check("test", fset, []*ast.File{f}, info)
	if err != nil {
		t.Fatal(err)
	}
	tr := &TypeResolver{Info: info, Fset: fset, Pkg: pkg}

	// 1 < 2 is always true — no warning
	warn := tr.EvalRequireExpr(f.Pos(), "1 < 2")
	if warn != "" {
		t.Errorf("unexpected warning for always-true expression: %q", warn)
	}
}

func TestEvalRequireExpr_NonConstant(t *testing.T) {
	fset := token.NewFileSet()
	src := `package test
`
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:  make(map[ast.Expr]types.TypeAndValue),
		Defs:   make(map[*ast.Ident]types.Object),
		Uses:   make(map[*ast.Ident]types.Object),
		Scopes: make(map[ast.Node]*types.Scope),
	}
	conf := &types.Config{}
	pkg, err := conf.Check("test", fset, []*ast.File{f}, info)
	if err != nil {
		t.Fatal(err)
	}
	tr := &TypeResolver{Info: info, Fset: fset, Pkg: pkg}

	// Non-constant expression — should return no warning
	warn := tr.EvalRequireExpr(f.Pos(), "x > 0")
	if warn != "" {
		t.Errorf("unexpected warning for non-constant expression: %q", warn)
	}
}

func TestEvalRequireExpr_NilResolver(t *testing.T) {
	// EvalRequireExpr uses @require -nd tr as its nil guard.
	// Without the overlay (plain go test), calling on nil receiver panics.
	// This test verifies the contract violation is detected (panics).
	var tr *TypeResolver
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil TypeResolver, but did not panic")
		}
	}()
	_ = tr.EvalRequireExpr(token.NoPos, "1 > 2")
}

func TestResolveVarType_Param(t *testing.T) {
	fset := token.NewFileSet()
	src := `package test

func Foo(x int, y string) {
	_ = x
	_ = y
}
`
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:  make(map[ast.Expr]types.TypeAndValue),
		Defs:   make(map[*ast.Ident]types.Object),
		Uses:   make(map[*ast.Ident]types.Object),
		Scopes: make(map[ast.Node]*types.Scope),
	}
	conf := &types.Config{}
	pkg, err := conf.Check("test", fset, []*ast.File{f}, info)
	if err != nil {
		t.Fatal(err)
	}
	tr := &TypeResolver{Info: info, Fset: fset, Pkg: pkg}

	// Find the FuncDecl
	var funcType *ast.FuncType
	ast.Inspect(f, func(n ast.Node) bool {
		if fd, ok := n.(*ast.FuncDecl); ok && fd.Name.Name == "Foo" {
			funcType = fd.Type
			return false
		}
		return true
	})
	if funcType == nil {
		t.Fatal("FuncDecl not found")
	}

	xType := tr.ResolveVarType(funcType, "x")
	if xType == nil {
		t.Fatal("ResolveVarType('x') = nil")
	}
	if xType.String() != "int" {
		t.Errorf("x type = %q, want 'int'", xType.String())
	}

	yType := tr.ResolveVarType(funcType, "y")
	if yType == nil {
		t.Fatal("ResolveVarType('y') = nil")
	}
	if yType.String() != "string" {
		t.Errorf("y type = %q, want 'string'", yType.String())
	}

	// Non-existent variable
	zType := tr.ResolveVarType(funcType, "z")
	if zType != nil {
		t.Errorf("ResolveVarType('z') = %v, want nil", zType)
	}
}

func TestResolveVarType_NamedReturn(t *testing.T) {
	fset := token.NewFileSet()
	src := `package test

func Bar() (result int, err error) {
	return 0, nil
}
`
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:  make(map[ast.Expr]types.TypeAndValue),
		Defs:   make(map[*ast.Ident]types.Object),
		Uses:   make(map[*ast.Ident]types.Object),
		Scopes: make(map[ast.Node]*types.Scope),
	}
	conf := &types.Config{}
	pkg, err := conf.Check("test", fset, []*ast.File{f}, info)
	if err != nil {
		t.Fatal(err)
	}
	tr := &TypeResolver{Info: info, Fset: fset, Pkg: pkg}

	var funcType *ast.FuncType
	ast.Inspect(f, func(n ast.Node) bool {
		if fd, ok := n.(*ast.FuncDecl); ok && fd.Name.Name == "Bar" {
			funcType = fd.Type
			return false
		}
		return true
	})

	resultType := tr.ResolveVarType(funcType, "result")
	if resultType == nil || resultType.String() != "int" {
		t.Errorf("result type = %v, want int", resultType)
	}

	errType := tr.ResolveVarType(funcType, "err")
	if errType == nil || errType.String() != "error" {
		t.Errorf("err type = %v, want error", errType)
	}
}

func TestResolveVarType_NilFuncType(t *testing.T) {
	// With @require -ret -nd funcType, passing nil funcType returns nil (zero value).
	fset := token.NewFileSet()
	info := &types.Info{
		Types:  make(map[ast.Expr]types.TypeAndValue),
		Defs:   make(map[*ast.Ident]types.Object),
		Uses:   make(map[*ast.Ident]types.Object),
		Scopes: make(map[ast.Node]*types.Scope),
	}
	tr := &TypeResolver{Info: info, Fset: fset}
	result := tr.ResolveVarType(nil, "x")
	if result != nil {
		t.Errorf("nil funcType should return nil, got %v", result)
	}
}

func TestNeedsImport_CrossPkgStruct(t *testing.T) {
	// Simulate a named struct type from another package
	otherPkg := types.NewPackage("other/pkg", "other")
	fields := []*types.Var{
		types.NewField(token.NoPos, otherPkg, "V", types.Typ[types.Int], false),
	}
	structType := types.NewStruct(fields, nil)
	named := types.NewNamed(types.NewTypeName(token.NoPos, otherPkg, "Config", nil), structType, nil)

	currentPkg := types.NewPackage("my/pkg", "main")
	got := NeedsImport(named, currentPkg)
	if got != "other/pkg" {
		t.Errorf("NeedsImport(cross-pkg struct) = %q, want %q", got, "other/pkg")
	}
}

func TestNeedsImport_SamePkgStruct(t *testing.T) {
	pkg := types.NewPackage("my/pkg", "main")
	fields := []*types.Var{
		types.NewField(token.NoPos, pkg, "V", types.Typ[types.Int], false),
	}
	structType := types.NewStruct(fields, nil)
	named := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Config", nil), structType, nil)

	got := NeedsImport(named, pkg)
	if got != "" {
		t.Errorf("NeedsImport(same-pkg struct) = %q, want empty", got)
	}
}

func TestTypeToASTExpr_CrossPkgNamed(t *testing.T) {
	otherPkg := types.NewPackage("other/pkg", "other")
	named := types.NewNamed(types.NewTypeName(token.NoPos, otherPkg, "Config", nil), types.Typ[types.Int], nil)

	currentPkg := types.NewPackage("my/pkg", "main")
	expr := typeToASTExpr(named, currentPkg)
	got := exprToString(expr)
	if got != "other.Config" {
		t.Errorf("typeToASTExpr(cross-pkg named) = %q, want %q", got, "other.Config")
	}
}

func TestTypeToASTExpr_SamePkgNamed(t *testing.T) {
	pkg := types.NewPackage("my/pkg", "main")
	named := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Config", nil), types.Typ[types.Int], nil)

	expr := typeToASTExpr(named, pkg)
	got := exprToString(expr)
	if got != "Config" {
		t.Errorf("typeToASTExpr(same-pkg named) = %q, want %q", got, "Config")
	}
}
