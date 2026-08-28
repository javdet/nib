package mcpconfig

import "errors"

// ErrStdioNotSupported is returned when tool discovery is requested for a server without an HTTP URL.
var ErrStdioNotSupported = errors.New("Tool discovery requires an HTTP URL; stdio/command servers are not supported in the UI.")
