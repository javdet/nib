package domain

import (
	"time"

	"github.com/google/uuid"
)

// LLMUsageRecord is one recorded LLM call, as written to llm_usage.
//
// DialogID and PlanID are optional: POST /api/v1/chat runs an agent loop that
// belongs to no dialog. CostUSD is optional because only some gateways price a
// call -- nil means "not reported", never "free".
type LLMUsageRecord struct {
	DialogID           *uuid.UUID
	PlanID             *uuid.UUID
	Mode               string
	Operation          string
	Model              string
	PromptTokens       int
	CachedPromptTokens int
	CompletionTokens   int
	ReasoningTokens    int
	TotalTokens        int
	CostUSD            *float64
	DurationMS         int
	Outcome            string
}

// AgentRunUsageRecord is one finished agent-runner container, from its webhook.
type AgentRunUsageRecord struct {
	DialogID     *uuid.UUID
	PlanID       *uuid.UUID
	JobName      string
	SessionID    string
	Status       string
	InputTokens  int
	OutputTokens int
	NumTurns     int
	CostUSD      *float64
	DurationMS   int
}

// StatsRange is a validated, bucketed window over the statistics tables. It is
// only ever built by the service, which bounds the bucket count.
type StatsRange struct {
	From   time.Time
	To     time.Time
	Bucket string
	// Unit and Step are the date_trunc unit and generate_series interval that
	// Bucket resolves to. They come from a fixed table, never from user input.
	Unit string
	Step string
}

// CountByKey is one row of any "group by a low-cardinality column" query.
type CountByKey struct {
	Key   string
	Count int
}

// UsageTotals aggregates llm_usage over a whole range.
//
// CostUSD is nil when nothing in the range was priced. CostedCalls says how many
// of Calls carried a cost, so the UI can distinguish "spent nothing" from "was
// never told" instead of drawing a flat zero.
type UsageTotals struct {
	Calls         int
	CostedCalls   int
	FailedCalls   int
	PromptTokens  int
	CachedTokens  int
	OutputTokens  int
	ReasonTokens  int
	TotalTokens   int
	CostUSD       *float64
	AvgDurationMS int
}

// UsageBucket is one time bucket of the token/cost series. Buckets with no rows
// are present with zero counts and a nil cost -- the series is gap-filled so a
// chart cannot draw a straight line across an idle stretch.
type UsageBucket struct {
	TS               time.Time
	Calls            int
	CostedCalls      int
	PromptTokens     int
	CachedTokens     int
	CompletionTokens int
	ReasoningTokens  int
	TotalTokens      int
	CostUSD          *float64
}

// UsageByKey is a token/cost breakdown grouped by model, mode or operation.
type UsageByKey struct {
	Key         string
	Calls       int
	TotalTokens int
	CostUSD     *float64
}

// PlanUsage is one plan's share of the spend. Title is empty when the dialog has
// been deleted: usage outlives the dialog it came from by design.
type PlanUsage struct {
	PlanID      uuid.UUID
	Title       string
	Calls       int
	TotalTokens int
	CostUSD     *float64
}

// CountBucket is one time bucket of a plain count series.
type CountBucket struct {
	TS    time.Time
	Count int
}

// StatusCountBucket is one time bucket of a count series split by status.
type StatusCountBucket struct {
	TS     time.Time
	Status string
	Count  int
}

// AgentRunTotals aggregates agent_run_usage over a whole range.
type AgentRunTotals struct {
	Runs          int
	CostedRuns    int
	Turns         int
	InputTokens   int
	OutputTokens  int
	CostUSD       *float64
	AvgDurationMS int
}

// AgentRunBucket is one time bucket of the agent-run series.
type AgentRunBucket struct {
	TS      time.Time
	Runs    int
	CostUSD *float64
}
