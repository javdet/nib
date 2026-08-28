package kbstore

import "errors"

var (
	// ErrCollectionMismatch is returned when a collection name already exists but
	// dimensions or embedding_model differ from the requested values.
	ErrCollectionMismatch = errors.New("kbstore: collection exists with different dimensions or embedding model")

	// ErrDimensionMismatch is returned when a vector length does not match the collection's dimensions.
	ErrDimensionMismatch = errors.New("kbstore: embedding vector length does not match collection dimensions")
)
