package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// maxToolNameLen is the shortest cap among the providers nib targets. OpenAI
// allows [A-Za-z0-9_-]{1,64}; the stricter compatible endpoints additionally
// require the first character to be a letter or underscore. The intersection
// is what sanitizeToolName produces.
const maxToolNameLen = 64

// sanitizeToolName rewrites an MCP tool name onto the character set every
// provider nib targets accepts. A server is free to publish dots, slashes,
// spaces or a name longer than the cap; sent as-is those are rejected by the
// provider, and one bad name fails the whole request rather than that tool.
//
// The result is not decoded back. Dispatch uses toolRoute.remoteName, which
// keeps the server's own spelling, because a lossy rewrite has no inverse.
//
// Returns "" when nothing usable survives, which the caller treats as a tool
// to skip.
func sanitizeToolName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			// Runs collapse so that "tools/list items" does not become
			// "tools_list_items" with a double separator in the middle.
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return ""
	}
	if c := out[0]; !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_') {
		out = "t_" + out
	}
	if len(out) > maxToolNameLen {
		// Truncation alone would collide between two long names sharing a
		// prefix, and a collision silently drops the second tool. The digest
		// is of the original, so the mapping stays stable across restarts.
		sum := sha256.Sum256([]byte(name))
		out = out[:maxToolNameLen-9] + "_" + hex.EncodeToString(sum[:])[:8]
	}
	return out
}
