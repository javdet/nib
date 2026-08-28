package kb

import (
	"encoding/json"

	"github.com/google/uuid"
)

const DefaultCollectionName = "default"

// Collection describes a row in kb_collections.
type Collection struct {
	ID             uuid.UUID
	Name           string
	Dimensions     int
	EmbeddingModel string
	Metric         string
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

// Status summarizes ingest state for a named collection.
type Status struct {
	Collection Collection
	ChunkCount int
	SourceURI  string
}

// CollectionSummary is a collection plus its ingest state.
type CollectionSummary struct {
	Collection
	ChunkCount int
	SourceURI  string
}
