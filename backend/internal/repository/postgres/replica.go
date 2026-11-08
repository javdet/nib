package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/javdet/nib/internal/repository"
)

var _ repository.ReplicaRepository = (*ReplicaRepo)(nil)

// ReplicaRepo implements ReplicaRepository on nib_replicas. Heartbeats are
// compared with the database's clock, never the process's, so two hosts whose
// clocks disagree still agree on who is alive.
type ReplicaRepo struct {
	pool *pgxpool.Pool
}

func NewReplicaRepo(pool *pgxpool.Pool) *ReplicaRepo {
	return &ReplicaRepo{pool: pool}
}

func (r *ReplicaRepo) RegisterReplica(ctx context.Context, rep repository.Replica) error {
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO nib_replicas (id, hostname, version, started_at, heartbeat_at)
		 VALUES ($1, $2, $3, now(), now())
		 ON CONFLICT (id) DO UPDATE SET heartbeat_at = now()`,
		rep.ID, rep.Hostname, rep.Version,
	); err != nil {
		return fmt.Errorf("register replica: %w", err)
	}
	return nil
}

func (r *ReplicaRepo) HeartbeatReplica(ctx context.Context, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE nib_replicas SET heartbeat_at = now() WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("replica heartbeat: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *ReplicaRepo) DeregisterReplica(ctx context.Context, id string) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM nib_replicas WHERE id = $1`, id); err != nil {
		return fmt.Errorf("deregister replica: %w", err)
	}
	return nil
}

func (r *ReplicaRepo) RetireReplicasOnHost(ctx context.Context, hostname, keep string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`DELETE FROM nib_replicas WHERE hostname = $1 AND id <> $2 RETURNING id`, hostname, keep)
	if err != nil {
		return nil, fmt.Errorf("retire replicas on %s: %w", hostname, err)
	}
	return collectIDs(rows)
}

func (r *ReplicaRepo) ListLiveReplicas(ctx context.Context, maxAge time.Duration) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id FROM nib_replicas WHERE heartbeat_at > now() - $1::interval ORDER BY id`,
		interval(maxAge),
	)
	if err != nil {
		return nil, fmt.Errorf("list live replicas: %w", err)
	}
	return collectIDs(rows)
}

func (r *ReplicaRepo) DeleteStaleReplicas(ctx context.Context, maxAge time.Duration) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`DELETE FROM nib_replicas WHERE heartbeat_at <= now() - $1::interval RETURNING id`,
		interval(maxAge),
	)
	if err != nil {
		return nil, fmt.Errorf("delete stale replicas: %w", err)
	}
	return collectIDs(rows)
}

// interval renders d as a Postgres interval literal.
func interval(d time.Duration) string {
	return fmt.Sprintf("%d milliseconds", max(d, 0).Milliseconds())
}

func collectIDs(rows pgx.Rows) ([]string, error) {
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("scan replica ids: %w", err)
	}
	return ids, nil
}
