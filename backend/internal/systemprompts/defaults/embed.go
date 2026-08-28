package defaults

import "embed"

// FS holds the system prompts compiled into the binary: one {mode}.md per entry
// in mode.Modes. These ship with the image and are the source of truth. The only
// runtime-editable prompt is discuss, whose edits live in a separate override
// file on the data volume.
//
//go:embed *.md
var FS embed.FS
