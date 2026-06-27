// Package search parses the request-search query DSL into a parameterised SQL
// WHERE fragment. The grammar is a pragmatic, Lucene-style search subset:
//
//	free text            → matches request content
//	content:foo          → content contains "foo"
//	method:POST          → exact method
//	type:web|email|dns   → exact request type
//	ip:1.2.3.4           → exact IP
//	host:example.com     → hostname contains
//	ua:curl              → user-agent contains
//	url:/path            → url contains
//	query:foo            → query string contains
//	headers.x-test:bar   → headers JSON contains both key and value
//	headers:bar          → headers JSON contains value
//	_exists_:field       → field is non-empty (custom_action_errors|custom_action_output|content)
//	created_at:[* TO now-14d]  → created_at within an (optionally open) range
//
// Terms are ANDed. Values may be double-quoted to include spaces. Column names
// are never taken from user input — only whitelisted columns are referenced and
// every value is bound as a placeholder argument, so the output is injection-safe.
package search

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Filter is a compiled query: an SQL fragment (ANDed clauses joined with AND)
// and its bound arguments. An empty SQL means "no constraints".
type Filter struct {
	SQL  string
	Args []any
}

// Empty reports whether the filter adds no constraints.
func (f Filter) Empty() bool { return strings.TrimSpace(f.SQL) == "" }

// Parse compiles a query string. `now` anchors relative date expressions.
func Parse(query string, now time.Time) Filter {
	var clauses []string
	var args []any

	for _, tok := range tokenize(query) {
		clause, a, ok := compileToken(tok, now)
		if !ok {
			continue
		}
		clauses = append(clauses, clause)
		args = append(args, a...)
	}

	return Filter{SQL: strings.Join(clauses, " AND "), Args: args}
}

func compileToken(tok string, now time.Time) (string, []any, bool) {
	field, value, hasColon := strings.Cut(tok, ":")
	if !hasColon {
		// free text → content match
		return "content LIKE ?", []any{like(tok)}, true
	}
	field = strings.ToLower(strings.TrimSpace(field))
	value = unquote(strings.TrimSpace(value))

	switch field {
	case "content", "body":
		return "content LIKE ?", []any{like(value)}, true
	case "method":
		return "method = ?", []any{strings.ToUpper(value)}, true
	case "type":
		return "type = ?", []any{strings.ToLower(value)}, true
	case "ip":
		return "ip = ?", []any{value}, true
	case "host", "hostname":
		return "hostname LIKE ?", []any{like(value)}, true
	case "ua", "user_agent":
		return "user_agent LIKE ?", []any{like(value)}, true
	case "url":
		return "url LIKE ?", []any{like(value)}, true
	case "query":
		return "query LIKE ?", []any{like(value)}, true
	case "headers":
		return "headers LIKE ?", []any{like(value)}, true
	case "_exists_":
		return existsClause(value)
	case "created_at", "date":
		return rangeClause(value, now)
	default:
		if strings.HasPrefix(field, "headers.") {
			key := strings.TrimPrefix(field, "headers.")
			return "(headers LIKE ? AND headers LIKE ?)", []any{like(key), like(value)}, true
		}
		// Unknown field: fall back to a free-text content match on the value.
		return "content LIKE ?", []any{like(value)}, true
	}
}

// existsFields whitelists the columns usable with _exists_.
var existsFields = map[string]bool{
	"custom_action_errors": true,
	"custom_action_output": true,
	"content":              true,
}

func existsClause(field string) (string, []any, bool) {
	field = strings.ToLower(field)
	if !existsFields[field] {
		return "", nil, false
	}
	// Non-empty: not blank, not an empty JSON object, not null.
	return fmt.Sprintf("(%s != '' AND %s != '{}' AND %s != 'null')", field, field, field), nil, true
}

func rangeClause(value string, now time.Time) (string, []any, bool) {
	inner := strings.TrimSpace(value)
	inner = strings.TrimPrefix(inner, "[")
	inner = strings.TrimSuffix(inner, "]")
	lo, hi, ok := strings.Cut(inner, " TO ")
	if !ok {
		return "", nil, false
	}
	var clauses []string
	var args []any
	if t, has := parseBound(strings.TrimSpace(lo), now); has {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, t.UTC().Format(time.RFC3339Nano))
	}
	if t, has := parseBound(strings.TrimSpace(hi), now); has {
		clauses = append(clauses, "created_at <= ?")
		args = append(args, t.UTC().Format(time.RFC3339Nano))
	}
	if len(clauses) == 0 {
		return "", nil, false
	}
	return "(" + strings.Join(clauses, " AND ") + ")", args, true
}

// parseBound parses a range endpoint: "*" (open), "now[+-]<n><unit>" where unit
// is d/h/m, or an absolute date (RFC3339 or YYYY-MM-DD).
func parseBound(s string, now time.Time) (time.Time, bool) {
	if s == "" || s == "*" {
		return time.Time{}, false
	}
	if strings.HasPrefix(s, "now") {
		rest := strings.TrimPrefix(s, "now")
		if rest == "" {
			return now, true
		}
		// Need at least sign + digit + unit, e.g. "-1d".
		if len(rest) < 3 {
			return time.Time{}, false
		}
		sign := time.Duration(1)
		switch rest[0] {
		case '-':
			sign = -1
		case '+':
			sign = 1
		default:
			return time.Time{}, false
		}
		num := rest[1 : len(rest)-1]
		unit := rest[len(rest)-1]
		n, err := strconv.Atoi(num)
		if err != nil {
			return time.Time{}, false
		}
		var d time.Duration
		switch unit {
		case 'd':
			d = time.Duration(n) * 24 * time.Hour
		case 'h':
			d = time.Duration(n) * time.Hour
		case 'm':
			d = time.Duration(n) * time.Minute
		default:
			return time.Time{}, false
		}
		return now.Add(sign * d), true
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func like(v string) string { return "%" + v + "%" }

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// tokenize splits a query on whitespace, keeping double-quoted strings and
// bracketed [ ... ] ranges intact.
func tokenize(q string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := false
	depth := 0

	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}

	for _, r := range q {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case r == '[' && !inQuote:
			depth++
			cur.WriteRune(r)
		case r == ']' && !inQuote:
			if depth > 0 {
				depth--
			}
			cur.WriteRune(r)
		case unicode.IsSpace(r) && !inQuote && depth == 0:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return tokens
}
