package metrics

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// PoolStats is a snapshot of a database connection pool.
//
// It mirrors pgxpool.Stat field for field rather than taking the pgx type
// directly, because pgxpool.Stat has no exported constructor -- a collector
// built on it could never be given a known value in a test.
type PoolStats struct {
	Total        int32
	Acquired     int32
	Idle         int32
	Constructing int32
	Max          int32

	Acquires            int64
	EmptyAcquires       int64
	CanceledAcquires    int64
	NewConns            int64
	MaxLifetimeDestroys int64
	MaxIdleDestroys     int64

	AcquireWait      time.Duration
	EmptyAcquireWait time.Duration
}

type dbMetrics struct {
	up           prometheus.Gauge
	pingDuration prometheus.Histogram
}

func newDBMetrics() dbMetrics {
	return dbMetrics{
		// The real readiness signal: /api/v1/health answers ok unconditionally
		// and never touches the database.
		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: "db",
			Name:      "up",
			Help:      "1 when the last database ping succeeded, 0 otherwise.",
		}),
		pingDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: Namespace,
			Subsystem: "db",
			Name:      "ping_duration_seconds",
			Help:      "Duration of the periodic database ping.",
			Buckets:   llmBuckets,
		}),
	}
}

func (d dbMetrics) collectors() []prometheus.Collector {
	return []prometheus.Collector{d.up, d.pingDuration}
}

func (m *Metrics) observeDBPing(d time.Duration, err error) {
	if !m.Enabled() {
		return
	}
	m.db.pingDuration.Observe(d.Seconds())
	if err != nil {
		m.db.up.Set(0)
		return
	}
	m.db.up.Set(1)
}

// RegisterPool exposes a connection pool through stats, which is called once per
// scrape.
//
// Scrape-time collection is right here: a pool snapshot is one in-memory struct
// read with no I/O. The counters have to be const metrics rather than
// prometheus.Counter values because the pool owns them -- Add-ing the totals on
// every scrape would multiply them.
func (m *Metrics) RegisterPool(stats func() PoolStats) error {
	if !m.Enabled() {
		return nil
	}
	if stats == nil {
		return fmt.Errorf("metrics: register pool: stats func is nil")
	}
	if err := m.reg.Register(&poolCollector{stats: stats}); err != nil {
		return fmt.Errorf("metrics: register pool: %w", err)
	}
	return nil
}

type poolCollector struct {
	stats func() PoolStats
}

var (
	poolConns = prometheus.NewDesc(
		Namespace+"_db_pool_connections",
		"Database pool connections by state.",
		[]string{"state"}, nil)
	poolMaxConns = prometheus.NewDesc(
		Namespace+"_db_pool_max_connections",
		"Maximum database pool size.",
		nil, nil)
	poolAcquires = prometheus.NewDesc(
		Namespace+"_db_pool_acquires_total",
		"Connections acquired from the pool.",
		nil, nil)
	poolEmptyAcquires = prometheus.NewDesc(
		Namespace+"_db_pool_empty_acquires_total",
		"Acquires that had to wait because the pool was empty.",
		nil, nil)
	poolCanceledAcquires = prometheus.NewDesc(
		Namespace+"_db_pool_canceled_acquires_total",
		"Acquires cancelled before a connection became available.",
		nil, nil)
	poolNewConns = prometheus.NewDesc(
		Namespace+"_db_pool_new_connections_total",
		"Connections the pool opened.",
		nil, nil)
	poolDestroys = prometheus.NewDesc(
		Namespace+"_db_pool_destroys_total",
		"Connections the pool closed, by reason.",
		[]string{"reason"}, nil)
	poolAcquireWait = prometheus.NewDesc(
		Namespace+"_db_pool_acquire_wait_seconds_total",
		"Cumulative time spent acquiring connections.",
		nil, nil)
	poolEmptyAcquireWait = prometheus.NewDesc(
		Namespace+"_db_pool_empty_acquire_wait_seconds_total",
		"Cumulative time spent waiting on an empty pool.",
		nil, nil)
)

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- poolConns
	ch <- poolMaxConns
	ch <- poolAcquires
	ch <- poolEmptyAcquires
	ch <- poolCanceledAcquires
	ch <- poolNewConns
	ch <- poolDestroys
	ch <- poolAcquireWait
	ch <- poolEmptyAcquireWait
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.stats()

	gauge := func(d *prometheus.Desc, v float64, labels ...string) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v, labels...)
	}
	counter := func(d *prometheus.Desc, v float64, labels ...string) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, v, labels...)
	}

	gauge(poolConns, float64(s.Acquired), "acquired")
	gauge(poolConns, float64(s.Idle), "idle")
	gauge(poolConns, float64(s.Constructing), "constructing")
	gauge(poolConns, float64(s.Total), "total")
	gauge(poolMaxConns, float64(s.Max))

	counter(poolAcquires, float64(s.Acquires))
	counter(poolEmptyAcquires, float64(s.EmptyAcquires))
	counter(poolCanceledAcquires, float64(s.CanceledAcquires))
	counter(poolNewConns, float64(s.NewConns))
	counter(poolDestroys, float64(s.MaxLifetimeDestroys), "max_lifetime")
	counter(poolDestroys, float64(s.MaxIdleDestroys), "max_idle")
	counter(poolAcquireWait, s.AcquireWait.Seconds())
	counter(poolEmptyAcquireWait, s.EmptyAcquireWait.Seconds())
}
