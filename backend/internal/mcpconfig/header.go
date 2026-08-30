package mcpconfig

import (
	"fmt"
	"sort"
	"strings"
)

// validateHeaders rejects a header that net/http would refuse to send, before
// it reaches the transport as an opaque "invalid header field value" error.
//
// The usual cause is a secret stored with a trailing newline — a token pasted
// into the value textarea, or copied out of a file — substituted into
// "Bearer ${TOKEN}". Naming the header and its unexpanded template points at
// the secret to fix without printing what the secret holds.
func validateHeaders(template, expanded map[string]string) error {
	for _, k := range sortedKeys(expanded) {
		if !validHeaderName(k) {
			return fmt.Errorf("%w: %q is not a valid header name", ErrInvalidHeader, k)
		}
		if !validHeaderValue(expanded[k]) {
			return fmt.Errorf(
				"%w: %q holds a newline or other control character after expanding %q — "+
					"re-save the secret it references as a single line, with no trailing newline",
				ErrInvalidHeader, k, template[k],
			)
		}
	}
	return nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// validHeaderValue mirrors net/http's own field-value check: every byte must be
// printable, with the horizontal tab as the one exception.
func validHeaderValue(v string) bool {
	for i := 0; i < len(v); i++ {
		if b := v[i]; (b < 0x20 && b != '\t') || b == 0x7f {
			return false
		}
	}
	return true
}

// validHeaderName mirrors net/http's field-name check: a non-empty RFC 7230 token.
func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	const tokenPunct = "!#$%&'*+-.^_`|~"
	for i := 0; i < len(name); i++ {
		b := name[i]
		switch {
		case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		case strings.IndexByte(tokenPunct, b) >= 0:
		default:
			return false
		}
	}
	return true
}
