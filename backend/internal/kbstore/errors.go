package kbstore

import "errors"

var (
	// ErrCollectionMismatch is returned when a collection name already exists but
	// dimensions or embedding_model differ from the requested values.
	ErrCollectionMismatch = errors.New("kbstore: collection exists with different dimensions or embedding model")

	// ErrDimensionMismatch is returned when a vector length does not match the collection's dimensions.
	ErrDimensionMismatch = errors.New("kbstore: embedding vector length does not match collection dimensions")

	// ErrUnsupportedDimensions is returned when a collection is asked for at a
	// width the chunk table cannot store.
	ErrUnsupportedDimensions = errors.New("kbstore: unsupported embedding dimensions")
)

// ChunkEmbeddingDimensions is the width of kb_chunks.embedding. The column is
// vector(1536) and the HNSW index spans exactly that width, so a collection
// cannot be created at any other size -- an insert would fail row by row while
// the collection row stayed committed and unrepairable.
const ChunkEmbeddingDimensions = 1536
