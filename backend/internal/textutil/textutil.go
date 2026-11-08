package textutil

import (
	"bytes"
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

// SanitizeJSON returns b with every \u0000 escape inside a JSON string removed.
// encoding/json writes a NUL as that escape, and a Postgres jsonb column rejects
// it, so a model's output carrying one would otherwise fail the whole write.
// Escapes are consumed left to right, so an escaped backslash followed by
// "u0000" is literal text and survives.
func SanitizeJSON(b []byte) []byte {
	if !bytes.Contains(b, []byte(`\u0000`)) {
		return b
	}
	out := make([]byte, 0, len(b))
	inString := false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if !inString {
			if c == '"' {
				inString = true
			}
			out = append(out, c)
			continue
		}
		switch c {
		case '"':
			inString = false
			out = append(out, c)
		case '\\':
			if i+5 < len(b) && string(b[i+1:i+6]) == "u0000" {
				i += 5
				continue
			}
			out = append(out, c)
			if i+1 < len(b) {
				i++
				out = append(out, b[i])
			}
		default:
			out = append(out, c)
		}
	}
	return out
}
