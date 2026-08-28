package mcpconfig

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrUnresolvedVariable is returned when a ${NAME} reference in mcp.json has no
// value in either the secret store or the process environment.
var ErrUnresolvedVariable = errors.New("unresolved variable in mcp config")

// varRef matches the variable name inside a ${...} reference. It is a superset
// of both POSIX environment variable names and the prompt secret name rules, so
// a reference can point at either source.
var varRef = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]*$`)

// Lookup resolves a variable name to its value. The bool reports whether a
// value was found; a non-nil error aborts expansion.
type Lookup func(ctx context.Context, name string) (string, bool, error)

// expandString replaces every ${NAME} reference in s with its resolved value.
//
// Only the ${NAME} form is expanded: a bare $NAME is left alone because URLs
// and passwords legitimately contain '$'. A literal "${" can be written as
// "$${". A reference that is unterminated or whose name is not a valid
// identifier is left in place rather than treated as an error, so unrelated
// text survives untouched.
//
// Missing values yield ErrUnresolvedVariable naming the reference. Resolved
// values are never included in the error.
func expandString(ctx context.Context, s string, lookup Lookup) (string, error) {
	if !strings.Contains(s, "${") {
		return s, nil
	}

	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], "$${") {
			b.WriteString("${")
			i += 3
			continue
		}
		if !strings.HasPrefix(s[i:], "${") {
			b.WriteByte(s[i])
			i++
			continue
		}

		end := strings.IndexByte(s[i+2:], '}')
		if end < 0 {
			b.WriteString(s[i:])
			break
		}
		name := s[i+2 : i+2+end]
		if !varRef.MatchString(name) {
			b.WriteString(s[i : i+2+end+1])
			i += 2 + end + 1
			continue
		}

		value, found, err := lookup(ctx, name)
		if err != nil {
			return "", err
		}
		if !found {
			return "", fmt.Errorf("%w: ${%s}", ErrUnresolvedVariable, name)
		}
		b.WriteString(value)
		i += 2 + end + 1
	}

	return b.String(), nil
}

// RedactValues replaces every non-empty value in values with "***". Errors
// raised against a resolved server embed the request URL, so any secret
// substituted into it would otherwise reach the logs.
func RedactValues(s string, values []string) string {
	for _, v := range values {
		if v == "" {
			continue
		}
		s = strings.ReplaceAll(s, v, "***")
	}
	return s
}
