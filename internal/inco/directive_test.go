package inco

import "testing"

func TestParseDirective_ExprOnly(t *testing.T) {
	// No action keyword → default panic.
	d := ParseDirective("// @require x > 0")
	if d == nil {
		t.Fatal("expected directive")
	}
	if d.Action != ActionPanic {
		t.Errorf("Action = %v, want Panic", d.Action)
	}
	if d.Expr != "x > 0" {
		t.Errorf("Expr = %q, want %q", d.Expr, "x > 0")
	}
	if len(d.ActionArgs) != 0 {
		t.Errorf("ActionArgs = %v, want empty", d.ActionArgs)
	}
}

func TestParseDirective_PanicExplicit(t *testing.T) {
	d := ParseDirective("// @require x != nil panic")
	if d == nil {
		t.Fatal("nil")
	}
	if d.Action != ActionPanic {
		t.Errorf("Action = %v", d.Action)
	}
	if d.Expr != "x != nil" {
		t.Errorf("Expr = %q", d.Expr)
	}
}

func TestParseDirective_PanicWithMessage(t *testing.T) {
	d := ParseDirective(`// @require x > 0 panic("bad input")`)
	if d == nil {
		t.Fatal("nil")
	}
	if d.Action != ActionPanic {
		t.Errorf("Action = %v", d.Action)
	}
	if len(d.ActionArgs) != 1 || d.ActionArgs[0] != `"bad input"` {
		t.Errorf("ActionArgs = %v", d.ActionArgs)
	}
	if d.Expr != "x > 0" {
		t.Errorf("Expr = %q", d.Expr)
	}
}

func TestParseDirective_BlockComment(t *testing.T) {
	d := ParseDirective("/* @require x > 0 panic */")
	if d == nil {
		t.Fatal("nil")
	}
	if d.Action != ActionPanic {
		t.Errorf("Action = %v", d.Action)
	}
}

func TestParseDirective_NotDirective(t *testing.T) {
	for _, input := range []string{
		"// regular comment",
		"// @unknown foo",
		"x + y",
		"",
		"// @require", // no expression
	} {
		if d := ParseDirective(input); d != nil {
			t.Errorf("ParseDirective(%q) = %+v, want nil", input, d)
		}
	}
}

// --- @must tests ---

func TestParseDirective_MustBare(t *testing.T) {
	d := ParseDirective("// @must")
	if d == nil {
		t.Fatal("nil")
	}
	if d.Kind != KindMust {
		t.Errorf("Kind = %v, want Must", d.Kind)
	}
	if d.Action != ActionPanic {
		t.Errorf("Action = %v, want Panic", d.Action)
	}
}

func TestParseDirective_MustPanicMsg(t *testing.T) {
	d := ParseDirective(`// @must panic("db error")`)
	if d == nil {
		t.Fatal("nil")
	}
	if d.Kind != KindMust {
		t.Errorf("Kind = %v", d.Kind)
	}
	if d.Action != ActionPanic {
		t.Errorf("Action = %v", d.Action)
	}
	if len(d.ActionArgs) != 1 || d.ActionArgs[0] != `"db error"` {
		t.Errorf("ActionArgs = %v", d.ActionArgs)
	}
}

// --- @expect tests ---

func TestParseDirective_ExpectBare(t *testing.T) {
	d := ParseDirective("// @expect")
	if d == nil {
		t.Fatal("nil")
	}
	if d.Kind != KindExpect {
		t.Errorf("Kind = %v, want Expect", d.Kind)
	}
	if d.Action != ActionPanic {
		t.Errorf("Action = %v, want Panic", d.Action)
	}
}

func TestParseDirective_ExpectPanicMsg(t *testing.T) {
	d := ParseDirective(`// @expect panic("key missing")`)
	if d == nil {
		t.Fatal("nil")
	}
	if d.Kind != KindExpect {
		t.Errorf("Kind = %v", d.Kind)
	}
	if d.Action != ActionPanic {
		t.Errorf("Action = %v", d.Action)
	}
	if len(d.ActionArgs) != 1 || d.ActionArgs[0] != `"key missing"` {
		t.Errorf("ActionArgs = %v", d.ActionArgs)
	}
}

func TestParseDirective_NotKeywordPrefix(t *testing.T) {
	// "panicHandler" should NOT match action "panic".
	d := ParseDirective("// @require panicHandler != nil")
	if d == nil {
		t.Fatal("nil")
	}
	// No action matched → default panic; entire string is expression.
	if d.Action != ActionPanic {
		t.Errorf("Action = %v, want Panic", d.Action)
	}
	if d.Expr != "panicHandler != nil" {
		t.Errorf("Expr = %q", d.Expr)
	}
}

func TestSplitTopLevel(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"a, b, c", []string{"a", "b", "c"}},
		{`nil, fmt.Errorf("x: %s", id)`, []string{"nil", `fmt.Errorf("x: %s", id)`}},
		{"single", []string{"single"}},
		{`"hello, world"`, []string{`"hello, world"`}},
		{"f(a, b), g(c)", []string{"f(a, b)", "g(c)"}},
	}
	for _, tt := range tests {
		got := splitTopLevel(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitTopLevel(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitTopLevel(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestActionKindString(t *testing.T) {
	tests := []struct {
		k    ActionKind
		want string
	}{
		{ActionPanic, "panic"},
		{ActionKind(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.k.String(); got != tt.want {
			t.Errorf("ActionKind(%d).String() = %q, want %q", tt.k, got, tt.want)
		}
	}
}

// --- @ensure tests ---

func TestParseDirective_EnsureBare(t *testing.T) {
	d := ParseDirective("// @ensure len(result) > 0")
	if d == nil {
		t.Fatal("nil")
	}
	if d.Kind != KindEnsure {
		t.Errorf("Kind = %v, want Ensure", d.Kind)
	}
	if d.Expr != "len(result) > 0" {
		t.Errorf("Expr = %q", d.Expr)
	}
	if d.Action != ActionPanic {
		t.Errorf("Action = %v, want Panic", d.Action)
	}
}

func TestParseDirective_EnsurePanicMsg(t *testing.T) {
	d := ParseDirective(`// @ensure result != nil panic("postcondition failed")`)
	if d == nil {
		t.Fatal("nil")
	}
	if d.Kind != KindEnsure {
		t.Errorf("Kind = %v", d.Kind)
	}
	if d.Expr != "result != nil" {
		t.Errorf("Expr = %q", d.Expr)
	}
	if len(d.ActionArgs) != 1 || d.ActionArgs[0] != `"postcondition failed"` {
		t.Errorf("ActionArgs = %v", d.ActionArgs)
	}
}

func TestParseDirective_EnsureNoExpr(t *testing.T) {
	// @ensure with no expression is invalid
	d := ParseDirective("// @ensure")
	if d != nil {
		t.Errorf("expected nil for bare @ensure, got %+v", d)
	}
}
