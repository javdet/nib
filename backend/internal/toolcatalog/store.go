// Package toolcatalog provides Postgres persistence and full-text search for
// MCP server and tool metadata (categories, tsvector + GIN).
package toolcatalog

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps a pgx pool for the tool catalog tables.
type Store struct {
	pool *pgxpool.Pool
}

// New opens a connection pool to the catalog database.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("toolcatalog: connect: %w", err)
	}
	return &Store{pool: pool}, nil
}

// NewWithPool returns a Store that uses an existing pool (does not take ownership).
func NewWithPool(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Close releases the pool.
func (s *Store) Close() {
	s.pool.Close()
}

// Begin starts a database transaction.
func (s *Store) Begin(ctx context.Context) (pgx.Tx, error) {
	return s.pool.Begin(ctx)
}
