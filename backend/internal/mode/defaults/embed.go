package defaults

import "embed"

// FS holds the per-mode system tool allow lists compiled into the binary, one
// {mode}.json per mode. They are reconciled into the tools directory at startup:
// a list absent from the data volume is written, and a tool added by a later
// release is appended to a list already there. Operator edits are never undone.
//
//go:embed *.json
var FS embed.FS
