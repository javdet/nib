// Package kb provides Postgres access for knowledge-base collections and chunks
// using pgx and pgvector (cosine distance search).
//
// It is the single data layer for the kb_collections and kb_chunks tables, shared
// by the backend (which passes in the process-wide pool via [New]) and the kb CLI
// (which owns its own pool via [Open]).
package kb

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
)

// Store wraps a pgx pool configured for pgvector types.
type Store struct {
	pool *pgxpool.Pool

	// owned is true when Open built the pool, so Close is the Store's to call.
	// A pool handed to New belongs to the caller and outlives this Store.
	owned bool
}

// New returns a Store that uses the given pool. The caller is responsible for
// registering pgvector types on it (see pgxvec.RegisterTypes) and for closing it.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Open builds a pool from dsn, registers pgvector types on every connection, and
// returns a Store that owns it. Callers must Close it.
func Open(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("kb: parse dsn: %w", err)
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvec.RegisterTypes(ctx, conn)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("kb: connect: %w", err)
	}
	return &Store{pool: pool, owned: true}, nil
}

// Close releases the pool when this Store opened it, and is a no-op otherwise —
// a pool passed to New is the caller's to close.
func (s *Store) Close() {
	if s.owned {
		s.pool.Close()
	}
}

// Begin starts a database transaction. Pass the returned tx to the *Tx variants
// to group deletes and inserts into one atomic ingest.
func (s *Store) Begin(ctx context.Context) (pgx.Tx, error) {
	return s.pool.Begin(ctx)
}
