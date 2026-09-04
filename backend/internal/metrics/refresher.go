package metrics

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// refreshTimeout bounds one refresh pass. Well under any sane interval, so a
// wedged database cannot stack passes up.
const refreshTimeout = 10 * time.Second

// PlanStatusCounter counts plans by lifecycle status. Implemented by
// service.ChatService; declared here so this package stays a leaf.
type PlanStatusCounter interface {
	PlanStatusCounts(ctx context.Context) (map[string]int, error)
}

// Pinger checks that a dependency is reachable. *pgxpool.Pool satisfies it.
type Pinger interface {
	Ping(ctx context.Context) error
}

// RefreshSources are the inputs the refresher polls. Each is optional.
type RefreshSources struct {
	Plans PlanStatusCounter
	DB    Pinger
}

// StartRefresher polls what is too expensive to compute at scrape time and
// returns a stop func that waits for the goroutine to exit.
//
// Plans-by-status is one Postgres query plus one plan_state file read per plan,
// against a volume that may be network-backed, so doing it inside Collect would
// put that latency on every scrape and block the handler on the database.
func (m *Metrics) StartRefresher(ctx context.Context, src RefreshSources, every time.Duration) (stop func()) {
	if !m.Enabled() || (src.Plans == nil && src.DB == nil) {
		return func() {}
	}
	if every <= 0 {
		every = 30 * time.Second
	}

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		// Refresh once up front so the first scrape after startup is populated
		// rather than showing every plan status at zero.
		m.refresh(ctx, src, every)

		ticker := time.NewTicker(every)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.refresh(ctx, src, every)
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() { close(done) })
		wg.Wait()
	}
}

func (m *Metrics) refresh(ctx context.Context, src RefreshSources, every time.Duration) {
	if src.DB != nil {
		start := time.Now()
		pingCtx, cancel := context.WithTimeout(ctx, refreshTimeout)
		err := src.DB.Ping(pingCtx)
		cancel()
		m.observeDBPing(time.Since(start), err)
		if err != nil {
			slog.Warn("metrics: database ping failed", "error", err)
		}
	}

	if src.Plans == nil {
		return
	}

	start := time.Now()
	planCtx, cancel := context.WithTimeout(ctx, refreshTimeout)
	counts, err := src.Plans.PlanStatusCounts(planCtx)
	cancel()
	elapsed := time.Since(start)

	m.observePlansRefresh(elapsed, err)
	if err != nil {
		// The gauges keep their last good values on purpose: a transient
		// database error should not read as every plan vanishing.
		slog.Warn("metrics: plan status refresh failed", "error", err)
		return
	}
	m.SetPlansByStatus(counts)

	if elapsed > every {
		slog.Warn("metrics: plan status refresh outran its interval, gauges will lag",
			"duration", elapsed,
			"interval", every,
		)
	}
}
