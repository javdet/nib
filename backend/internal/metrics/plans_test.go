package metrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestSetPlansByStatus_seedsEveryStatus(t *testing.T) {
	m := newTestMetrics(t)

	// Setup seeds all six at zero so the very first scrape is complete rather
	// than missing whichever statuses no plan happens to be in.
	if got, want := testutil.CollectAndCount(m.plans.byStatus), len(PlanStatuses); got != want {
		t.Fatalf("seeded plan series = %d, want %d", got, want)
	}
}

func TestSetPlansByStatus_zeroesStatusesThatEmptied(t *testing.T) {
	m := newTestMetrics(t)

	m.SetPlansByStatus(map[string]int{"draft": 3, "done": 1})
	if got := testutil.ToFloat64(m.plans.byStatus.WithLabelValues("draft")); got != 3 {
		t.Fatalf("draft = %v, want 3", got)
	}

	m.SetPlansByStatus(map[string]int{"done": 1})

	// Writing only the statuses present in the map would leave draft frozen at
	// 3 forever, which is why every known status is written on every call.
	if got := testutil.ToFloat64(m.plans.byStatus.WithLabelValues("draft")); got != 0 {
		t.Errorf("draft after it emptied = %v, want 0", got)
	}
	if got := testutil.ToFloat64(m.plans.byStatus.WithLabelValues("done")); got != 1 {
		t.Errorf("done = %v, want 1", got)
	}
}

func TestSetPlansByStatus_ignoresUnknownStatus(t *testing.T) {
	m := newTestMetrics(t)

	// A plan_state file holding a status this build does not know about must not
	// create a series for it.
	m.SetPlansByStatus(map[string]int{"draft": 1, "invented_status": 7})

	if got, want := testutil.CollectAndCount(m.plans.byStatus), len(PlanStatuses); got != want {
		t.Errorf("plan series = %d, want %d", got, want)
	}
}

type fakePlanCounter struct {
	counts map[string]int
	err    error
	calls  int
}

func (f *fakePlanCounter) PlanStatusCounts(context.Context) (map[string]int, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.counts, nil
}

type fakePinger struct {
	err   error
	calls int
}

func (f *fakePinger) Ping(context.Context) error {
	f.calls++
	return f.err
}

func TestStartRefresher_refreshesImmediately(t *testing.T) {
	m := newTestMetrics(t)

	plans := &fakePlanCounter{counts: map[string]int{"in_progress": 2}}
	db := &fakePinger{}

	// A long interval proves the first pass is not the ticker firing.
	stop := m.StartRefresher(context.Background(), RefreshSources{Plans: plans, DB: db}, time.Hour)
	stop()

	if plans.calls != 1 {
		t.Errorf("plan counts calls = %d, want 1", plans.calls)
	}
	if db.calls != 1 {
		t.Errorf("ping calls = %d, want 1", db.calls)
	}
	if got := testutil.ToFloat64(m.plans.byStatus.WithLabelValues("in_progress")); got != 2 {
		t.Errorf("in_progress = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.db.up); got != 1 {
		t.Errorf("db up = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.plans.refreshedAt); got == 0 {
		t.Error("refresh timestamp was never set")
	}
}

func TestStartRefresher_planErrorKeepsLastGoodGauges(t *testing.T) {
	m := newTestMetrics(t)

	plans := &fakePlanCounter{counts: map[string]int{"draft": 5}}
	stop := m.StartRefresher(context.Background(), RefreshSources{Plans: plans}, time.Hour)
	stop()

	plans.err = errors.New("database is down")
	stop = m.StartRefresher(context.Background(), RefreshSources{Plans: plans}, time.Hour)
	stop()

	// A transient database error must not read as every plan vanishing.
	if got := testutil.ToFloat64(m.plans.byStatus.WithLabelValues("draft")); got != 5 {
		t.Errorf("draft after a failed refresh = %v, want the last good 5", got)
	}
	if got := testutil.ToFloat64(m.plans.refreshErrors); got != 1 {
		t.Errorf("refresh errors = %v, want 1", got)
	}
}

func TestStartRefresher_pingErrorMarksDBDown(t *testing.T) {
	m := newTestMetrics(t)

	db := &fakePinger{err: errors.New("connection refused")}
	stop := m.StartRefresher(context.Background(), RefreshSources{DB: db}, time.Hour)
	stop()

	if got := testutil.ToFloat64(m.db.up); got != 0 {
		t.Errorf("db up = %v, want 0", got)
	}
}

func TestStartRefresher_stopIsIdempotentAndHaltsPolling(t *testing.T) {
	m := newTestMetrics(t)

	plans := &fakePlanCounter{counts: map[string]int{}}
	stop := m.StartRefresher(context.Background(), RefreshSources{Plans: plans}, 10*time.Millisecond)
	stop()
	stop()

	after := plans.calls
	time.Sleep(50 * time.Millisecond)
	if plans.calls != after {
		t.Errorf("refresher kept polling after stop: %d -> %d", after, plans.calls)
	}
}

func TestStartRefresher_disabledAndEmptySourcesAreNoops(t *testing.T) {
	disabled, err := Setup(Config{Enabled: false})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { SetDefault(nil) })
	disabled.StartRefresher(context.Background(), RefreshSources{Plans: &fakePlanCounter{}}, time.Second)()

	m := newTestMetrics(t)
	m.StartRefresher(context.Background(), RefreshSources{}, time.Second)()
}
