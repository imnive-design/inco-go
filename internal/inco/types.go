// Package inco implements a compile-time code injection engine.
//
// Directives:
//
//	// @require <expression> [panic[("msg")]]
//	result, _ := foo() // @must [panic[("msg")]]
//	v, _ := m[k]       // @expect [panic[("msg")]]
//	// @ensure <expression> [panic[("msg")]]
//
// The only action is panic (the default).
// Use panic("custom message") to customise the message.
package inco

import "fmt"

// ---------------------------------------------------------------------------
// Action
// ---------------------------------------------------------------------------

// ActionKind identifies the response to a require violation.
type ActionKind int

const (
	ActionPanic ActionKind = iota // default — only action
)

var actionNames = map[ActionKind]string{
	ActionPanic: "panic",
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
	Action     ActionKind    // always ActionPanic
	ActionArgs []string      // e.g. panic("msg") → ['"msg"']
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
