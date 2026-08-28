package llm

import "context"

// Embedder produces vector embeddings for one or more input strings in a single API call.
type Embedder interface {
	Embed(ctx context.Context, texts []string) (vectors [][]float32, dimension int, err error)
}
