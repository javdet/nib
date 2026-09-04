package metrics

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRegisterPool_exposesEveryStat(t *testing.T) {
	m := newTestMetrics(t)

	// Feeding a literal is only possible because the collector takes a
	// func() PoolStats: pgxpool.Stat has no exported constructor.
	stats := PoolStats{
		Total: 7, Acquired: 3, Idle: 4, Constructing: 1, Max: 10,
		Acquires: 100, EmptyAcquires: 5, CanceledAcquires: 2, NewConns: 9,
		MaxLifetimeDestroys: 4, MaxIdleDestroys: 3,
		AcquireWait: 2 * time.Second, EmptyAcquireWait: 500 * time.Millisecond,
	}
	if err := m.RegisterPool(func() PoolStats { return stats }); err != nil {
		t.Fatalf("RegisterPool: %v", err)
	}

	want := `
# HELP nib_db_pool_connections Database pool connections by state.
# TYPE nib_db_pool_connections gauge
nib_db_pool_connections{state="acquired"} 3
nib_db_pool_connections{state="constructing"} 1
nib_db_pool_connections{state="idle"} 4
nib_db_pool_connections{state="total"} 7
`
	if err := testutil.GatherAndCompare(m.Registry(), stringReader(want), "nib_db_pool_connections"); err != nil {
		t.Error(err)
	}

	wantDestroys := `
# HELP nib_db_pool_destroys_total Connections the pool closed, by reason.
# TYPE nib_db_pool_destroys_total counter
nib_db_pool_destroys_total{reason="max_idle"} 3
nib_db_pool_destroys_total{reason="max_lifetime"} 4
`
	if err := testutil.GatherAndCompare(m.Registry(), stringReader(wantDestroys), "nib_db_pool_destroys_total"); err != nil {
		t.Error(err)
	}

	wantWait := `
# HELP nib_db_pool_acquire_wait_seconds_total Cumulative time spent acquiring connections.
# TYPE nib_db_pool_acquire_wait_seconds_total counter
nib_db_pool_acquire_wait_seconds_total 2
`
	if err := testutil.GatherAndCompare(m.Registry(), stringReader(wantWait), "nib_db_pool_acquire_wait_seconds_total"); err != nil {
		t.Error(err)
	}
}

func TestRegisterPool_reflectsLaterSnapshots(t *testing.T) {
	m := newTestMetrics(t)

	acquired := int32(1)
	if err := m.RegisterPool(func() PoolStats { return PoolStats{Acquired: acquired} }); err != nil {
		t.Fatalf("RegisterPool: %v", err)
	}

	// The pool owns the counters, so they have to be read per scrape rather than
	// added into a Counter -- which would multiply the totals every scrape.
	acquired = 42
	want := `
# HELP nib_db_pool_connections Database pool connections by state.
# TYPE nib_db_pool_connections gauge
nib_db_pool_connections{state="acquired"} 42
nib_db_pool_connections{state="constructing"} 0
nib_db_pool_connections{state="idle"} 0
nib_db_pool_connections{state="total"} 0
`
	if err := testutil.GatherAndCompare(m.Registry(), stringReader(want), "nib_db_pool_connections"); err != nil {
		t.Error(err)
	}
}

func TestRegisterPool_rejectsNilStats(t *testing.T) {
	m := newTestMetrics(t)
	if err := m.RegisterPool(nil); err == nil {
		t.Fatal("RegisterPool(nil) returned no error")
	}
}

func stringReader(s string) io.Reader { return strings.NewReader(s) }
