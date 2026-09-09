package kb

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// DeleteChunksBySourceURIs removes kb_chunks rows for the given collection and
// source_uri values. Empty uris is a no-op.
func (s *Store) DeleteChunksBySourceURIs(ctx context.Context, collectionID uuid.UUID, uris []string) error {
	return s.deleteChunksBySourceURIs(ctx, s.pool, collectionID, uris)
}

// DeleteChunksBySourceURIsTx is like [Store.DeleteChunksBySourceURIs] but runs on tx.
func (s *Store) DeleteChunksBySourceURIsTx(ctx context.Context, tx pgx.Tx, collectionID uuid.UUID, uris []string) error {
	return s.deleteChunksBySourceURIs(ctx, tx, collectionID, uris)
}

func (s *Store) deleteChunksBySourceURIs(ctx context.Context, e execer, collectionID uuid.UUID, uris []string) error {
	if len(uris) == 0 {
		return nil
	}
	const q = `DELETE FROM kb_chunks WHERE collection_id = $1 AND source_uri = ANY($2)`
	if _, err := e.Exec(ctx, q, collectionID, uris); err != nil {
		return fmt.Errorf("kb: delete chunks: %w", err)
	}
	return nil
}
