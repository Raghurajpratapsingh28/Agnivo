// Package strx provides string helpers tuned for the platform's needs: URL-safe
// slugs, truncation, casing conversions, and redaction of secrets in logs.
package strx

import (
	"strings"
	"unicode"
)

// Slugify converts s into a lowercase, URL-safe slug: runs of non-alphanumeric
// characters collapse to single hyphens and leading/trailing hyphens are
// trimmed. It is deterministic and ASCII-folding for letters and digits.
func Slugify(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevHyphen := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// Truncate shortens s to at most max runes, appending an ellipsis when the
// string was cut. It is rune-safe and never splits a multi-byte character.
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max == 1 {
		return string(runes[:1])
	}
	return string(runes[:max-1]) + "\u2026"
}

// IsBlank reports whether s is empty or contains only whitespace.
func IsBlank(s string) bool { return strings.TrimSpace(s) == "" }

// Coalesce returns the first non-blank string in vals, or "" when all are blank.
func Coalesce(vals ...string) string {
	for _, v := range vals {
		if !IsBlank(v) {
			return v
		}
	}
	return ""
}

// ToSnake converts a camelCase or PascalCase identifier to snake_case.
func ToSnake(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 4)
	var prev rune
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev)) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
		prev = r
	}
	return b.String()
}

// Redact masks all but the last n characters of a secret with asterisks. When s
// is shorter than or equal to n, the entire value is masked so short secrets
// are never leaked.
func Redact(s string, n int) string {
	if n < 0 {
		n = 0
	}
	r := []rune(s)
	if len(r) <= n {
		return strings.Repeat("*", len(r))
	}
	return strings.Repeat("*", len(r)-n) + string(r[len(r)-n:])
}
