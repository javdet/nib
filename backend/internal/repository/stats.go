package repository

import (
	"context"

	"github.com/javdet/nib/internal/domain"
)

// StatsWriter records usage as it happens. It is separate from StatsReader
// because the writers sit on the agent's hot path and the readers behind one
// dashboard endpoint: nothing needs both.
type StatsWriter interface {
	InsertLLMUsage(ctx context.Context, rec domain.LLMUsageRecord) error
	// InsertAgentRunUsage ignores a repeat delivery of the same job, which the
	// agent container retries with curl.
	InsertAgentRunUsage(ctx context.Context, rec domain.AgentRunUsageRecord) error
	InsertPlanStatusTransition(ctx context.Context, planID, from, to string) error
}

// StatsReader answers the aggregation queries behind GET /api/v1/stats.
type StatsReader interface {
	UsageTotals(ctx context.Context, r domain.StatsRange) (domain.UsageTotals, error)
	UsageSeries(ctx context.Context, r domain.StatsRange) ([]domain.UsageBucket, error)
	UsageByColumn(ctx context.Context, r domain.StatsRange, column string, limit int) ([]domain.UsageByKey, error)
	TopPlansByUsage(ctx context.Context, r domain.StatsRange, limit int) ([]domain.PlanUsage, error)
	PlansCreatedSeries(ctx context.Context, r domain.StatsRange) ([]domain.CountBucket, error)
	PlansCreatedInRange(ctx context.Context, r domain.StatsRange) (int, error)
	PlansTotal(ctx context.Context) (int, error)
	PlansByMode(ctx context.Context) ([]domain.CountByKey, error)
	StatusTransitionSeries(ctx context.Context, r domain.StatsRange) ([]domain.StatusCountBucket, error)
	AgentRunTotals(ctx context.Context, r domain.StatsRange) (domain.AgentRunTotals, error)
	AgentRunSeries(ctx context.Context, r domain.StatsRange) ([]domain.AgentRunBucket, error)
	AgentRunsByStatus(ctx context.Context, r domain.StatsRange) ([]domain.CountByKey, error)
}

// StatsRepository is both halves, which is what the composition root wires.
type StatsRepository interface {
	StatsWriter
	StatsReader
}
