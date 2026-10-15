package mcpconfig

import "errors"

// ErrStdioNotSupported is returned when a server without an HTTP URL is saved
// through the dialog or has its tools listed: mcpclient speaks streamable HTTP
// only, so a command-only entry would be stored and then never contacted.
var ErrStdioNotSupported = errors.New("MCP servers need an HTTP URL; stdio (command) servers are not supported")

// ErrInvalidHeader is returned when a header, after ${NAME} substitution, is
// one net/http would refuse to send. The message names the header and its
// unexpanded template, never the substituted value.
var ErrInvalidHeader = errors.New("invalid MCP header")

// ErrDiscoveryFailed wraps the transport error from contacting a configured MCP
// server. It exists so the handler can report the cause instead of a bare 500:
// the wrapped message has already had every substituted secret redacted.
var ErrDiscoveryFailed = errors.New("mcp server unreachable")
