package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/javdet/nib/internal/domain"
)

// fakeStatsReader answers every query with zero values except the ones a test
// sets, so each test states only what it is about.
type fakeStatsReader struct {
	totals   domain.UsageTotals
	byColumn map[string][]domain.UsageByKey
}

func (f *fakeStatsReader) UsageTotals(context.Context, domain.StatsRange) (domain.UsageTotals, error) {
	return f.totals, nil
}
func (f *fakeStatsReader) UsageSeries(context.Context, domain.StatsRange) ([]domain.UsageBucket, error) {
	return nil, nil
}
func (f *fakeStatsReader) UsageByColumn(_ context.Context, _ domain.StatsRange, column string, _ int) ([]domain.UsageByKey, error) {
	return f.byColumn[column], nil
}
func (f *fakeStatsReader) TopPlansByUsage(context.Context, domain.StatsRange, int) ([]domain.PlanUsage, error) {
	return nil, nil
}
func (f *fakeStatsReader) PlansCreatedSeries(context.Context, domain.StatsRange) ([]domain.CountBucket, error) {
	return nil, nil
}
func (f *fakeStatsReader) PlansCreatedInRange(context.Context, domain.StatsRange) (int, error) {
	return 0, nil
}
func (f *fakeStatsReader) PlansTotal(context.Context) (int, error) { return 0, nil }
func (f *fakeStatsReader) PlansByMode(context.Context) ([]domain.CountByKey, error) {
	return nil, nil
}
func (f *fakeStatsReader) StatusTransitionSeries(context.Context, domain.StatsRange) ([]domain.StatusCountBucket, error) {
	return nil, nil
}
func (f *fakeStatsReader) AgentRunTotals(context.Context, domain.StatsRange) (domain.AgentRunTotals, error) {
	return domain.AgentRunTotals{}, nil
}
func (f *fakeStatsReader) AgentRunSeries(context.Context, domain.StatsRange) ([]domain.AgentRunBucket, error) {
	return nil, nil
}
func (f *fakeStatsReader) AgentRunsByStatus(context.Context, domain.StatsRange) ([]domain.CountByKey, error) {
	return nil, nil
}

type fakeStatusCounter struct {
	counts map[string]int
	calls  int
	err    error
}

func (f *fakeStatusCounter) PlanStatusCounts(context.Context) (map[string]int, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.counts, nil
}

func testRange() domain.StatsRange {
	return domain.StatsRange{
		From: time.Now().Add(-24 * time.Hour), To: time.Now(),
		Bucket: "day", Unit: "day", Step: "1 day",
	}
}

// An unpriced range must reach the UI as null, not as 0: "nobody told us" and
// "it was free" are different claims and the page renders them differently.
func TestReportKeepsUnreportedCostNil(t *testing.T) {
	svc := NewStatsService(&fakeStatsReader{
		totals: domain.UsageTotals{Calls: 12, CostedCalls: 0, CostUSD: nil},
	}, &fakeStatusCounter{counts: map[string]int{}})

	rep, err := svc.Report(context.Background(), testRange())
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	if rep.Usage.Totals.CostUSD != nil {
		t.Errorf("CostUSD = %v, want nil when no call was priced", *rep.Usage.Totals.CostUSD)
	}
	if rep.Usage.Totals.CostedCalls != 0 {
		t.Errorf("CostedCalls = %d, want 0", rep.Usage.Totals.CostedCalls)
	}
	if rep.Usage.Totals.Calls != 12 {
		t.Errorf("Calls = %d, want 12", rep.Usage.Totals.Calls)
	}
}

// The by-model query is capped because the provider chooses the model string.
// Whatever the cap cut off has to reappear, or the breakdown silently adds up
// to less than the headline total and reads as a bug.
func TestReportFoldsModelTailIntoOther(t *testing.T) {
	rows := make([]domain.UsageByKey, 0, modelLimit)
	for i := 0; i < modelLimit; i++ {
		rows = append(rows, domain.UsageByKey{Key: "m", Calls: 1, TotalTokens: 10})
	}

	svc := NewStatsService(&fakeStatsReader{
		totals:   domain.UsageTotals{Calls: 50, TotalTokens: 500},
		byColumn: map[string][]domain.UsageByKey{"model": rows},
	}, &fakeStatusCounter{counts: map[string]int{}})

	rep, err := svc.Report(context.Background(), testRange())
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}

	last := rep.Usage.ByModel[len(rep.Usage.ByModel)-1]
	if last.Key != "other" {
		t.Fatalf("last by-model key = %q, want %q", last.Key, "other")
	}
	if want := 50 - modelLimit; last.Calls != want {
		t.Errorf("other calls = %d, want %d", last.Calls, want)
	}
	if want := 500 - modelLimit*10; last.TotalTokens != want {
		t.Errorf("other tokens = %d, want %d", last.TotalTokens, want)
	}
}

func TestReportDoesNotFoldWhenUnderTheCap(t *testing.T) {
	svc := NewStatsService(&fakeStatsReader{
		totals: domain.UsageTotals{Calls: 50, TotalTokens: 500},
		byColumn: map[string][]domain.UsageByKey{
			"model": {{Key: "gpt-5.6", Calls: 3, TotalTokens: 30}},
		},
	}, &fakeStatusCounter{counts: map[string]int{}})

	rep, err := svc.Report(context.Background(), testRange())
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	for _, m := range rep.Usage.ByModel {
		if m.Key == "other" {
			t.Fatal("a short by-model list must not gain an 'other' row")
		}
	}
}

// The snapshot is N plan_state file reads, so a held-down refresh must not
// re-read every file.
func TestPlanStatusSnapshotIsCached(t *testing.T) {
	counter := &fakeStatusCounter{counts: map[string]int{"draft": 2, "done": 1}}
	svc := NewStatsService(&fakeStatsReader{}, counter)

	for i := 0; i < 3; i++ {
		if _, err := svc.Report(context.Background(), testRange()); err != nil {
			t.Fatalf("Report() error = %v", err)
		}
	}
	if counter.calls != 1 {
		t.Errorf("PlanStatusCounts called %d times, want 1 within the TTL", counter.calls)
	}
}

// Every status is published, including the zeroes: a status missing from the
// donut reads as "not a thing" rather than "none right now".
func TestPlanStatusSnapshotPublishesEveryStatus(t *testing.T) {
	svc := NewStatsService(&fakeStatsReader{},
		&fakeStatusCounter{counts: map[string]int{"draft": 2}})

	rep, err := svc.Report(context.Background(), testRange())
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	if got, want := len(rep.Plans.CurrentByStatus), len(actionPlanStatuses); got != want {
		t.Fatalf("CurrentByStatus has %d entries, want %d", got, want)
	}
	if rep.Plans.AsOf.IsZero() {
		t.Error("AsOf must be set: the snapshot is of now, not of the range")
	}
}

// A transient failure should not read as every plan vanishing, which is the
// same reasoning the metrics refresher applies to its gauges.
func TestPlanStatusSnapshotFailureDoesNotFailTheReport(t *testing.T) {
	svc := NewStatsService(&fakeStatsReader{},
		&fakeStatusCounter{err: errors.New("boom")})

	rep, err := svc.Report(context.Background(), testRange())
	if err != nil {
		t.Fatalf("Report() must survive a snapshot failure, got %v", err)
	}
	if rep.Plans.CurrentByStatus != nil {
		t.Errorf("CurrentByStatus = %v, want nil when nothing has ever been cached", rep.Plans.CurrentByStatus)
	}
}
