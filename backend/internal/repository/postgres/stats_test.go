package postgres

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

func ptr[T any](v T) *T { return &v }

func day(d, h int) time.Time { return time.Date(2026, 3, d, h, 0, 0, 0, time.UTC) }

// statsRange spans three daily buckets. To falls mid-day so the last bucket is
// part of the range.
var statsRange = domain.StatsRange{From: day(1, 0), To: day(3, 12), Bucket: "day", Unit: "day", Step: "1 day"}

func floatOrNil(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestStatsRepoUsage(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewStatsRepo(pool)
	plan := testDialog(t, pool)
	deletedPlan := uuid.New()

	for _, r := range []struct {
		rec domain.LLMUsageRecord
		at  time.Time
	}{
		{at: day(1, 10), rec: domain.LLMUsageRecord{PlanID: &plan, Mode: "plan", Model: "m-priced", TotalTokens: 100, PromptTokens: 80, CompletionTokens: 20, CostUSD: ptr(0.5), DurationMS: 100, Outcome: "success"}},
		// Usage outlives its dialog: a plan id with no row behind it still counts.
		{at: day(3, 11), rec: domain.LLMUsageRecord{PlanID: &deletedPlan, Mode: "execute", Model: "m-unpriced", TotalTokens: 50, DurationMS: 300, Outcome: "error"}},
		{at: day(5, 0), rec: domain.LLMUsageRecord{Mode: "plan", Model: "m-outside", TotalTokens: 999, CostUSD: ptr(9.0), Outcome: "success"}},
	} {
		if err := repo.InsertLLMUsage(ctx, r.rec); err != nil {
			t.Fatalf("InsertLLMUsage(%s): %v", r.rec.Model, err)
		}
		if _, err := pool.Exec(ctx, `UPDATE llm_usage SET created_at = $1 WHERE model = $2`, r.at, r.rec.Model); err != nil {
			t.Fatalf("backdate %s: %v", r.rec.Model, err)
		}
	}

	totals, err := repo.UsageTotals(ctx, statsRange)
	if err != nil {
		t.Fatalf("UsageTotals: %v", err)
	}
	want := domain.UsageTotals{
		Calls: 2, CostedCalls: 1, FailedCalls: 1, PromptTokens: 80, OutputTokens: 20,
		TotalTokens: 150, CostUSD: ptr(0.5), AvgDurationMS: 200,
	}
	if !reflect.DeepEqual(totals, want) {
		t.Fatalf("UsageTotals = %+v (cost %v), want %+v", totals, floatOrNil(totals.CostUSD), want)
	}

	// Nothing priced in a range is an unknown cost, not $0.
	empty, err := repo.UsageTotals(ctx, domain.StatsRange{From: day(10, 0), To: day(11, 0)})
	if err != nil {
		t.Fatalf("UsageTotals(empty): %v", err)
	}
	if empty.Calls != 0 || empty.CostUSD != nil {
		t.Fatalf("UsageTotals(empty) = %+v, want no calls and a nil cost", empty)
	}

	series, err := repo.UsageSeries(ctx, statsRange)
	if err != nil {
		t.Fatalf("UsageSeries: %v", err)
	}
	if len(series) != 3 {
		t.Fatalf("UsageSeries returned %d buckets, want 3 gap-filled days", len(series))
	}
	for i, b := range series {
		if !b.TS.Equal(day(1+i, 0)) {
			t.Errorf("bucket %d TS = %v, want %v", i, b.TS, day(1+i, 0))
		}
	}
	if series[0].Calls != 1 || floatOrNil(series[0].CostUSD) != 0.5 {
		t.Errorf("day 1 = %+v, want one call costing 0.5", series[0])
	}
	if series[1].Calls != 0 || series[1].CostUSD != nil {
		t.Errorf("day 2 = %+v, want an empty bucket with a nil cost", series[1])
	}
	if series[2].Calls != 1 || series[2].CostedCalls != 0 || series[2].CostUSD != nil {
		t.Errorf("day 3 = %+v, want one unpriced call", series[2])
	}

	byModel, err := repo.UsageByColumn(ctx, statsRange, "model", 10)
	if err != nil {
		t.Fatalf("UsageByColumn: %v", err)
	}
	if len(byModel) != 2 {
		t.Fatalf("UsageByColumn(model) = %+v, want the two in-range models", byModel)
	}
	if _, err := repo.UsageByColumn(ctx, statsRange, "model; DROP TABLE llm_usage", 10); err == nil {
		t.Fatal("UsageByColumn with a column outside the whitelist succeeded, want an error")
	}

	top, err := repo.TopPlansByUsage(ctx, statsRange, 10)
	if err != nil {
		t.Fatalf("TopPlansByUsage: %v", err)
	}
	if len(top) != 2 || top[0].PlanID != plan || top[0].Title != "test" || top[0].TotalTokens != 100 {
		t.Fatalf("TopPlansByUsage = %+v, want the live plan first", top)
	}
	if top[1].PlanID != deletedPlan || top[1].Title != "" {
		t.Fatalf("TopPlansByUsage[1] = %+v, want the deleted plan with an empty title", top[1])
	}
}

func TestStatsRepoAgentRuns(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewStatsRepo(pool)

	// A webhook retry carries the same job name and must not double the cost.
	// Deliveries without a job name cannot be deduplicated and all count.
	for _, rec := range []domain.AgentRunUsageRecord{
		{JobName: "job-1", Status: "success", NumTurns: 4, InputTokens: 10, OutputTokens: 5, CostUSD: ptr(1.25), DurationMS: 1000},
		{JobName: "job-1", Status: "success", NumTurns: 4, InputTokens: 10, OutputTokens: 5, CostUSD: ptr(1.25), DurationMS: 1000},
		{Status: "failed", NumTurns: 1, DurationMS: 3000},
		{Status: "failed", NumTurns: 1, DurationMS: 2000},
	} {
		if err := repo.InsertAgentRunUsage(ctx, rec); err != nil {
			t.Fatalf("InsertAgentRunUsage: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE agent_run_usage SET created_at = $1`, day(2, 6)); err != nil {
		t.Fatalf("backdate agent runs: %v", err)
	}

	totals, err := repo.AgentRunTotals(ctx, statsRange)
	if err != nil {
		t.Fatalf("AgentRunTotals: %v", err)
	}
	want := domain.AgentRunTotals{Runs: 3, CostedRuns: 1, Turns: 6, InputTokens: 10, OutputTokens: 5, CostUSD: ptr(1.25), AvgDurationMS: 2000}
	if !reflect.DeepEqual(totals, want) {
		t.Fatalf("AgentRunTotals = %+v (cost %v), want %+v", totals, floatOrNil(totals.CostUSD), want)
	}

	series, err := repo.AgentRunSeries(ctx, statsRange)
	if err != nil {
		t.Fatalf("AgentRunSeries: %v", err)
	}
	if len(series) != 3 || series[0].Runs != 0 || series[1].Runs != 3 || series[2].Runs != 0 {
		t.Fatalf("AgentRunSeries = %+v, want 0, 3, 0 runs", series)
	}

	byStatus, err := repo.AgentRunsByStatus(ctx, statsRange)
	if err != nil {
		t.Fatalf("AgentRunsByStatus: %v", err)
	}
	if want := []domain.CountByKey{{Key: "failed", Count: 2}, {Key: "success", Count: 1}}; !reflect.DeepEqual(byStatus, want) {
		t.Fatalf("AgentRunsByStatus = %+v, want %+v", byStatus, want)
	}
}

func TestStatsRepoPlans(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewStatsRepo(pool)
	dialogs := NewDialogRepo(pool)

	inRange := mustCreateDialog(t, dialogs, "main", "a", nil)
	mustCreateDialog(t, dialogs, "plan", "b", nil)
	mustCreateDialog(t, dialogs, "execute", "child", &inRange.ID)
	mustCreateDialog(t, dialogs, "discuss", "chat", nil)
	if _, err := pool.Exec(ctx, `UPDATE chat_dialogs SET created_at = $1 WHERE id = $2`, day(2, 9), inRange.ID); err != nil {
		t.Fatalf("backdate dialog: %v", err)
	}

	// Only roots in a plan mode are plans; the child and the chat are not.
	if n, err := repo.PlansTotal(ctx); err != nil || n != 2 {
		t.Fatalf("PlansTotal = %d, %v, want 2", n, err)
	}
	if n, err := repo.PlansCreatedInRange(ctx, statsRange); err != nil || n != 1 {
		t.Fatalf("PlansCreatedInRange = %d, %v, want 1", n, err)
	}
	byMode, err := repo.PlansByMode(ctx)
	if err != nil {
		t.Fatalf("PlansByMode: %v", err)
	}
	if len(byMode) != 2 {
		t.Fatalf("PlansByMode = %+v, want main and plan", byMode)
	}
	created, err := repo.PlansCreatedSeries(ctx, statsRange)
	if err != nil {
		t.Fatalf("PlansCreatedSeries: %v", err)
	}
	if len(created) != 3 || created[0].Count != 0 || created[1].Count != 1 || created[2].Count != 0 {
		t.Fatalf("PlansCreatedSeries = %+v, want 0, 1, 0", created)
	}

	if err := repo.InsertPlanStatusTransition(ctx, inRange.ID.String(), "draft", "in_progress"); err != nil {
		t.Fatalf("InsertPlanStatusTransition: %v", err)
	}
	if err := repo.InsertPlanStatusTransition(ctx, inRange.ID.String(), "in_progress", "done"); err != nil {
		t.Fatalf("InsertPlanStatusTransition: %v", err)
	}
	if err := repo.InsertPlanStatusTransition(ctx, "not-a-uuid", "draft", "done"); err == nil {
		t.Fatal("InsertPlanStatusTransition with a bad id succeeded, want an error")
	}
	if err := repo.InsertPlanStatusTransition(ctx, inRange.ID.String(), "done", "done"); err == nil {
		t.Fatal("InsertPlanStatusTransition to the same status succeeded, want the CHECK to refuse it")
	}
	if _, err := pool.Exec(ctx, `UPDATE plan_status_transitions SET created_at = $1`, day(3, 1)); err != nil {
		t.Fatalf("backdate transitions: %v", err)
	}
	transitions, err := repo.StatusTransitionSeries(ctx, statsRange)
	if err != nil {
		t.Fatalf("StatusTransitionSeries: %v", err)
	}
	want := []domain.StatusCountBucket{
		{TS: day(3, 0), Status: "done", Count: 1},
		{TS: day(3, 0), Status: "in_progress", Count: 1},
	}
	if len(transitions) != len(want) {
		t.Fatalf("StatusTransitionSeries = %+v, want %+v", transitions, want)
	}
	for i := range want {
		if !transitions[i].TS.Equal(want[i].TS) || transitions[i].Status != want[i].Status || transitions[i].Count != want[i].Count {
			t.Fatalf("StatusTransitionSeries[%d] = %+v, want %+v", i, transitions[i], want[i])
		}
	}
}
