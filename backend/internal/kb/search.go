package kb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
)

// SearchHit is one ranked result from cosine similarity search.
// Score is higher for more similar chunks: 1 minus cosine distance (see pgvector <=>).
type SearchHit struct {
	ChunkID   uuid.UUID
	Content   string
	SourceURI string
	Metadata  json.RawMessage
	Score     float64
}

// SearchCosine returns the nearest chunks by cosine distance (pgvector <=> with vector_cosine_ops).
// Score is 1 minus cosine distance so larger values mean closer matches.
func (s *Store) SearchCosine(ctx context.Context, collectionID uuid.UUID, queryEmbedding []float32, limit int) ([]SearchHit, error) {
	if limit < 1 {
		return nil, fmt.Errorf("kb: limit must be at least 1")
	}

	col, err := s.GetCollection(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	if len(queryEmbedding) != col.Dimensions {
		return nil, fmt.Errorf("%w: query has %d, collection has %d", ErrDimensionMismatch, len(queryEmbedding), col.Dimensions)
	}

	qv := pgvector.NewVector(queryEmbedding)

	const sql = `
SELECT id, content, source_uri, metadata,
       1::float8 - (embedding <=> $1::vector) AS score
FROM kb_chunks
WHERE collection_id = $2
ORDER BY embedding <=> $1::vector
LIMIT $3`

	rows, err := s.pool.Query(ctx, sql, qv, collectionID, limit)
	if err != nil {
		return nil, fmt.Errorf("kb: search: %w", err)
	}
	defer rows.Close()

	var out []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.ChunkID, &h.Content, &h.SourceURI, &h.Metadata, &h.Score); err != nil {
			return nil, fmt.Errorf("kb: scan search row: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("kb: search rows: %w", err)
	}
	return out, nil
}
