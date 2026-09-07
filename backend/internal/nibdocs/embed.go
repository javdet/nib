package nibdocs

import "embed"

// docsFS carries the documentation shipped inside the image so the agent can
// answer questions about nib itself. docs/repo-guide.md is a copy of the
// repository-root CLAUDE.md kept byte-identical by `make sync-docs` and guarded
// by TestRepoGuideMatchesRepoRoot -- edit the root file, never this copy.
//
// The documents are embedded rather than written to the data volume on purpose:
// nothing in the backend serves files from disk, and no route exposes these, so
// they ship with the image without becoming reachable over HTTP.
//
//go:embed docs/*.md
var docsFS embed.FS
