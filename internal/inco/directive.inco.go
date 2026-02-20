package inco

import "strings"

// keywords maps directive prefixes to their DirectiveKind.
var keywords = map[string]DirectiveKind{
	"@require": KindRequire,
	"@must":    KindMust,
	"@expect":  KindExpect,
	"@ensure":  KindEnsure,
}

// actionKeywords maps action names to their ActionKind.
var actionKeywords = map[string]ActionKind{
	"panic":    ActionPanic,
	"return":   ActionReturn,
	"continue": ActionContinue,
	"break":    ActionBreak,
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

	// Find the rightmost action keyword split across all known actions.
	type actionMatch struct {
		pos    int
		action ActionKind
		args   []string
	}
	var best *actionMatch

	for keyword, action := range actionKeywords {
		// Try "expr keyword(args...)" — find rightmost " keyword("
		needle := " " + keyword + "("
		if idx := strings.LastIndex(rest, needle); idx >= 0 {
			argStart := idx + 1 + len(keyword) // position of '('
			args, remaining, ok := parseActionArgs(rest[argStart:])
			if ok && strings.TrimSpace(remaining) == "" {
				if best == nil || idx > best.pos {
					best = &actionMatch{pos: idx, action: action, args: args}
				}
			}
		}

		// Try "expr keyword" — bare keyword at end
		suffix := " " + keyword
		if strings.HasSuffix(rest, suffix) {
			idx := len(rest) - len(suffix)
			if best == nil || idx > best.pos {
				best = &actionMatch{pos: idx, action: action}
			}
		}
	}

	if best != nil {
		d.Action = best.action
		d.ActionArgs = best.args
		d.Expr = strings.TrimSpace(rest[:best.pos])
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

	for keyword, action := range actionKeywords {
		if !strings.HasPrefix(rest, keyword) {
			continue
		}
		after := rest[len(keyword):]
		if len(after) > 0 && after[0] != ' ' && after[0] != '\t' && after[0] != '(' {
			continue // not a full keyword match
		}
		d.Action = action
		after = strings.TrimSpace(after)
		if strings.HasPrefix(after, "(") {
			args, _, ok := parseActionArgs(after)
			if ok {
				d.ActionArgs = args
			}
		}
		return d
	}

	return nil // unrecognized action
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
