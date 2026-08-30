package defaults

import "embed"

// FS holds the knowledge base skeleton compiled into the binary. Unlike skills,
// it is never seeded onto the data volume: it is served as-is whenever a
// collection has no uploaded document, so a template improved in a later image
// reaches every install without touching operator content.
//
//go:embed skeleton.md
var FS embed.FS
