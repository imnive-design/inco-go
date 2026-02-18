package inco

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

// TypeResolver provides semantic type information for generating type-aware
// contract assertions. It wraps the output of go/types' Check method.
type TypeResolver struct {
	Info *types.Info
	Fset *token.FileSet
	Pkg  *types.Package
}

// ResolveVarType finds the type of a variable by name within the scope of
// the given function signature (parameters and named return values).
// Returns nil if the variable cannot be resolved.
func (tr *TypeResolver) ResolveVarType(funcType *ast.FuncType, varName string) types.Type {
	// @require -nd tr
	// @require tr.Info != nil, "TypeResolver.Info must be initialized"
	// @require -ret -nd funcType
	if funcType == nil {
		return nil
	}

	// Search parameters
	if funcType.Params != nil {
		for _, field := range funcType.Params.List {
			for _, name := range field.Names {
				if name.Name == varName {
					if obj := tr.Info.ObjectOf(name); obj != nil {
						return obj.Type()
					}
				}
			}
		}
	}

	// Search named return values
	if funcType.Results != nil {
		for _, field := range funcType.Results.List {
			for _, name := range field.Names {
				if name.Name == varName {
					if obj := tr.Info.ObjectOf(name); obj != nil {
						return obj.Type()
					}
				}
			}
		}
	}

	// Fallback: scope hierarchy lookup
	if scope, ok := tr.Info.Scopes[funcType]; ok {
		if _, obj := scope.LookupParent(varName, token.NoPos); obj != nil {
			return obj.Type()
		}
	}

	return nil
}

// EvalRequireExpr attempts to statically evaluate a contract expression using
// the type checker's constant folding. If the expression is a compile-time
// constant that evaluates to false, it returns a warning string; otherwise "".
func (tr *TypeResolver) EvalRequireExpr(pos token.Pos, expr string) string {
	// @require -nd tr
	// @require tr.Info != nil && tr.Pkg != nil, "TypeResolver must be fully initialized"
	tv, err := types.Eval(tr.Fset, tr.Pkg, pos, expr)
	if err != nil {
		return ""
	}
	if tv.Value != nil && tv.Value.String() == "false" {
		return fmt.Sprintf("expression %q is always false (compile-time constant)", expr)
	}
	return ""
}

// ZeroCheckExpr generates the AST expression that evaluates to true when the
// variable IS at its zero value, based on its resolved type.
//
// Type → Generated condition:
//
//	pointer/slice/map/chan/func/interface → var == nil
//	string                               → var == ""
//	integer                              → var == 0
//	float                                → var == 0.0
//	complex                              → var == 0
//	bool                                 → !var
//	comparable struct                    → var == (T{})
//	comparable array                     → var == ([N]T{})
//	unknown / non-comparable             → var == nil  (fallback)
func ZeroCheckExpr(varName string, typ types.Type, currentPkg *types.Package) ast.Expr {
	// @require len(varName) > 0, "varName must not be empty"

	// Guard: nil typ → fallback to nil check (also enforced by inco directive below)
	// @require -ret(nilCheckExpr(varName)) -nd typ
	if typ == nil {
		return nilCheckExpr(varName)
	}

	// --- Handle generic type parameters ---
	if tp, ok := typ.(*types.TypeParam); ok {
		return typeParamZeroCheckExpr(varName, tp)
	}

	underlying := typ.Underlying()

	switch t := underlying.(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature:
		return nilCheckExpr(varName)

	case *types.Interface:
		return nilCheckExpr(varName)

	case *types.Basic:
		info := t.Info()
		switch {
		case info&types.IsString != 0:
			return &ast.BinaryExpr{
				X:  ast.NewIdent(varName),
				Op: token.EQL,
				Y:  &ast.BasicLit{Kind: token.STRING, Value: `""`},
			}
		case info&types.IsInteger != 0:
			return &ast.BinaryExpr{
				X:  ast.NewIdent(varName),
				Op: token.EQL,
				Y:  &ast.BasicLit{Kind: token.INT, Value: "0"},
			}
		case info&types.IsFloat != 0:
			return &ast.BinaryExpr{
				X:  ast.NewIdent(varName),
				Op: token.EQL,
				Y:  &ast.BasicLit{Kind: token.FLOAT, Value: "0.0"},
			}
		case info&types.IsComplex != 0:
			return &ast.BinaryExpr{
				X:  ast.NewIdent(varName),
				Op: token.EQL,
				Y:  &ast.BasicLit{Kind: token.INT, Value: "0"},
			}
		case info&types.IsBoolean != 0:
			return &ast.UnaryExpr{
				Op: token.NOT,
				X:  ast.NewIdent(varName),
			}
		}

	case *types.Struct:
		if types.Comparable(typ) {
			return &ast.BinaryExpr{
				X:  ast.NewIdent(varName),
				Op: token.EQL,
				Y: &ast.ParenExpr{X: &ast.CompositeLit{
					Type: typeToASTExpr(typ, currentPkg),
				}},
			}
		}

	case *types.Array:
		if types.Comparable(typ) {
			return &ast.BinaryExpr{
				X:  ast.NewIdent(varName),
				Op: token.EQL,
				Y: &ast.ParenExpr{X: &ast.CompositeLit{
					Type: typeToASTExpr(typ, currentPkg),
				}},
			}
		}

	default:
		_ = t
	}

	// Fallback
	return nilCheckExpr(varName)
}

// typeParamZeroCheckExpr generates a zero-value check for a generic type parameter.
//
// For comparable type params:  v == *new(T)
// For non-comparable (any):    reflect.ValueOf(&v).Elem().IsZero()
func typeParamZeroCheckExpr(varName string, tp *types.TypeParam) ast.Expr {
	// @require len(varName) > 0, "varName must not be empty"
	// @require -nd tp
	if isTypeParamComparable(tp) {
		// Generate: v == *new(T)
		// *new(T) produces the zero value of any comparable type parameter
		return &ast.BinaryExpr{
			X:  ast.NewIdent(varName),
			Op: token.EQL,
			Y: &ast.StarExpr{
				X: &ast.CallExpr{
					Fun:  ast.NewIdent("new"),
					Args: []ast.Expr{ast.NewIdent(tp.Obj().Name())},
				},
			},
		}
	}

	// Non-comparable type param: use reflect.ValueOf(&v).Elem().IsZero()
	// reflect.ValueOf(&v).Elem().IsZero()
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X: &ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent("reflect"),
							Sel: ast.NewIdent("ValueOf"),
						},
						Args: []ast.Expr{
							&ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(varName)},
						},
					},
					Sel: ast.NewIdent("Elem"),
				},
			},
			Sel: ast.NewIdent("IsZero"),
		},
	}
}

// isTypeParamComparable returns true if a type parameter's constraint
// implies comparability (either explicitly via "comparable" or via a
// type set of all comparable types).
func isTypeParamComparable(tp *types.TypeParam) bool {
	// @require -nd tp
	return tp.Constraint() != nil && types.Comparable(tp)
}

// nilCheckExpr builds: varName == nil
func nilCheckExpr(varName string) ast.Expr {
	// @require len(varName) > 0, "varName must not be empty"
	return &ast.BinaryExpr{
		X:  ast.NewIdent(varName),
		Op: token.EQL,
		Y:  ast.NewIdent("nil"),
	}
}

// ZeroValueDesc returns a human-readable description of the zero value for use
// in panic messages.
func ZeroValueDesc(typ types.Type) string {
	// @require -ret("nil") -nd typ
	if typ == nil {
		return "nil"
	}
	// Handle generic type parameters
	if tp, ok := typ.(*types.TypeParam); ok {
		if isTypeParamComparable(tp) {
			return fmt.Sprintf("zero value of type param %s", tp.Obj().Name())
		}
		return fmt.Sprintf("zero value of type param %s (reflect)", tp.Obj().Name())
	}
	underlying := typ.Underlying()
	switch t := underlying.(type) {
	case *types.Pointer, *types.Interface, *types.Slice, *types.Map, *types.Chan, *types.Signature:
		return "nil"
	case *types.Basic:
		info := t.Info()
		switch {
		case info&types.IsString != 0:
			return "empty string"
		case info&types.IsNumeric != 0:
			return "zero"
		case info&types.IsBoolean != 0:
			return "false"
		}
	case *types.Struct:
		return "zero-valued struct"
	case *types.Array:
		return "zero-valued array"
	}
	return "zero-valued"
}

// typeToASTExpr converts a types.Type to an ast.Expr suitable for use in
// generated Go source code (e.g., composite literal types).
//
// For named types from other packages, it produces a SelectorExpr (pkg.Name).
// For same-package or builtin types, it produces an Ident.
func typeToASTExpr(typ types.Type, currentPkg *types.Package) ast.Expr {
	// @require -nd typ
	switch t := typ.(type) {
	case *types.Named:
		obj := t.Obj()
		pkg := obj.Pkg()
		if pkg == nil || currentPkg == nil || pkg == currentPkg {
			return ast.NewIdent(obj.Name())
		}
		// Cross-package type: pkg.Name
		return &ast.SelectorExpr{
			X:   ast.NewIdent(pkg.Name()),
			Sel: ast.NewIdent(obj.Name()),
		}

	case *types.TypeParam:
		return ast.NewIdent(t.Obj().Name())

	case *types.Basic:
		return ast.NewIdent(t.Name())

	case *types.Pointer:
		return &ast.StarExpr{X: typeToASTExpr(t.Elem(), currentPkg)}

	case *types.Slice:
		return &ast.ArrayType{Elt: typeToASTExpr(t.Elem(), currentPkg)}

	case *types.Array:
		return &ast.ArrayType{
			Len: &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", t.Len())},
			Elt: typeToASTExpr(t.Elem(), currentPkg),
		}

	case *types.Map:
		return &ast.MapType{
			Key:   typeToASTExpr(t.Key(), currentPkg),
			Value: typeToASTExpr(t.Elem(), currentPkg),
		}

	default:
		return ast.NewIdent(types.TypeString(typ, nil))
	}
}

// findEnclosingFuncType returns the *ast.FuncType of the innermost enclosing
// function (FuncDecl or FuncLit) that contains the given position.
func findEnclosingFuncType(f *ast.File, pos token.Pos) *ast.FuncType {
	// @require -nd f
	var result *ast.FuncType
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		// Skip subtrees that don't contain pos
		if pos < n.Pos() || pos > n.End() {
			return false
		}
		switch node := n.(type) {
		case *ast.FuncDecl:
			result = node.Type
		case *ast.FuncLit:
			result = node.Type
		}
		return true
	})
	return result
}

// NeedsImport returns the import path that must be present for the generated
// code to reference a type. Returns "" if no import is needed.
//
// Only struct and array types generate composite literals (T{}) that reference
// the type name; other zero-value checks (== nil, == 0, etc.) don't need imports.
func NeedsImport(typ types.Type, currentPkg *types.Package) string {
	// @require -ret -nd typ

	// Generic type parameters: non-comparable ones need "reflect"
	if tp, ok := typ.(*types.TypeParam); ok {
		if !isTypeParamComparable(tp) {
			return "reflect"
		}
		return ""
	}

	if currentPkg == nil {
		return ""
	}

	// Only composite-literal checks (struct/array) reference the type name
	underlying := typ.Underlying()
	switch underlying.(type) {
	case *types.Struct, *types.Array:
		if !types.Comparable(typ) {
			return ""
		}
	default:
		return ""
	}

	named, ok := typ.(*types.Named)
	if !ok {
		return ""
	}
	pkg := named.Obj().Pkg()
	if pkg == nil || pkg == currentPkg {
		return ""
	}
	return pkg.Path()
}

// IsErrorType checks if the type implements the builtin error interface.
func IsErrorType(t types.Type) bool {
	// @require -ret -nd t
	errType := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	return types.Implements(t, errType)
}

// ZeroValueLiteral generates an AST expression representing the zero value
// of the given type, suitable for use in return statements.
//
// Type → Generated literal:
//
//	pointer/slice/map/chan/func/interface → nil
//	string                               → ""
//	integer                              → 0
//	float                                → 0
//	complex                              → 0
//	bool                                 → false
//	struct                               → T{}
//	array                                → [N]T{}
//	type param                           → *new(T)
func ZeroValueLiteral(typ types.Type, currentPkg *types.Package) ast.Expr {
	// @require -ret(ast.NewIdent("nil")) -nd typ

	// Handle generic type parameters
	if tp, ok := typ.(*types.TypeParam); ok {
		return &ast.StarExpr{
			X: &ast.CallExpr{
				Fun:  ast.NewIdent("new"),
				Args: []ast.Expr{ast.NewIdent(tp.Obj().Name())},
			},
		}
	}

	underlying := typ.Underlying()

	switch t := underlying.(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature, *types.Interface:
		return ast.NewIdent("nil")

	case *types.Basic:
		info := t.Info()
		switch {
		case info&types.IsString != 0:
			return &ast.BasicLit{Kind: token.STRING, Value: `""`}
		case info&types.IsInteger != 0:
			return &ast.BasicLit{Kind: token.INT, Value: "0"}
		case info&types.IsFloat != 0:
			return &ast.BasicLit{Kind: token.FLOAT, Value: "0"}
		case info&types.IsBoolean != 0:
			return ast.NewIdent("false")
		case info&types.IsComplex != 0:
			return &ast.BasicLit{Kind: token.INT, Value: "0"}
		}

	case *types.Struct:
		return &ast.CompositeLit{Type: typeToASTExpr(typ, currentPkg)}

	case *types.Array:
		return &ast.CompositeLit{Type: typeToASTExpr(typ, currentPkg)}

	default:
		_ = t
	}

	return ast.NewIdent("nil")
}

// zeroValueFromASTType infers a zero-value expression from the AST type node
// when full type resolution is unavailable.
func zeroValueFromASTType(typeExpr ast.Expr) ast.Expr {
	switch t := typeExpr.(type) {
	case *ast.StarExpr:
		return ast.NewIdent("nil")
	case *ast.ArrayType:
		if t.Len == nil {
			return ast.NewIdent("nil") // slice
		}
		return &ast.CompositeLit{Type: t}
	case *ast.MapType:
		return ast.NewIdent("nil")
	case *ast.ChanType:
		return ast.NewIdent("nil")
	case *ast.InterfaceType:
		return ast.NewIdent("nil")
	case *ast.FuncType:
		return ast.NewIdent("nil")
	case *ast.Ident:
		switch t.Name {
		case "string":
			return &ast.BasicLit{Kind: token.STRING, Value: `""`}
		case "bool":
			return ast.NewIdent("false")
		case "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64",
			"byte", "rune", "uintptr":
			return &ast.BasicLit{Kind: token.INT, Value: "0"}
		case "float32", "float64":
			return &ast.BasicLit{Kind: token.FLOAT, Value: "0"}
		case "complex64", "complex128":
			return &ast.BasicLit{Kind: token.INT, Value: "0"}
		case "error":
			return ast.NewIdent("nil")
		}
	case *ast.SelectorExpr:
		return ast.NewIdent("nil")
	}
	return ast.NewIdent("nil")
}
