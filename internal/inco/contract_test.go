package inco

import (
	"testing"
)

func TestParseDirective_RequireND(t *testing.T) {
	tests := []struct {
		input    string
		wantVars []string
	}{
		{"// @require -nd x", []string{"x"}},
		{"// @require -nd x, y", []string{"x", "y"}},
		{"// @require -nd   a ,  b , c ", []string{"a", "b", "c"}},
		{"  // @require -nd ptr  ", []string{"ptr"}},
	}
	for _, tt := range tests {
		d := ParseDirective(tt.input)
		if d == nil {
			t.Fatalf("ParseDirective(%q) = nil, want directive", tt.input)
		}
		if d.Kind != Require {
			t.Errorf("Kind = %v, want Require", d.Kind)
		}
		if !d.ND {
			t.Errorf("ND = false, want true")
		}
		if len(d.Vars) != len(tt.wantVars) {
			t.Errorf("Vars = %v, want %v", d.Vars, tt.wantVars)
			continue
		}
		for i, v := range d.Vars {
			if v != tt.wantVars[i] {
				t.Errorf("Vars[%d] = %q, want %q", i, v, tt.wantVars[i])
			}
		}
	}
}

func TestParseDirective_RequireExpr(t *testing.T) {
	tests := []struct {
		input   string
		expr    string
		message string
	}{
		{"// @require len(x) > 0", "len(x) > 0", ""},
		{`// @require age > 0, "age must be positive"`, "age > 0", "age must be positive"},
		{"// @require a > b", "a > b", ""},
		{`// @require x != nil, "x required"`, "x != nil", "x required"},
	}
	for _, tt := range tests {
		d := ParseDirective(tt.input)
		if d == nil {
			t.Fatalf("ParseDirective(%q) = nil", tt.input)
		}
		if d.Kind != Require {
			t.Errorf("Kind = %v, want Require", d.Kind)
		}
		if d.ND {
			t.Errorf("ND = true, want false")
		}
		if d.Expr != tt.expr {
			t.Errorf("Expr = %q, want %q", d.Expr, tt.expr)
		}
		if d.Message != tt.message {
			t.Errorf("Message = %q, want %q", d.Message, tt.message)
		}
	}
}

func TestParseDirective_EnsureIsNil(t *testing.T) {
	// @ensure was removed — parser should return nil
	d := ParseDirective("// @ensure -nd result")
	if d != nil {
		t.Errorf("ParseDirective(@ensure) should return nil, got %+v", d)
	}
}

func TestParseDirective_Must(t *testing.T) {
	for _, input := range []string{"// @must", "  // @must  ", "/* @must */"} {
		d := ParseDirective(input)
		if d == nil {
			t.Fatalf("ParseDirective(%q) = nil", input)
		}
		if d.Kind != Must {
			t.Errorf("Kind = %v, want Must for %q", d.Kind, input)
		}
	}
}

func TestParseDirective_BlockComment(t *testing.T) {
	d := ParseDirective("/* @require -nd db */")
	if d == nil {
		t.Fatal("ParseDirective returned nil for block comment")
	}
	if d.Kind != Require || !d.ND {
		t.Errorf("got Kind=%v ND=%v, want Require/true", d.Kind, d.ND)
	}
	if len(d.Vars) != 1 || d.Vars[0] != "db" {
		t.Errorf("Vars = %v, want [db]", d.Vars)
	}
}

func TestParseDirective_NotADirective(t *testing.T) {
	inputs := []string{
		"// regular comment",
		"// @unknown directive",
		"x + y",
		" ",
		"// just some text",
	}
	for _, input := range inputs {
		d := ParseDirective(input)
		if d != nil {
			t.Errorf("ParseDirective(%q) = %+v, want nil", input, d)
		}
	}
}

func TestParseDirective_NDNoVars(t *testing.T) {
	d := ParseDirective("// @require -nd")
	if d == nil {
		t.Fatal("ParseDirective returned nil")
	}
	if !d.ND {
		t.Error("ND = false, want true")
	}
	if len(d.Vars) != 0 {
		t.Errorf("Vars = %v, want empty", d.Vars)
	}
}

func TestParseDirective_EmptyRequire(t *testing.T) {
	d := ParseDirective("// @require")
	if d == nil {
		t.Fatal("ParseDirective returned nil")
	}
	if d.Kind != Require {
		t.Errorf("Kind = %v, want Require", d.Kind)
	}
	if d.ND || d.Expr != "" || len(d.Vars) != 0 {
		t.Errorf("unexpected fields: ND=%v Expr=%q Vars=%v", d.ND, d.Expr, d.Vars)
	}
}

func TestKindString(t *testing.T) {
	tests := []struct {
		k    Kind
		want string
	}{
		{Require, "require"},
		{Must, "must"},
		{Kind(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.k.String(); got != tt.want {
			t.Errorf("Kind(%d).String() = %q, want %q", tt.k, got, tt.want)
		}
	}
}

func TestParseDirective_ExprWithCommaNoMessage(t *testing.T) {
	d := ParseDirective("// @require f(a, b) > 0")
	if d == nil {
		t.Fatal("ParseDirective returned nil")
	}
	if d.Expr != "f(a, b) > 0" {
		t.Errorf("Expr = %q, want %q", d.Expr, "f(a, b) > 0")
	}
	if d.Message != "" {
		t.Errorf("Message = %q, want empty", d.Message)
	}
}

func TestParseDirective_ExprWithCommaAndMessage(t *testing.T) {
	d := ParseDirective(`// @require f(a, b) > 0, "must be positive"`)
	if d == nil {
		t.Fatal("ParseDirective returned nil")
	}
	if d.Expr != "f(a, b) > 0" {
		t.Errorf("Expr = %q, want %q", d.Expr, "f(a, b) > 0")
	}
	if d.Message != "must be positive" {
		t.Errorf("Message = %q, want %q", d.Message, "must be positive")
	}
}

// --- Tests for -ret and -log flags ---

func TestParseDirective_RequireRetND(t *testing.T) {
	tests := []struct {
		input    string
		wantVars []string
	}{
		{"// @require -ret -nd x", []string{"x"}},
		{"// @require -nd -ret x, y", []string{"x", "y"}},
		{"// @require -ret -nd a, b, c", []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		d := ParseDirective(tt.input)
		if d == nil {
			t.Fatalf("ParseDirective(%q) = nil, want directive", tt.input)
		}
		if d.Kind != Require {
			t.Errorf("Kind = %v, want Require", d.Kind)
		}
		if !d.Ret {
			t.Errorf("Ret = false, want true for %q", tt.input)
		}
		if !d.ND {
			t.Errorf("ND = false, want true for %q", tt.input)
		}
		if len(d.Vars) != len(tt.wantVars) {
			t.Errorf("Vars = %v, want %v for %q", d.Vars, tt.wantVars, tt.input)
			continue
		}
		for i, v := range d.Vars {
			if v != tt.wantVars[i] {
				t.Errorf("Vars[%d] = %q, want %q for %q", i, v, tt.wantVars[i], tt.input)
			}
		}
	}
}

func TestParseDirective_RequireRetExpr(t *testing.T) {
	tests := []struct {
		input   string
		expr    string
		message string
	}{
		{"// @require -ret len(x) > 0", "len(x) > 0", ""},
		{`// @require -ret age > 0, "age must be positive"`, "age > 0", "age must be positive"},
	}
	for _, tt := range tests {
		d := ParseDirective(tt.input)
		if d == nil {
			t.Fatalf("ParseDirective(%q) = nil", tt.input)
		}
		if !d.Ret {
			t.Error("Ret = false, want true")
		}
		if d.Log {
			t.Error("Log = true, want false")
		}
		if d.Expr != tt.expr {
			t.Errorf("Expr = %q, want %q", d.Expr, tt.expr)
		}
		if d.Message != tt.message {
			t.Errorf("Message = %q, want %q", d.Message, tt.message)
		}
	}
}

func TestParseDirective_RequireLogND(t *testing.T) {
	d := ParseDirective("// @require -log -nd x, y")
	if d == nil {
		t.Fatal("ParseDirective returned nil")
	}
	if !d.Log {
		t.Error("Log = false, want true")
	}
	if !d.Ret {
		t.Error("Ret = false, want true (-log implies -ret)")
	}
	if !d.ND {
		t.Error("ND = false, want true")
	}
	if len(d.Vars) != 2 || d.Vars[0] != "x" || d.Vars[1] != "y" {
		t.Errorf("Vars = %v, want [x y]", d.Vars)
	}
}

func TestParseDirective_RequireLogExpr(t *testing.T) {
	d := ParseDirective(`// @require -log amount > 0, "amount must be positive"`)
	if d == nil {
		t.Fatal("ParseDirective returned nil")
	}
	if !d.Log {
		t.Error("Log = false, want true")
	}
	if !d.Ret {
		t.Error("Ret = false, want true (-log implies -ret)")
	}
	if d.Expr != "amount > 0" {
		t.Errorf("Expr = %q, want %q", d.Expr, "amount > 0")
	}
	if d.Message != "amount must be positive" {
		t.Errorf("Message = %q, want %q", d.Message, "amount must be positive")
	}
}

func TestParseDirective_AllFlagsCombined(t *testing.T) {
	d := ParseDirective("// @require -ret -log -nd x")
	if d == nil {
		t.Fatal("ParseDirective returned nil")
	}
	if !d.Ret || !d.Log || !d.ND {
		t.Errorf("flags: Ret=%v Log=%v ND=%v, want all true", d.Ret, d.Log, d.ND)
	}
	if len(d.Vars) != 1 || d.Vars[0] != "x" {
		t.Errorf("Vars = %v, want [x]", d.Vars)
	}
}

func TestParseDirective_EnsureRetIsNil(t *testing.T) {
	// @ensure was removed — parser should return nil
	d := ParseDirective("// @ensure -ret -nd result")
	if d != nil {
		t.Errorf("ParseDirective(@ensure -ret) should return nil, got %+v", d)
	}
}

// --- Tests for -ret(expr, ...) custom return expressions ---

func TestParseDirective_RetExprs_Simple(t *testing.T) {
	d := ParseDirective(`// @require -ret(nil, ErrNotFound) -nd db`)
	if d == nil {
		t.Fatal("ParseDirective returned nil")
	}
	if !d.Ret {
		t.Error("Ret = false, want true")
	}
	if !d.ND {
		t.Error("ND = false, want true")
	}
	if len(d.RetExprs) != 2 || d.RetExprs[0] != "nil" || d.RetExprs[1] != "ErrNotFound" {
		t.Errorf("RetExprs = %v, want [nil ErrNotFound]", d.RetExprs)
	}
	if len(d.Vars) != 1 || d.Vars[0] != "db" {
		t.Errorf("Vars = %v, want [db]", d.Vars)
	}
}

func TestParseDirective_RetExprs_NestedCall(t *testing.T) {
	d := ParseDirective(`// @require -ret(nil, fmt.Errorf("bad id: %s", id)) len(id) > 0`)
	if d == nil {
		t.Fatal("ParseDirective returned nil")
	}
	if !d.Ret {
		t.Error("Ret = false, want true")
	}
	if len(d.RetExprs) != 2 {
		t.Fatalf("RetExprs len = %d, want 2", len(d.RetExprs))
	}
	if d.RetExprs[0] != "nil" {
		t.Errorf("RetExprs[0] = %q, want %q", d.RetExprs[0], "nil")
	}
	if d.RetExprs[1] != `fmt.Errorf("bad id: %s", id)` {
		t.Errorf("RetExprs[1] = %q, want %q", d.RetExprs[1], `fmt.Errorf("bad id: %s", id)`)
	}
	if d.Expr != "len(id) > 0" {
		t.Errorf("Expr = %q, want %q", d.Expr, "len(id) > 0")
	}
}

func TestParseDirective_RetExprs_SingleValue(t *testing.T) {
	d := ParseDirective(`// @require -ret(defaultVal) x != nil`)
	if d == nil {
		t.Fatal("ParseDirective returned nil")
	}
	if len(d.RetExprs) != 1 || d.RetExprs[0] != "defaultVal" {
		t.Errorf("RetExprs = %v, want [defaultVal]", d.RetExprs)
	}
	if d.Expr != "x != nil" {
		t.Errorf("Expr = %q, want %q", d.Expr, "x != nil")
	}
}

func TestParseDirective_RetExprs_WithLog(t *testing.T) {
	d := ParseDirective(`// @require -log -ret(nil, ErrBadRequest) amount > 0, "positive amount required"`)
	if d == nil {
		t.Fatal("ParseDirective returned nil")
	}
	if !d.Log {
		t.Error("Log = false, want true")
	}
	if !d.Ret {
		t.Error("Ret = false, want true")
	}
	if len(d.RetExprs) != 2 || d.RetExprs[0] != "nil" || d.RetExprs[1] != "ErrBadRequest" {
		t.Errorf("RetExprs = %v, want [nil ErrBadRequest]", d.RetExprs)
	}
	if d.Expr != "amount > 0" {
		t.Errorf("Expr = %q, want %q", d.Expr, "amount > 0")
	}
	if d.Message != "positive amount required" {
		t.Errorf("Message = %q, want %q", d.Message, "positive amount required")
	}
}

func TestParseDirective_RetExprs_FlagOrderVariants(t *testing.T) {
	tests := []struct {
		input    string
		wantVars []string
		wantRet  []string
	}{
		{"// @require -ret(nil, ErrX) -nd a, b", []string{"a", "b"}, []string{"nil", "ErrX"}},
		{"// @require -nd -ret(nil, ErrX) a, b", []string{"a", "b"}, []string{"nil", "ErrX"}},
	}
	for _, tt := range tests {
		d := ParseDirective(tt.input)
		if d == nil {
			t.Fatalf("ParseDirective(%q) = nil", tt.input)
		}
		if !d.Ret || !d.ND {
			t.Errorf("flags wrong for %q: Ret=%v ND=%v", tt.input, d.Ret, d.ND)
		}
		if len(d.RetExprs) != len(tt.wantRet) {
			t.Errorf("RetExprs = %v, want %v for %q", d.RetExprs, tt.wantRet, tt.input)
			continue
		}
		for i, v := range d.RetExprs {
			if v != tt.wantRet[i] {
				t.Errorf("RetExprs[%d] = %q, want %q for %q", i, v, tt.wantRet[i], tt.input)
			}
		}
		if len(d.Vars) != len(tt.wantVars) {
			t.Errorf("Vars = %v, want %v for %q", d.Vars, tt.wantVars, tt.input)
		}
	}
}

func TestSplitTopLevelCommas(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"a, b, c", []string{"a", "b", "c"}},
		{`nil, fmt.Errorf("x: %s", id)`, []string{"nil", `fmt.Errorf("x: %s", id)`}},
		{"defaultVal", []string{"defaultVal"}},
		{`"hello, world"`, []string{`"hello, world"`}},
		{"f(a, b), g(c, d)", []string{"f(a, b)", "g(c, d)"}},
	}
	for _, tt := range tests {
		got := splitTopLevelCommas(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitTopLevelCommas(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i, v := range got {
			if v != tt.want[i] {
				t.Errorf("splitTopLevelCommas(%q)[%d] = %q, want %q", tt.input, i, v, tt.want[i])
			}
		}
	}
}

func TestParseDirective_MustRet(t *testing.T) {
	d := ParseDirective("// @must -ret")
	if d == nil {
		t.Fatal("ParseDirective returned nil")
	}
	if d.Kind != Must {
		t.Errorf("Kind = %v, want Must", d.Kind)
	}
	if !d.Ret {
		t.Error("Ret should be true")
	}
	if len(d.RetExprs) != 0 {
		t.Errorf("RetExprs = %v, want empty", d.RetExprs)
	}
}

func TestParseDirective_MustRetExprs(t *testing.T) {
	d := ParseDirective("// @must -ret(nil, ErrNotFound)")
	if d == nil {
		t.Fatal("ParseDirective returned nil")
	}
	if d.Kind != Must {
		t.Errorf("Kind = %v, want Must", d.Kind)
	}
	if !d.Ret {
		t.Error("Ret should be true")
	}
	if len(d.RetExprs) != 2 || d.RetExprs[0] != "nil" || d.RetExprs[1] != "ErrNotFound" {
		t.Errorf("RetExprs = %v, want [nil, ErrNotFound]", d.RetExprs)
	}
}

func TestParseDirective_MustPlain(t *testing.T) {
	// Ensure @must without flags still works
	d := ParseDirective("// @must")
	if d == nil {
		t.Fatal("ParseDirective returned nil")
	}
	if d.Kind != Must {
		t.Errorf("Kind = %v, want Must", d.Kind)
	}
	if d.Ret {
		t.Error("Ret should be false for plain @must")
	}
}

func TestParseDirective_MustLog(t *testing.T) {
	d := ParseDirective("// @must -log")
	if d == nil {
		t.Fatal("ParseDirective returned nil")
	}
	if d.Kind != Must {
		t.Errorf("Kind = %v, want Must", d.Kind)
	}
	if !d.Log {
		t.Error("Log = false, want true")
	}
	if !d.Ret {
		t.Error("Ret = false, want true (-log implies -ret)")
	}
}

func TestParseDirective_MustLogRet(t *testing.T) {
	d := ParseDirective("// @must -log -ret")
	if d == nil {
		t.Fatal("ParseDirective returned nil")
	}
	if !d.Log {
		t.Error("Log = false, want true")
	}
	if !d.Ret {
		t.Error("Ret = false, want true")
	}
}

func TestParseDirective_MustLogRetExprs(t *testing.T) {
	d := ParseDirective(`// @must -log -ret("", ErrNotFound)`)
	if d == nil {
		t.Fatal("ParseDirective returned nil")
	}
	if !d.Log {
		t.Error("Log = false, want true")
	}
	if !d.Ret {
		t.Error("Ret = false, want true")
	}
	if len(d.RetExprs) != 2 || d.RetExprs[0] != `""` || d.RetExprs[1] != "ErrNotFound" {
		t.Errorf("RetExprs = %v, want [\"\", ErrNotFound]", d.RetExprs)
	}
}
