package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// defaultCoordinationPoolSize bounds the connections advisory locks and the
// bus listener hold at once. A lock pins its connection while held, and the
// longest-held ones (a transcript across an agent round, a launch claim
// across a decompose turn) number about one per agent running, so this is a
// few dozen at the busiest, per process. Every process adds this much to the
// server's max_connections budget.
const defaultCoordinationPoolSize = 24

// newCoordinationPool is a second pool on the same database, for connections
// held for as long as a lock is or the process lives. NIB_COORDINATION_POOL_SIZE
// overrides its size.
func newCoordinationPool(ctx context.Context, base *pgxpool.Config) (*pgxpool.Pool, error) {
	cfg := base.Copy()
	cfg.MaxConns = int32(coordinationPoolSize())
	cfg.MinConns = 0
	cfg.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect coordination pool: %w", err)
	}
	return pool, nil
}

func coordinationPoolSize() int {
	raw := strings.TrimSpace(os.Getenv("NIB_COORDINATION_POOL_SIZE"))
	if raw == "" {
		return defaultCoordinationPoolSize
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 2 {
		slog.Warn("NIB_COORDINATION_POOL_SIZE is not a number of at least 2, using the default",
			"value", raw, "default", defaultCoordinationPoolSize)
		return defaultCoordinationPoolSize
	}
	return n
}
