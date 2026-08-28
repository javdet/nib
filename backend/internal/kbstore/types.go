package kbstore

import (
	"encoding/json"

	"github.com/google/uuid"
)

// Collection describes a row in kb_collections.
type Collection struct {
	ID              uuid.UUID
	Name            string
	Dimensions      int
	EmbeddingModel  string
	Metric          string
}

// ChunkInput is one row to insert into kb_chunks. Embedding must match the
// collection's dimensions. Metadata may be nil for an empty JSON object.
type ChunkInput struct {
	SourceURI  string
	ChunkIndex int
	Content    string
	Metadata   json.RawMessage
	Embedding  []float32
}

// SearchHit is one ranked result from cosine similarity search.
// Score is higher for more similar chunks: 1 minus cosine distance (see pgvector <=>).
type SearchHit struct {
	ChunkID   uuid.UUID
	Content   string
	SourceURI string
	Metadata  json.RawMessage
	Score     float64
}
