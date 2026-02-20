package inco

import "strings"

// keywords maps directive prefixes to their DirectiveKind.
var keywords = map[string]DirectiveKind{
	"@require": KindRequire,
	"@must":    KindMust,
	"@expect":  KindExpect,
	"@ensure":  KindEnsure,
}

// ParseDirective extracts a Directive from a comment string.
// Returns nil when the comment is not a valid directive.
func ParseDirective(comment string) *Directive {
	s := stripComment(comment)
	if s == "" {
		return nil
	}

	var kind DirectiveKind
	var keyword string
	for kw, k := range keywords {
		if strings.HasPrefix(s, kw) {
			kind = k
			keyword = kw
			break
		}
	}
	if keyword == "" {
		return nil
	}

	rest := strings.TrimSpace(s[len(keyword):])
	d := &Directive{Kind: kind, Action: ActionPanic}

	if parse, ok := kindParsers[kind]; ok {
		return parse(d, rest)
	}
	return d
}

// kindParsers maps each DirectiveKind to its rest-string parser.
var kindParsers = map[DirectiveKind]func(d *Directive, rest string) *Directive{
	KindRequire: parseRequireRest,
	KindMust:    parseInlineRest,
	KindExpect:  parseInlineRest,
	KindEnsure:  parseRequireRest,
}

func parseRequireRest(d *Directive, rest string) *Directive {
	if rest == "" {
		return nil // expression is mandatory
	}

	// Try "expr panic(args...)" — find rightmost " panic("
	needle := " panic("
	if idx := strings.LastIndex(rest, needle); idx >= 0 {
		argStart := idx + len(" panic") // position of '('
		args, remaining, ok := parseActionArgs(rest[argStart:])
		if ok && strings.TrimSpace(remaining) == "" {
			d.ActionArgs = args
			d.Expr = strings.TrimSpace(rest[:idx])
			if d.Expr == "" {
				return nil
			}
			return d
		}
	}

	// Try "expr panic" — bare panic at end
	if strings.HasSuffix(rest, " panic") {
		d.Expr = strings.TrimSpace(rest[:len(rest)-len(" panic")])
		if d.Expr == "" {
			return nil
		}
		return d
	}

	// No action found — entire rest is the expression.
	d.Expr = rest
	return d
}

func parseInlineRest(d *Directive, rest string) *Directive {
	if rest == "" {
		return d // bare → default panic
	}
	if !strings.HasPrefix(rest, "panic") {
		return nil
	}
	after := rest[len("panic"):]
	if len(after) > 0 && after[0] != ' ' && after[0] != '\t' && after[0] != '(' {
		return nil
	}
	after = strings.TrimSpace(after)
	if strings.HasPrefix(after, "(") {
		args, _, ok := parseActionArgs(after)
		if ok {
			d.ActionArgs = args
		}
	}
	return d
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// stripComment removes Go comment delimiters and returns trimmed content.
func stripComment(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "//") {
		return strings.TrimSpace(s[2:])
	}
	if strings.HasPrefix(s, "/*") && strings.HasSuffix(s, "*/") {
		return strings.TrimSpace(s[2 : len(s)-2])
	}
	return ""
}

// parseActionArgs parses "(arg1, arg2, ...)" respecting nested parens/strings.
// Returns parsed args, the remaining string after ')', and whether parsing succeeded.
func parseActionArgs(s string) ([]string, string, bool) {
	if len(s) == 0 || s[0] != '(' {
		return nil, s, false
	}
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				inner := s[1:i]
				args := splitTopLevel(inner)
				return args, s[i+1:], true
			}
		case '"':
			// Skip string literal
			i++
			for i < len(s) && s[i] != '"' {
				if s[i] == '\\' {
					i++ // skip escaped char
				}
				i++
			}
		}
	}
	return nil, s, false // unmatched paren
}

// splitTopLevel splits s by top-level commas, respecting nested parens,
// brackets, braces and double-quoted strings.
func splitTopLevel(s string) []string {
	var result []string
	depth := 0
	inStr := false
	start := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '"' && !inStr:
			inStr = true
		case ch == '"' && inStr && (i == 0 || s[i-1] != '\\'):
			inStr = false
		case inStr:
			if ch == '\\' {
				i++ // skip next
			}
		case ch == '(' || ch == '[' || ch == '{':
			depth++
		case ch == ')' || ch == ']' || ch == '}':
			depth--
		case ch == ',' && depth == 0:
			result = append(result, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	if last := strings.TrimSpace(s[start:]); last != "" {
		result = append(result, last)
	}
	return result
}
