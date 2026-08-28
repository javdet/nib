package kbstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// UpsertCollection inserts a collection or returns the existing row when name matches
// and dimensions and embedding_model are identical. If the name exists with different
// dimensions or embedding_model, it returns ErrCollectionMismatch.
func (s *Store) UpsertCollection(ctx context.Context, name string, dimensions int, embeddingModel string, metric string) (Collection, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Collection{}, fmt.Errorf("kbstore: collection name is required")
	}
	if dimensions < 1 {
		return Collection{}, fmt.Errorf("kbstore: dimensions must be positive")
	}
	embeddingModel = strings.TrimSpace(embeddingModel)
	if embeddingModel == "" {
		return Collection{}, fmt.Errorf("kbstore: embedding_model is required")
	}
	metric = strings.TrimSpace(metric)
	if metric == "" {
		metric = "cosine"
	}

	const sel = `
SELECT id, name, dimensions, embedding_model, metric
FROM kb_collections
WHERE name = $1`

	var existing Collection
	err := s.pool.QueryRow(ctx, sel, name).Scan(
		&existing.ID,
		&existing.Name,
		&existing.Dimensions,
		&existing.EmbeddingModel,
		&existing.Metric,
	)
	if err == nil {
		if existing.Dimensions != dimensions || existing.EmbeddingModel != embeddingModel {
			return Collection{}, ErrCollectionMismatch
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Collection{}, fmt.Errorf("kbstore: select collection: %w", err)
	}

	const ins = `
INSERT INTO kb_collections (name, dimensions, embedding_model, metric)
VALUES ($1, $2, $3, $4)
RETURNING id, name, dimensions, embedding_model, metric`

	var created Collection
	err = s.pool.QueryRow(ctx, ins, name, dimensions, embeddingModel, metric).Scan(
		&created.ID,
		&created.Name,
		&created.Dimensions,
		&created.EmbeddingModel,
		&created.Metric,
	)
	if err != nil {
		return Collection{}, fmt.Errorf("kbstore: insert collection: %w", err)
	}
	return created, nil
}

// GetCollectionByName loads a collection by unique name.
func (s *Store) GetCollectionByName(ctx context.Context, name string) (Collection, error) {
	const q = `
SELECT id, name, dimensions, embedding_model, metric
FROM kb_collections
WHERE name = $1`

	var c Collection
	err := s.pool.QueryRow(ctx, q, strings.TrimSpace(name)).Scan(
		&c.ID, &c.Name, &c.Dimensions, &c.EmbeddingModel, &c.Metric,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Collection{}, fmt.Errorf("kbstore: collection %q: %w", name, pgx.ErrNoRows)
	}
	if err != nil {
		return Collection{}, fmt.Errorf("kbstore: get collection: %w", err)
	}
	return c, nil
}

// GetCollection loads a collection by id.
func (s *Store) GetCollection(ctx context.Context, id uuid.UUID) (Collection, error) {
	const q = `
SELECT id, name, dimensions, embedding_model, metric
FROM kb_collections
WHERE id = $1`

	var c Collection
	err := s.pool.QueryRow(ctx, q, id).Scan(
		&c.ID, &c.Name, &c.Dimensions, &c.EmbeddingModel, &c.Metric,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Collection{}, fmt.Errorf("kbstore: collection %s: %w", id, pgx.ErrNoRows)
	}
	if err != nil {
		return Collection{}, fmt.Errorf("kbstore: get collection: %w", err)
	}
	return c, nil
}
