package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
)

var _ repository.StatsRepository = (*StatsRepo)(nil)

// StatsRepo implements StatsRepository using PostgreSQL.
//
// This is deliberately a thin scan layer: there is no database harness in the
// test suite, so every decision that can be tested (range and bucket
// validation, top-N folding, nil-cost handling) lives in the service instead.
type StatsRepo struct {
	pool *pgxpool.Pool
}

func NewStatsRepo(pool *pgxpool.Pool) *StatsRepo {
	return &StatsRepo{pool: pool}
}

func (r *StatsRepo) InsertLLMUsage(ctx context.Context, rec domain.LLMUsageRecord) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO llm_usage (
			dialog_id, plan_id, mode, operation, model,
			prompt_tokens, cached_prompt_tokens, completion_tokens,
			reasoning_tokens, total_tokens, cost_usd, duration_ms, outcome
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		rec.DialogID, rec.PlanID, rec.Mode, rec.Operation, rec.Model,
		rec.PromptTokens, rec.CachedPromptTokens, rec.CompletionTokens,
		rec.ReasoningTokens, rec.TotalTokens, rec.CostUSD, rec.DurationMS, rec.Outcome)
	if err != nil {
		return fmt.Errorf("insert llm usage: %w", err)
	}
	return nil
}

func (r *StatsRepo) InsertAgentRunUsage(ctx context.Context, rec domain.AgentRunUsageRecord) error {
	// ON CONFLICT DO NOTHING against the partial unique index on job_name: the
	// container retries its webhook, and a retry must not double the cost.
	_, err := r.pool.Exec(ctx,
		`INSERT INTO agent_run_usage (
			dialog_id, plan_id, job_name, session_id, status,
			input_tokens, output_tokens, num_turns, cost_usd, duration_ms
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT DO NOTHING`,
		rec.DialogID, rec.PlanID, rec.JobName, rec.SessionID, rec.Status,
		rec.InputTokens, rec.OutputTokens, rec.NumTurns, rec.CostUSD, rec.DurationMS)
	if err != nil {
		return fmt.Errorf("insert agent run usage: %w", err)
	}
	return nil
}

func (r *StatsRepo) InsertPlanStatusTransition(ctx context.Context, planID, from, to string) error {
	id, err := uuid.Parse(planID)
	if err != nil {
		return fmt.Errorf("insert plan status transition: parse plan id: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO plan_status_transitions (plan_id, from_status, to_status)
		 VALUES ($1, $2, $3)`, id, from, to)
	if err != nil {
		return fmt.Errorf("insert plan status transition: %w", err)
	}
	return nil
}

// bucketExpr truncates to the range's unit in UTC.
//
// The three-argument date_trunc(unit, ts, zone) is PostgreSQL 16+, and
// externalDatabase lets an operator point nib at an older server -- the same
// reason migration 000037 guards uuidv7(). AT TIME ZONE 'UTC' works everywhere.
const bucketExpr = `date_trunc($1, created_at AT TIME ZONE 'UTC')`

// bucketSeries is the gap-filling spine every series query joins against.
//
// Without it a chart draws a straight line across a stretch with no activity,
// which reads as steady spend rather than none.
const bucketSeries = `
	SELECT generate_series(
		date_trunc($1, $2::timestamptz AT TIME ZONE 'UTC'),
		date_trunc($1, $3::timestamptz AT TIME ZONE 'UTC'),
		$4::interval
	) AS bucket`

func (r *StatsRepo) UsageTotals(ctx context.Context, rg domain.StatsRange) (domain.UsageTotals, error) {
	var t domain.UsageTotals
	// sum(cost_usd) is deliberately not coalesced: it is NULL only when every
	// row in the range was unpriced, which is "unknown", not "$0".
	err := r.pool.QueryRow(ctx,
		`SELECT count(*), count(cost_usd),
		        count(*) FILTER (WHERE outcome <> 'success'),
		        coalesce(sum(prompt_tokens), 0), coalesce(sum(cached_prompt_tokens), 0),
		        coalesce(sum(completion_tokens), 0), coalesce(sum(reasoning_tokens), 0),
		        coalesce(sum(total_tokens), 0), sum(cost_usd),
		        coalesce(avg(duration_ms), 0)::int
		   FROM llm_usage
		  WHERE created_at >= $1 AND created_at < $2`, rg.From, rg.To).
		Scan(&t.Calls, &t.CostedCalls, &t.FailedCalls,
			&t.PromptTokens, &t.CachedTokens, &t.OutputTokens, &t.ReasonTokens,
			&t.TotalTokens, &t.CostUSD, &t.AvgDurationMS)
	if err != nil {
		return domain.UsageTotals{}, fmt.Errorf("usage totals: %w", err)
	}
	return t, nil
}

func (r *StatsRepo) UsageSeries(ctx context.Context, rg domain.StatsRange) ([]domain.UsageBucket, error) {
	rows, err := r.pool.Query(ctx,
		`WITH buckets AS (`+bucketSeries+`),
		 agg AS (
			SELECT `+bucketExpr+` AS bucket,
			       count(*) AS calls, count(cost_usd) AS costed,
			       sum(prompt_tokens) AS prompt, sum(cached_prompt_tokens) AS cached,
			       sum(completion_tokens) AS completion, sum(reasoning_tokens) AS reasoning,
			       sum(total_tokens) AS total, sum(cost_usd) AS cost
			  FROM llm_usage
			 WHERE created_at >= $2 AND created_at < $3
			 GROUP BY 1
		 )
		 SELECT b.bucket, coalesce(a.calls, 0), coalesce(a.costed, 0),
		        coalesce(a.prompt, 0), coalesce(a.cached, 0), coalesce(a.completion, 0),
		        coalesce(a.reasoning, 0), coalesce(a.total, 0), a.cost
		   FROM buckets b LEFT JOIN agg a ON a.bucket = b.bucket
		  ORDER BY b.bucket`, rg.Unit, rg.From, rg.To, rg.Step)
	if err != nil {
		return nil, fmt.Errorf("usage series: %w", err)
	}
	defer rows.Close()

	var out []domain.UsageBucket
	for rows.Next() {
		var b domain.UsageBucket
		if err := rows.Scan(&b.TS, &b.Calls, &b.CostedCalls, &b.PromptTokens,
			&b.CachedTokens, &b.CompletionTokens, &b.ReasoningTokens,
			&b.TotalTokens, &b.CostUSD); err != nil {
			return nil, fmt.Errorf("scan usage bucket: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// usageGroupColumns whitelists what UsageByColumn may group by. The column is
// interpolated into the SQL, so it must never come from a request.
var usageGroupColumns = map[string]struct{}{
	"model": {}, "mode": {}, "operation": {},
}

func (r *StatsRepo) UsageByColumn(ctx context.Context, rg domain.StatsRange, column string, limit int) ([]domain.UsageByKey, error) {
	if _, ok := usageGroupColumns[column]; !ok {
		return nil, fmt.Errorf("usage by column: unsupported column %q", column)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT `+column+`, count(*), coalesce(sum(total_tokens), 0), sum(cost_usd)
		   FROM llm_usage
		  WHERE created_at >= $1 AND created_at < $2
		  GROUP BY 1 ORDER BY 2 DESC LIMIT $3`, rg.From, rg.To, limit)
	if err != nil {
		return nil, fmt.Errorf("usage by %s: %w", column, err)
	}
	defer rows.Close()

	var out []domain.UsageByKey
	for rows.Next() {
		var u domain.UsageByKey
		if err := rows.Scan(&u.Key, &u.Calls, &u.TotalTokens, &u.CostUSD); err != nil {
			return nil, fmt.Errorf("scan usage by %s: %w", column, err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *StatsRepo) TopPlansByUsage(ctx context.Context, rg domain.StatsRange, limit int) ([]domain.PlanUsage, error) {
	// LEFT JOIN, not an inner one: llm_usage has no foreign key and outlives
	// the dialog it came from, so a deleted plan must still show its spend.
	rows, err := r.pool.Query(ctx,
		`SELECT u.plan_id, coalesce(d.title, ''), count(*),
		        coalesce(sum(u.total_tokens), 0), sum(u.cost_usd)
		   FROM llm_usage u
		   LEFT JOIN chat_dialogs d ON d.id = u.plan_id
		  WHERE u.created_at >= $1 AND u.created_at < $2 AND u.plan_id IS NOT NULL
		  GROUP BY u.plan_id, d.title
		  ORDER BY coalesce(sum(u.total_tokens), 0) DESC
		  LIMIT $3`, rg.From, rg.To, limit)
	if err != nil {
		return nil, fmt.Errorf("top plans by usage: %w", err)
	}
	defer rows.Close()

	var out []domain.PlanUsage
	for rows.Next() {
		var p domain.PlanUsage
		if err := rows.Scan(&p.PlanID, &p.Title, &p.Calls, &p.TotalTokens, &p.CostUSD); err != nil {
			return nil, fmt.Errorf("scan top plan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *StatsRepo) PlansCreatedSeries(ctx context.Context, rg domain.StatsRange) ([]domain.CountBucket, error) {
	// planDialogPredicate is shared with the dialog repo on purpose: the plan
	// definition already has to agree across it, the plan_dialogs view and the
	// mode CHECK, and a fourth copy here would be a fourth thing to drift.
	rows, err := r.pool.Query(ctx,
		`WITH buckets AS (`+bucketSeries+`),
		 agg AS (
			SELECT `+bucketExpr+` AS bucket, count(*) AS n
			  FROM chat_dialogs
			 WHERE `+planDialogPredicate+` AND created_at >= $2 AND created_at < $3
			 GROUP BY 1
		 )
		 SELECT b.bucket, coalesce(a.n, 0)
		   FROM buckets b LEFT JOIN agg a ON a.bucket = b.bucket
		  ORDER BY b.bucket`, rg.Unit, rg.From, rg.To, rg.Step)
	if err != nil {
		return nil, fmt.Errorf("plans created series: %w", err)
	}
	defer rows.Close()

	var out []domain.CountBucket
	for rows.Next() {
		var b domain.CountBucket
		if err := rows.Scan(&b.TS, &b.Count); err != nil {
			return nil, fmt.Errorf("scan plans created bucket: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *StatsRepo) PlansCreatedInRange(ctx context.Context, rg domain.StatsRange) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM chat_dialogs
		  WHERE `+planDialogPredicate+` AND created_at >= $1 AND created_at < $2`,
		rg.From, rg.To).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("plans created in range: %w", err)
	}
	return n, nil
}

func (r *StatsRepo) PlansTotal(ctx context.Context) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM chat_dialogs WHERE `+planDialogPredicate).Scan(&n); err != nil {
		return 0, fmt.Errorf("plans total: %w", err)
	}
	return n, nil
}

func (r *StatsRepo) PlansByMode(ctx context.Context) ([]domain.CountByKey, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT mode, count(*) FROM chat_dialogs
		  WHERE `+planDialogPredicate+`
		  GROUP BY mode ORDER BY 2 DESC`)
	if err != nil {
		return nil, fmt.Errorf("plans by mode: %w", err)
	}
	defer rows.Close()
	return scanCountByKey(rows, "plans by mode")
}

func (r *StatsRepo) StatusTransitionSeries(ctx context.Context, rg domain.StatsRange) ([]domain.StatusCountBucket, error) {
	// Not gap-filled: the caller pivots this into one series per status, and a
	// missing (bucket, status) pair is a zero there rather than a hole.
	rows, err := r.pool.Query(ctx,
		`SELECT `+bucketExpr+` AS bucket, to_status, count(*)
		   FROM plan_status_transitions
		  WHERE created_at >= $2 AND created_at < $3
		  GROUP BY 1, 2 ORDER BY 1, 2`, rg.Unit, rg.From, rg.To)
	if err != nil {
		return nil, fmt.Errorf("status transition series: %w", err)
	}
	defer rows.Close()

	var out []domain.StatusCountBucket
	for rows.Next() {
		var b domain.StatusCountBucket
		if err := rows.Scan(&b.TS, &b.Status, &b.Count); err != nil {
			return nil, fmt.Errorf("scan status transition bucket: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *StatsRepo) AgentRunTotals(ctx context.Context, rg domain.StatsRange) (domain.AgentRunTotals, error) {
	var t domain.AgentRunTotals
	err := r.pool.QueryRow(ctx,
		`SELECT count(*), count(cost_usd), coalesce(sum(num_turns), 0),
		        coalesce(sum(input_tokens), 0), coalesce(sum(output_tokens), 0),
		        sum(cost_usd), coalesce(avg(duration_ms), 0)::int
		   FROM agent_run_usage
		  WHERE created_at >= $1 AND created_at < $2`, rg.From, rg.To).
		Scan(&t.Runs, &t.CostedRuns, &t.Turns, &t.InputTokens, &t.OutputTokens,
			&t.CostUSD, &t.AvgDurationMS)
	if err != nil {
		return domain.AgentRunTotals{}, fmt.Errorf("agent run totals: %w", err)
	}
	return t, nil
}

func (r *StatsRepo) AgentRunSeries(ctx context.Context, rg domain.StatsRange) ([]domain.AgentRunBucket, error) {
	rows, err := r.pool.Query(ctx,
		`WITH buckets AS (`+bucketSeries+`),
		 agg AS (
			SELECT `+bucketExpr+` AS bucket, count(*) AS runs, sum(cost_usd) AS cost
			  FROM agent_run_usage
			 WHERE created_at >= $2 AND created_at < $3
			 GROUP BY 1
		 )
		 SELECT b.bucket, coalesce(a.runs, 0), a.cost
		   FROM buckets b LEFT JOIN agg a ON a.bucket = b.bucket
		  ORDER BY b.bucket`, rg.Unit, rg.From, rg.To, rg.Step)
	if err != nil {
		return nil, fmt.Errorf("agent run series: %w", err)
	}
	defer rows.Close()

	var out []domain.AgentRunBucket
	for rows.Next() {
		var b domain.AgentRunBucket
		if err := rows.Scan(&b.TS, &b.Runs, &b.CostUSD); err != nil {
			return nil, fmt.Errorf("scan agent run bucket: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *StatsRepo) AgentRunsByStatus(ctx context.Context, rg domain.StatsRange) ([]domain.CountByKey, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT status, count(*) FROM agent_run_usage
		  WHERE created_at >= $1 AND created_at < $2
		  GROUP BY status ORDER BY 2 DESC`, rg.From, rg.To)
	if err != nil {
		return nil, fmt.Errorf("agent runs by status: %w", err)
	}
	defer rows.Close()
	return scanCountByKey(rows, "agent runs by status")
}

func scanCountByKey(rows pgx.Rows, op string) ([]domain.CountByKey, error) {
	var out []domain.CountByKey
	for rows.Next() {
		var c domain.CountByKey
		if err := rows.Scan(&c.Key, &c.Count); err != nil {
			return nil, fmt.Errorf("scan %s: %w", op, err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
