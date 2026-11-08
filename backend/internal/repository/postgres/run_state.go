package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/javdet/nib/internal/repository"
	"github.com/javdet/nib/internal/textutil"
)

var _ repository.RunStateRepository = (*RunStateRepo)(nil)

// RunStateRepo implements RunStateRepository using PostgreSQL.
type RunStateRepo struct {
	pool *pgxpool.Pool
}

func NewRunStateRepo(pool *pgxpool.Pool) *RunStateRepo {
	return &RunStateRepo{pool: pool}
}

func (r *RunStateRepo) GetRunState(ctx context.Context, kind repository.RunStateKind, dialogID uuid.UUID) ([]byte, error) {
	var data []byte
	err := r.pool.QueryRow(ctx,
		`SELECT data::text FROM run_state WHERE kind = $1 AND dialog_id = $2`,
		string(kind), dialogID,
	).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get run state %s: %w", kind, err)
	}
	return data, nil
}

func (r *RunStateRepo) GetRunStates(ctx context.Context, kind repository.RunStateKind, dialogIDs []uuid.UUID) (map[uuid.UUID][]byte, error) {
	out := make(map[uuid.UUID][]byte, len(dialogIDs))
	if len(dialogIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT dialog_id, data::text FROM run_state WHERE kind = $1 AND dialog_id = ANY($2)`,
		string(kind), dialogIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("get run states %s: %w", kind, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			return nil, fmt.Errorf("scan run state %s: %w", kind, err)
		}
		out[id] = data
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate run states %s: %w", kind, err)
	}
	return out, nil
}

func (r *RunStateRepo) PutRunState(ctx context.Context, kind repository.RunStateKind, dialogID uuid.UUID, data []byte) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO run_state (kind, dialog_id, data, updated_at)
		 VALUES ($1, $2, $3::jsonb, now())
		 ON CONFLICT (kind, dialog_id) DO UPDATE SET data = EXCLUDED.data, updated_at = now()`,
		string(kind), dialogID, string(textutil.SanitizeJSON(data)),
	)
	if err != nil {
		return fmt.Errorf("put run state %s: %w", kind, dialogMissing(err))
	}
	return nil
}

func (r *RunStateRepo) InsertRunState(ctx context.Context, kind repository.RunStateKind, dialogID uuid.UUID, data []byte) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`INSERT INTO run_state (kind, dialog_id, data)
		 VALUES ($1, $2, $3::jsonb)
		 ON CONFLICT (kind, dialog_id) DO NOTHING`,
		string(kind), dialogID, string(textutil.SanitizeJSON(data)),
	)
	if err != nil {
		return false, fmt.Errorf("insert run state %s: %w", kind, dialogMissing(err))
	}
	return tag.RowsAffected() == 1, nil
}

func (r *RunStateRepo) DeleteRunState(ctx context.Context, kind repository.RunStateKind, dialogID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx,
		`DELETE FROM run_state WHERE kind = $1 AND dialog_id = $2`,
		string(kind), dialogID,
	); err != nil {
		return fmt.Errorf("delete run state %s: %w", kind, err)
	}
	return nil
}

func (r *RunStateRepo) ListRunStates(ctx context.Context, kind repository.RunStateKind) ([]repository.RunStateEntry, error) {
	return r.list(ctx, `SELECT dialog_id, data::text FROM run_state WHERE kind = $1 ORDER BY dialog_id`, kind)
}

// ListRunningRunStates leaves the filtering of every finished run to the
// server: the sweep runs every minute on every process, and almost every
// document it would otherwise fetch and decode is long finished.
func (r *RunStateRepo) ListRunningRunStates(ctx context.Context, kind repository.RunStateKind) ([]repository.RunStateEntry, error) {
	return r.list(ctx, `SELECT dialog_id, data::text FROM run_state
		 WHERE kind = $1 AND jsonb_path_exists(data, '$.** ? (@ == "running")')
		 ORDER BY dialog_id`, kind)
}

func (r *RunStateRepo) list(ctx context.Context, query string, kind repository.RunStateKind) ([]repository.RunStateEntry, error) {
	rows, err := r.pool.Query(ctx, query, string(kind))
	if err != nil {
		return nil, fmt.Errorf("list run states %s: %w", kind, err)
	}
	defer rows.Close()

	var out []repository.RunStateEntry
	for rows.Next() {
		var e repository.RunStateEntry
		if err := rows.Scan(&e.DialogID, &e.Data); err != nil {
			return nil, fmt.Errorf("scan run state %s: %w", kind, err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate run states %s: %w", kind, err)
	}
	return out, nil
}

// dialogMissing turns the foreign-key violation of a document whose dialog is
// gone into ErrNotFound, which is what a caller can act on.
func dialogMissing(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return fmt.Errorf("dialog does not exist: %w", repository.ErrNotFound)
	}
	return err
}
