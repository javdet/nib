package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/javdet/nib/internal/repository"
)

var _ repository.ExecutionLeaseRepository = (*ExecutionLeaseRepo)(nil)

// ExecutionLeaseRepo implements ExecutionLeaseRepository on the single-row
// execution_lease table.
type ExecutionLeaseRepo struct {
	pool *pgxpool.Pool
}

func NewExecutionLeaseRepo(pool *pgxpool.Pool) *ExecutionLeaseRepo {
	return &ExecutionLeaseRepo{pool: pool}
}

func (r *ExecutionLeaseRepo) TryAcquireLease(ctx context.Context, rec repository.LeaseRecord) (bool, repository.LeaseRecord, error) {
	tag, err := r.pool.Exec(ctx,
		`INSERT INTO execution_lease (slot, token, owner, holder, expires_at)
		 VALUES (1, $1, $2, $3::jsonb, $4)
		 ON CONFLICT (slot) DO NOTHING`,
		rec.Token, rec.Owner, string(rec.Holder), nullTime(rec.ExpiresAt),
	)
	if err != nil {
		return false, repository.LeaseRecord{}, fmt.Errorf("acquire execution lease: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return true, rec, nil
	}

	held, err := r.GetLease(ctx)
	if errors.Is(err, repository.ErrNotFound) {
		// Released between the insert and the read: refused all the same, and
		// the caller's next attempt finds the slot free. An empty holder says
		// so.
		return false, repository.LeaseRecord{}, nil
	}
	if err != nil {
		return false, repository.LeaseRecord{}, err
	}
	return false, held, nil
}

func (r *ExecutionLeaseRepo) GetLease(ctx context.Context) (repository.LeaseRecord, error) {
	var rec repository.LeaseRecord
	var holder []byte
	var expires *time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT token, owner, holder::text, expires_at FROM execution_lease WHERE slot = 1`,
	).Scan(&rec.Token, &rec.Owner, &holder, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.LeaseRecord{}, repository.ErrNotFound
	}
	if err != nil {
		return repository.LeaseRecord{}, fmt.Errorf("get execution lease: %w", err)
	}
	rec.Holder = holder
	if expires != nil {
		rec.ExpiresAt = *expires
	}
	return rec, nil
}

func (r *ExecutionLeaseRepo) UpdateLeaseHolder(ctx context.Context, token uuid.UUID, holder []byte) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE execution_lease SET holder = $2::jsonb WHERE slot = 1 AND token = $1`,
		token, string(holder),
	)
	if err != nil {
		return false, fmt.Errorf("update execution lease: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *ExecutionLeaseRepo) ExtendLease(ctx context.Context, token uuid.UUID, expiresAt time.Time) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE execution_lease SET expires_at = $2 WHERE slot = 1 AND token = $1`,
		token, nullTime(expiresAt),
	)
	if err != nil {
		return false, fmt.Errorf("extend execution lease: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *ExecutionLeaseRepo) DeleteLease(ctx context.Context, token uuid.UUID) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM execution_lease WHERE slot = 1 AND token = $1`, token)
	if err != nil {
		return false, fmt.Errorf("release execution lease: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
