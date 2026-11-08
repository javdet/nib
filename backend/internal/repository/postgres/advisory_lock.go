package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// advisoryKeyPrefix namespaces every lock name before it is hashed to the
// 64-bit key, so nib's locks stay apart from the migration runner's and from
// anything else sharing the database. A 64-bit hash, not hashtext's 32 bits:
// two names held at once by one goroutine that collided would sit on two
// sessions and wedge it for good.
const advisoryKeyPrefix = "nib-lock:"

// connWait bounds the wait for a connection to lock on. A lock holder may take
// a second lock -- a transcript held across a round whose tool writes the plan
// -- so with every connection held by such holders, waiting without a bound
// would deadlock them all. Past it the caller is told, and falls back to its
// in-process lock.
const connWait = 5 * time.Second

// unlockTimeout bounds the release of a lock. A release that cannot reach the
// server drops the connection instead, which releases the lock just the same.
const unlockTimeout = 5 * time.Second

// AdvisoryLocker takes session-level advisory locks, each on a connection of
// its own held for as long as the lock is. A process that dies loses its
// sessions, so a crash can never leave a lock held.
//
// It wants a pool of its own: a lock held across an agent round pins its
// connection for minutes, and taking those from the main pool would starve
// ordinary queries.
type AdvisoryLocker struct {
	pool *pgxpool.Pool
}

func NewAdvisoryLocker(pool *pgxpool.Pool) *AdvisoryLocker {
	return &AdvisoryLocker{pool: pool}
}

func (l *AdvisoryLocker) Acquire(ctx context.Context, key string) (func(), error) {
	conn, err := l.acquireConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire lock connection: %w", err)
	}
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, advisoryKeyPrefix+key); err != nil {
		// A wait cancelled by ctx may leave the statement in flight; the
		// session is discarded rather than handed back with a lock it might
		// still be granted.
		discard(conn)
		return nil, fmt.Errorf("advisory lock %s: %w", key, err)
	}
	return releaser(conn, key), nil
}

func (l *AdvisoryLocker) TryAcquire(ctx context.Context, key string) (func(), bool, error) {
	conn, err := l.acquireConn(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("acquire lock connection: %w", err)
	}
	var ok bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1, 0))`, advisoryKeyPrefix+key).Scan(&ok); err != nil {
		discard(conn)
		return nil, false, fmt.Errorf("try advisory lock %s: %w", key, err)
	}
	if !ok {
		conn.Release()
		return nil, false, nil
	}
	return releaser(conn, key), true, nil
}

func (l *AdvisoryLocker) acquireConn(ctx context.Context) (*pgxpool.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, connWait)
	defer cancel()
	return l.pool.Acquire(ctx)
}

func releaser(conn *pgxpool.Conn, key string) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), unlockTimeout)
		defer cancel()
		var released bool
		err := conn.QueryRow(ctx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, advisoryKeyPrefix+key).Scan(&released)
		if err != nil || !released {
			slog.Warn("advisory unlock failed, dropping the session instead", "key", key, "error", err)
			discard(conn)
			return
		}
		conn.Release()
	}
}

// discard closes the connection's session before handing it back, so the pool
// destroys it instead of reusing a session that may still hold a lock.
func discard(conn *pgxpool.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), unlockTimeout)
	defer cancel()
	_ = conn.Conn().Close(ctx)
	conn.Release()
}
