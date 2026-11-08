package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/javdet/nib/internal/textutil"
)

var _ repository.CommandRunRepository = (*CommandRunRepo)(nil)

// maxStoredScriptBytes caps what one row keeps of a script. A model can pass a
// heredoc of any size, and the audit needs the gist, not a second copy of it.
const maxStoredScriptBytes = 64 << 10

// CommandRunRepo implements CommandRunRepository using PostgreSQL.
type CommandRunRepo struct {
	pool *pgxpool.Pool
}

func NewCommandRunRepo(pool *pgxpool.Pool) *CommandRunRepo {
	return &CommandRunRepo{pool: pool}
}

const commandRunColumns = `id, plan_id, dialog_id, action_key, mode, kind, script, isolated, uid,
	status, exit_code, reason, output_bytes, output_capped, started_at, finished_at, duration_ms`

func (r *CommandRunRepo) InsertCommandRun(ctx context.Context, run domain.CommandRun) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO command_runs (`+commandRunColumns+`)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		run.ID, nullUUID(run.PlanID), nullUUID(run.DialogID), run.ActionKey, run.Mode, string(run.Kind),
		storedScript(run.Script), run.Isolated, run.UID, string(run.Status), run.ExitCode,
		textutil.Sanitize(run.Reason), run.OutputBytes, run.OutputCapped, run.StartedAt, run.FinishedAt, run.DurationMS)
	if err != nil {
		return fmt.Errorf("insert command run: %w", err)
	}
	return nil
}

func (r *CommandRunRepo) FinishCommandRun(ctx context.Context, run domain.CommandRun) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE command_runs
		    SET status = $2, exit_code = $3, reason = $4, output_bytes = $5,
		        output_capped = $6, finished_at = $7, duration_ms = $8
		  WHERE id = $1`,
		run.ID, string(run.Status), run.ExitCode, textutil.Sanitize(run.Reason), run.OutputBytes,
		run.OutputCapped, run.FinishedAt, run.DurationMS)
	if err != nil {
		return fmt.Errorf("finish command run: %w", err)
	}
	return nil
}

func (r *CommandRunRepo) ListCommandRuns(ctx context.Context, planID uuid.UUID, actionKey string, limit int) ([]domain.CommandRun, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+commandRunColumns+`
		   FROM command_runs
		  WHERE plan_id = $1 AND ($2 = '' OR action_key = $2)
		  ORDER BY started_at DESC
		  LIMIT $3`, planID, actionKey, limit)
	if err != nil {
		return nil, fmt.Errorf("list command runs: %w", err)
	}
	defer rows.Close()
	var out []domain.CommandRun
	for rows.Next() {
		run, err := scanCommandRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan command run: %w", err)
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list command runs: %w", err)
	}
	return out, nil
}

func (r *CommandRunRepo) GetCommandRun(ctx context.Context, id uuid.UUID) (domain.CommandRun, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+commandRunColumns+` FROM command_runs WHERE id = $1`, id)
	run, err := scanCommandRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CommandRun{}, repository.ErrNotFound
	}
	if err != nil {
		return domain.CommandRun{}, fmt.Errorf("get command run: %w", err)
	}
	return run, nil
}

func (r *CommandRunRepo) MarkRunningCommandRunsLost(ctx context.Context, reason string) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE command_runs
		    SET status = 'lost', reason = $1, finished_at = now(),
		        duration_ms = (extract(epoch FROM now() - started_at) * 1000)::bigint
		  WHERE status = 'running'`, reason)
	if err != nil {
		return 0, fmt.Errorf("mark command runs lost: %w", err)
	}
	return tag.RowsAffected(), nil
}

func scanCommandRun(row pgx.Row) (domain.CommandRun, error) {
	var (
		run              domain.CommandRun
		planID, dialogID *uuid.UUID
		kind, status     string
	)
	err := row.Scan(&run.ID, &planID, &dialogID, &run.ActionKey, &run.Mode, &kind, &run.Script,
		&run.Isolated, &run.UID, &status, &run.ExitCode, &run.Reason, &run.OutputBytes,
		&run.OutputCapped, &run.StartedAt, &run.FinishedAt, &run.DurationMS)
	if err != nil {
		return domain.CommandRun{}, err
	}
	if planID != nil {
		run.PlanID = *planID
	}
	if dialogID != nil {
		run.DialogID = *dialogID
	}
	run.Kind = domain.CommandRunKind(kind)
	run.Status = domain.CommandRunStatus(status)
	return run, nil
}

func nullUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func storedScript(script string) string {
	return textutil.TruncateBytes(textutil.Sanitize(strings.TrimSpace(script)), "\n... [truncated]", maxStoredScriptBytes)
}
