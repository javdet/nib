package kb

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Status returns collection metadata, chunk count, and the source_uri of the most
// recently inserted chunk (empty when the collection has no chunks).
func (s *Store) Status(ctx context.Context, collectionName string) (Status, error) {
	if collectionName == "" {
		collectionName = DefaultCollectionName
	}

	coll, err := s.GetCollectionByName(ctx, collectionName)
	if errors.Is(err, pgx.ErrNoRows) {
		return Status{
			Collection: Collection{Name: collectionName},
		}, nil
	}
	if err != nil {
		return Status{}, err
	}

	const countQ = `SELECT COUNT(*)::int FROM kb_chunks WHERE collection_id = $1`
	var chunkCount int
	if err := s.pool.QueryRow(ctx, countQ, coll.ID).Scan(&chunkCount); err != nil {
		return Status{}, fmt.Errorf("kb: count chunks: %w", err)
	}

	var sourceURI string
	if chunkCount > 0 {
		const sourceQ = `
SELECT source_uri
FROM kb_chunks
WHERE collection_id = $1
ORDER BY created_at DESC
LIMIT 1`
		if err := s.pool.QueryRow(ctx, sourceQ, coll.ID).Scan(&sourceURI); err != nil {
			return Status{}, fmt.Errorf("kb: latest source_uri: %w", err)
		}
	}

	return Status{
		Collection: coll,
		ChunkCount: chunkCount,
		SourceURI:  sourceURI,
	}, nil
}
