package defaults

import "embed"

// FS holds the skills compiled into the binary: one {name}.md per built-in
// skill. They are seeded into the skills directory on the first start that sees
// them and are owned by the operator from then on — the image never rewrites a
// skill file it already seeded, so edits and deletions survive an upgrade.
//
//go:embed *.md
var FS embed.FS
