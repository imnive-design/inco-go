// Package inco implements a compile-time code injection engine.
//
// Directives:
//
//	// @require <expression> [action]
//	result, _ := foo() // @must [action]
//	v, _ := m[k]       // @expect [action]
//	// @ensure <expression> [action]
//
// Actions:
//
//	panic            — panic with default message (default action)
//	panic("msg")     — panic with custom message
//	return           — bare return
//	return(x, y)     — return with values
//	continue         — continue enclosing loop
//	break            — break enclosing loop
package inco

import "fmt"

// ---------------------------------------------------------------------------
// Action
// ---------------------------------------------------------------------------

// ActionKind identifies the response to a directive violation.
type ActionKind int

const (
	ActionPanic    ActionKind = iota // default — panic
	ActionReturn                     // return (with optional values)
	ActionContinue                   // continue enclosing loop
	ActionBreak                      // break enclosing loop
)

var actionNames = map[ActionKind]string{
	ActionPanic:    "panic",
	ActionReturn:   "return",
	ActionContinue: "continue",
	ActionBreak:    "break",
}

func (k ActionKind) String() string {
	if s, ok := actionNames[k]; ok {
		return s
	}
	return "unknown"
}

// ---------------------------------------------------------------------------
// DirectiveKind
// ---------------------------------------------------------------------------

// DirectiveKind distinguishes the three directive types.
type DirectiveKind int

const (
	KindRequire DirectiveKind = iota // standalone: @require <expr>
	KindMust                         // inline: error check
	KindExpect                       // inline: ok/bool check
	KindEnsure                       // standalone: postcondition via defer
)

var kindNames = map[DirectiveKind]string{
	KindRequire: "require",
	KindMust:    "must",
	KindExpect:  "expect",
	KindEnsure:  "ensure",
}

func (k DirectiveKind) String() string {
	if s, ok := kindNames[k]; ok {
		return s
	}
	return "unknown"
}

// ---------------------------------------------------------------------------
// Directive
// ---------------------------------------------------------------------------

// Directive is the parsed form of a single @require / @must / @expect / @ensure comment.
type Directive struct {
	Kind       DirectiveKind // require, must, expect, or ensure
	Action     ActionKind    // panic (default), return, continue, break
	ActionArgs []string      // e.g. panic("msg") → ['"msg"'], return(0, err) → ["0", "err"]
	Expr       string        // the Go boolean expression (@require only)
}

// ---------------------------------------------------------------------------
// Engine types
// ---------------------------------------------------------------------------

// Overlay is the JSON structure consumed by `go build -overlay`.
type Overlay struct {
	Replace map[string]string `json:"Replace"`
}

// ---------------------------------------------------------------------------
// Recover helper
// ---------------------------------------------------------------------------

// Recover converts a panic (from @require/@must/@expect/@ensure violations)
// into an error. Call it via defer:
//
//	var err error
//	defer inco.Recover(&err)
//	inco.NewEngine(dir).Run()
func Recover(errp *error) {
	if r := recover(); r != nil {
		if e, ok := r.(error); ok {
			*errp = e
		} else {
			*errp = fmt.Errorf("%v", r)
		}
	}
}
