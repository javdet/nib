// Package kb provides Postgres access for knowledge-base collections and chunks
// using pgx and pgvector.
package kb

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps a pgx pool configured for pgvector types (see pgxvec.RegisterTypes in main).
type Store struct {
	pool *pgxpool.Pool
}

// New returns a Store that uses the given pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}
