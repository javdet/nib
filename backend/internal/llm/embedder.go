package llm

import "github.com/javdet/nib/internal/embed"

// Embedder produces vector embeddings for one or more input strings in a single
// API call. It is an alias for [embed.Embedder] so the backend's configured
// provider and the standalone clients in that package are one interface: the kb
// CLI and the agent must agree on this shape or they cannot read each other's
// collections.
type Embedder = embed.Embedder
