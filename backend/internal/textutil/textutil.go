package textutil

import (
	"strings"
	"unicode/utf8"
)

// Sanitize returns s with invalid UTF-8 sequences removed and NUL bytes stripped.
// Postgres text columns reject both.
func Sanitize(s string) string {
	s = strings.ToValidUTF8(s, "")
	return strings.ReplaceAll(s, "\x00", "")
}

// TruncateRunes shortens s to maxRunes Unicode code points.
func TruncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}

// TruncateBytes caps the total byte length of s plus suffix to maxBytes.
// When truncation is required, s is cut on a rune boundary before suffix is appended.
func TruncateBytes(s, suffix string, maxBytes int) string {
	if maxBytes <= 0 {
		return suffix
	}
	if len(s) <= maxBytes {
		return s
	}
	max := maxBytes - len(suffix)
	if max < 0 {
		return suffix
	}
	truncated := s[:max]
	for len(truncated) > 0 && !utf8.ValidString(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated + suffix
}
