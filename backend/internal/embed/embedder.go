// Package embed provides thin net/http clients for text embedding APIs (OpenRouter
// and Google Gemini) behind a shared Embedder interface.
package embed

import (
	"context"
)

// Embedder produces vector embeddings for one or more input strings in a single
// provider call. Implementations must return one vector per input, in order;
// dimension is the length of each vector (all rows share the same dimension).
type Embedder interface {
	Embed(ctx context.Context, texts []string) (vectors [][]float32, dimension int, err error)
}
