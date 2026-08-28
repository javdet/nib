package kbstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

type batchConn interface {
	SendBatch(context.Context, *pgx.Batch) pgx.BatchResults
}

// InsertChunks inserts rows into kb_chunks in a single batch. Embeddings must match
// the collection's dimensions.
func (s *Store) InsertChunks(ctx context.Context, collectionID uuid.UUID, chunks []ChunkInput) error {
	return s.insertChunks(ctx, s.pool, collectionID, chunks)
}

// InsertChunksTx is like [InsertChunks] but sends the batch on tx (e.g. after deletes
// in the same transaction).
func (s *Store) InsertChunksTx(ctx context.Context, tx pgx.Tx, collectionID uuid.UUID, chunks []ChunkInput) error {
	return s.insertChunks(ctx, tx, collectionID, chunks)
}

func (s *Store) insertChunks(ctx context.Context, conn batchConn, collectionID uuid.UUID, chunks []ChunkInput) error {
	if len(chunks) == 0 {
		return nil
	}

	col, err := s.GetCollection(ctx, collectionID)
	if err != nil {
		return err
	}

	for i := range chunks {
		if len(chunks[i].Embedding) != col.Dimensions {
			return fmt.Errorf("%w: chunk %d: got %d, want %d", ErrDimensionMismatch, i, len(chunks[i].Embedding), col.Dimensions)
		}
	}

	batch := &pgx.Batch{}
	const stmt = `
INSERT INTO kb_chunks (collection_id, source_uri, chunk_index, content, metadata, embedding)
VALUES ($1, $2, $3, $4, COALESCE($5::jsonb, '{}'::jsonb), $6)`

	for _, ch := range chunks {
		meta := ch.Metadata
		if len(meta) == 0 {
			meta = []byte(`{}`)
		}
		vec := pgvector.NewVector(ch.Embedding)
		batch.Queue(stmt, collectionID, ch.SourceURI, ch.ChunkIndex, ch.Content, meta, vec)
	}

	br := conn.SendBatch(ctx, batch)
	defer br.Close()

	for range chunks {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("kbstore: insert chunk: %w", err)
		}
	}
	return br.Close()
}
