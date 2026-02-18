package inco

import (
	"strings"
)

// Kind represents the type of contract directive.
type Kind int

const (
	// Require defines a precondition (checked at function entry).
	Require Kind = iota
	// Must asserts that an operation must not return an error (panic on error).
	Must
)

func (k Kind) String() string {
	switch k {
	case Require:
		return "require"
	case Must:
		return "must"
	default:
		return "unknown"
	}
}

// Directive represents a parsed contract annotation.
type Directive struct {
	Kind     Kind
	ND       bool     // -nd (non-defaulted) flag
	Ret      bool     // -ret (return on violation) flag
	Log      bool     // -log (log and return on violation) flag; implies Ret
	RetExprs []string // custom return expressions for -ret(expr1, expr2, ...)
	Vars     []string // variable names for -nd mode
	Expr     string   // custom boolean expression
	Message  string   // custom error message (optional)
}

// ParseDirective parses a Go comment text into a Directive.
// Returns nil if the comment does not contain a contract directive.
//
// Supported forms:
//
//	// @require -nd var1, var2
//	// @require len(x) > 0, "x must not be empty"
//	// @must
func ParseDirective(text string) *Directive {
	// @require len(text) > 0, "text must not be empty"
	s := strings.TrimSpace(text)

	// Strip comment markers
	if strings.HasPrefix(s, "//") {
		s = strings.TrimSpace(s[2:])
	} else if strings.HasPrefix(s, "/*") {
		s = strings.TrimSuffix(strings.TrimPrefix(s, "/*"), "*/")
		s = strings.TrimSpace(s)
	}

	if !strings.HasPrefix(s, "@") {
		return nil
	}

	switch {
	case strings.HasPrefix(s, "@require"):
		return parseRequire(strings.TrimPrefix(s, "@require"))
	case strings.HasPrefix(s, "@must"):
		return parseMust(strings.TrimPrefix(s, "@must"))
	}

	return nil
}

func parseMust(rest string) *Directive {
	rest = strings.TrimSpace(rest)
	d := &Directive{Kind: Must}

	// Parse flags: -ret, -ret(...), -log
	for {
		if strings.HasPrefix(rest, "-ret") {
			d.Ret = true
			after := rest[4:]
			if strings.HasPrefix(strings.TrimSpace(after), "(") {
				after = strings.TrimSpace(after)
				exprs, remaining, ok := parseRetExprs(after)
				if ok {
					d.RetExprs = exprs
					rest = strings.TrimSpace(remaining)
				} else {
					rest = strings.TrimSpace(after)
				}
			} else {
				rest = strings.TrimSpace(after)
			}
		} else if strings.HasPrefix(rest, "-log") {
			d.Log = true
			d.Ret = true // -log implies -ret
			rest = strings.TrimSpace(rest[4:])
		} else {
			break
		}
	}

	return d
}

func parseRequire(rest string) *Directive {
	rest = strings.TrimSpace(rest)
	d := &Directive{Kind: Require}

	if rest == "" {
		return d
	}

	// Parse flags (order-insensitive): -nd, -ret, -ret(...), -log
	for {
		if strings.HasPrefix(rest, "-nd") {
			d.ND = true
			rest = strings.TrimSpace(rest[3:])
		} else if strings.HasPrefix(rest, "-ret") {
			d.Ret = true
			after := rest[4:]
			// Check for -ret(expr1, expr2, ...)
			if strings.HasPrefix(strings.TrimSpace(after), "(") {
				after = strings.TrimSpace(after)
				// Find matching closing paren
				exprs, remaining, ok := parseRetExprs(after)
				if ok {
					d.RetExprs = exprs
					rest = strings.TrimSpace(remaining)
				} else {
					rest = strings.TrimSpace(after)
				}
			} else {
				rest = strings.TrimSpace(after)
			}
		} else if strings.HasPrefix(rest, "-log") {
			d.Log = true
			d.Ret = true // -log implies -ret
			rest = strings.TrimSpace(rest[4:])
		} else {
			break
		}
	}

	if rest == "" {
		return d
	}

	// -nd mode: parse variable list
	if d.ND {
		for _, v := range strings.Split(rest, ",") {
			v = strings.TrimSpace(v)
			if v != "" {
				d.Vars = append(d.Vars, v)
			}
		}
		return d
	}

	// Expression form: expr [, "message"]
	if idx := strings.LastIndex(rest, ","); idx >= 0 {
		candidate := strings.TrimSpace(rest[idx+1:])
		if len(candidate) >= 2 && candidate[0] == '"' && candidate[len(candidate)-1] == '"' {
			d.Expr = strings.TrimSpace(rest[:idx])
			d.Message = candidate[1 : len(candidate)-1]
			return d
		}
	}

	d.Expr = rest
	return d
}

// parseRetExprs parses the parenthesized expression list after -ret.
// Input starts with '('. Returns the parsed expressions, remaining text after ')', and ok.
func parseRetExprs(s string) (exprs []string, remaining string, ok bool) {
	if len(s) == 0 || s[0] != '(' {
		return nil, s, false
	}

	// Find matching closing paren, respecting nesting and string literals
	depth := 0
	inString := false
	escaped := false
	closeIdx := -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inString {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == '(' {
			depth++
		} else if c == ')' {
			depth--
			if depth == 0 {
				closeIdx = i
				break
			}
		}
	}
	if closeIdx < 0 {
		return nil, s, false
	}

	inner := s[1:closeIdx]
	remaining = s[closeIdx+1:]

	exprs = splitTopLevelCommas(inner)
	if len(exprs) == 0 {
		return nil, remaining, false
	}
	return exprs, remaining, true
}

// splitTopLevelCommas splits a string by commas that are not inside
// parentheses, brackets, braces, or string literals.
func splitTopLevelCommas(s string) []string {
	var parts []string
	depth := 0
	inString := false
	escaped := false
	start := 0

	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inString {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch c {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				part := strings.TrimSpace(s[start:i])
				if part != "" {
					parts = append(parts, part)
				}
				start = i + 1
			}
		}
	}

	// Last segment
	part := strings.TrimSpace(s[start:])
	if part != "" {
		parts = append(parts, part)
	}
	return parts
}
