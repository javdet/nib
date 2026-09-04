package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// PlanStatuses mirrors the service.ActionPlanStatus constants.
//
// It is duplicated rather than imported because this package is a leaf: it must
// not depend on internal/service, which records into it. The drift risk is
// covered by a test in internal/service that asserts the two lists agree.
var PlanStatuses = []string{"draft", "scheduled", "in_progress", "done", "reopened", "rolled_back"}

type planMetrics struct {
	byStatus        *prometheus.GaugeVec
	refreshDuration prometheus.Histogram
	refreshErrors   prometheus.Counter
	refreshedAt     prometheus.Gauge
}

func newPlanMetrics() planMetrics {
	return planMetrics{
		// No _total suffix: that is reserved for counters, and this is a gauge.
		byStatus: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "plans",
			Help:      "Plans by lifecycle status.",
		}, []string{"status"}),
		refreshDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "plans_refresh_duration_seconds",
			Help:      "Duration of a plans-by-status refresh: one query plus one file read per plan.",
			Buckets:   llmBuckets,
		}),
		refreshErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "plans_refresh_errors_total",
			Help:      "Failed plans-by-status refreshes, after which the gauges are stale.",
		}),
		refreshedAt: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "plans_refresh_timestamp_seconds",
			Help:      "Unix timestamp of the last successful plans-by-status refresh.",
		}),
	}
}

func (p planMetrics) collectors() []prometheus.Collector {
	return []prometheus.Collector{p.byStatus, p.refreshDuration, p.refreshErrors, p.refreshedAt}
}

// seed publishes every status at zero, so the first scrape is complete rather
// than missing the statuses no plan happens to be in yet.
func (p planMetrics) seed() {
	for _, s := range PlanStatuses {
		p.byStatus.WithLabelValues(s).Set(0)
	}
}

// SetPlansByStatus publishes the plan counts.
//
// It writes every known status on every call, not just the ones present in
// counts: a status that empties out has to fall to zero rather than freeze at
// whatever it last was.
func (m *Metrics) SetPlansByStatus(counts map[string]int) {
	if !m.Enabled() {
		return
	}
	for _, s := range PlanStatuses {
		m.plans.byStatus.WithLabelValues(s).Set(float64(counts[s]))
	}
}

func (m *Metrics) observePlansRefresh(d time.Duration, err error) {
	if !m.Enabled() {
		return
	}
	m.plans.refreshDuration.Observe(d.Seconds())
	if err != nil {
		m.plans.refreshErrors.Inc()
		return
	}
	m.plans.refreshedAt.Set(float64(time.Now().Unix()))
}
