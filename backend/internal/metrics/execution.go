package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type executionMetrics struct {
	leaseHeld         prometheus.Gauge
	leaseAcquisitions *prometheus.CounterVec
	leaseRejections   *prometheus.CounterVec
	leaseExpirations  prometheus.Counter
	leaseHoldSeconds  *prometheus.HistogramVec
	forceStops        *prometheus.CounterVec

	actionStarted  *prometheus.CounterVec
	actionFinished *prometheus.CounterVec
	actionDuration *prometheus.HistogramVec
	actionActive   prometheus.Gauge

	fanoutStarted    prometheus.Counter
	fanoutFinished   *prometheus.CounterVec
	fanoutDuration   *prometheus.HistogramVec
	fanoutStages     *prometheus.CounterVec
	fanoutBlockers   prometheus.Counter
	fanoutRejections *prometheus.CounterVec
	fanoutActive     prometheus.Gauge

	stuckReconciled *prometheus.CounterVec
	planTransitions *prometheus.CounterVec
}

func newExecutionMetrics() executionMetrics {
	return executionMetrics{
		leaseHeld: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: "execution",
			Name:      "lease_held",
			Help:      "1 while the single global execution lease is held, 0 otherwise.",
		}),
		leaseAcquisitions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "execution",
			Name:      "lease_acquisitions_total",
			Help:      "Successful execution lease grants by holder kind.",
		}, []string{"kind"}),
		// One execution at a time is a policy, not a capacity limit, and a
		// second request is refused rather than queued. This counter is the only
		// measure of operators being turned away by it.
		leaseRejections: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "execution",
			Name:      "lease_rejections_total",
			Help:      "Execution requests refused because the lease was already held.",
		}, []string{"kind"}),
		leaseExpirations: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "execution",
			Name:      "lease_expirations_total",
			Help:      "Stale container leases dropped because nothing would ever end them.",
		}),
		leaseHoldSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace,
			Subsystem: "execution",
			Name:      "lease_hold_seconds",
			Help:      "How long the execution lease was held, by holder kind.",
			Buckets:   runBuckets,
		}, []string{"kind"}),
		forceStops: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "execution",
			Name:      "force_stops_total",
			Help:      "Operator force stops of the running execution.",
		}, []string{"kind", "outcome"}),
		actionStarted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "action_exec",
			Name:      "runs_started_total",
			Help:      "Action executions started, by holder kind.",
		}, []string{"kind"}),
		actionFinished: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "action_exec",
			Name:      "runs_finished_total",
			Help:      "Action executions that reached a terminal status.",
		}, []string{"status"}),
		actionDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace,
			Subsystem: "action_exec",
			Name:      "run_duration_seconds",
			Help:      "Action execution duration by terminal status.",
			Buckets:   runBuckets,
		}, []string{"status"}),
		actionActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: "action_exec",
			Name:      "active",
			Help:      "Action executions currently running.",
		}),
		fanoutStarted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "plan_fanout",
			Name:      "runs_started_total",
			Help:      "Plan fan-out runs started.",
		}),
		fanoutFinished: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "plan_fanout",
			Name:      "runs_finished_total",
			Help:      "Plan fan-out runs that reached a terminal status.",
		}, []string{"status"}),
		fanoutDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace,
			Subsystem: "plan_fanout",
			Name:      "run_duration_seconds",
			Help:      "Plan fan-out run duration by terminal status.",
			Buckets:   runBuckets,
		}, []string{"status"}),
		fanoutStages: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "plan_fanout",
			Name:      "stages_total",
			Help:      "Fan-out stages that reached a terminal status, by stage kind.",
		}, []string{"kind", "status"}),
		fanoutBlockers: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "plan_fanout",
			Name:      "blockers_total",
			Help:      "Questions a fan-out suspended on to ask the operator.",
		}),
		fanoutRejections: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "plan_fanout",
			Name:      "rejections_total",
			Help:      "Fan-out requests refused, by reason.",
		}, []string{"reason"}),
		fanoutActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: "plan_fanout",
			Name:      "active",
			Help:      "Plan fan-out runs currently running.",
		}),
		// Non-zero after a start means the previous process died mid-run.
		stuckReconciled: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "stuck_runs_reconciled_total",
			Help:      "Runs a previous process left marked running, closed at boot.",
		}, []string{"kind"}),
		planTransitions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "plan_status_transitions_total",
			Help:      "Plan lifecycle status changes.",
		}, []string{"from", "to"}),
	}
}

func (e executionMetrics) collectors() []prometheus.Collector {
	return []prometheus.Collector{
		e.leaseHeld, e.leaseAcquisitions, e.leaseRejections, e.leaseExpirations,
		e.leaseHoldSeconds, e.forceStops,
		e.actionStarted, e.actionFinished, e.actionDuration, e.actionActive,
		e.fanoutStarted, e.fanoutFinished, e.fanoutDuration, e.fanoutStages,
		e.fanoutBlockers, e.fanoutRejections, e.fanoutActive,
		e.stuckReconciled, e.planTransitions,
	}
}

// RecordLeaseAcquired marks the execution lease as taken.
func RecordLeaseAcquired(kind string) {
	m := active()
	if m == nil {
		return
	}
	m.execution.leaseAcquisitions.WithLabelValues(kind).Inc()
	m.execution.leaseHeld.Set(1)
}

// RecordLeaseReleased marks the lease as free and records how long it was held.
func RecordLeaseReleased(kind string, held time.Duration) {
	m := active()
	if m == nil {
		return
	}
	m.execution.leaseHeld.Set(0)
	if held > 0 {
		m.execution.leaseHoldSeconds.WithLabelValues(kind).Observe(held.Seconds())
	}
}

// RecordLeaseRejected records an execution refused because the lease was held.
func RecordLeaseRejected(kind string) {
	if m := active(); m != nil {
		m.execution.leaseRejections.WithLabelValues(kind).Inc()
	}
}

// RecordLeaseExpired records a stale container lease being dropped.
func RecordLeaseExpired() {
	if m := active(); m != nil {
		m.execution.leaseExpirations.Inc()
	}
}

// RecordForceStop records an operator force stop and whether it succeeded.
func RecordForceStop(kind string, err error) {
	m := active()
	if m == nil {
		return
	}
	outcome := OutcomeSuccess
	if err != nil {
		outcome = OutcomeError
	}
	m.execution.forceStops.WithLabelValues(kind, outcome).Inc()
}

// RecordActionExecStarted records an action execution beginning.
func RecordActionExecStarted(kind string) {
	m := active()
	if m == nil {
		return
	}
	m.execution.actionStarted.WithLabelValues(kind).Inc()
	m.execution.actionActive.Inc()
}

// RecordActionExecFinished records an action execution reaching a terminal status.
func RecordActionExecFinished(status string, d time.Duration) {
	m := active()
	if m == nil {
		return
	}
	m.execution.actionFinished.WithLabelValues(status).Inc()
	m.execution.actionActive.Dec()
	if d > 0 {
		m.execution.actionDuration.WithLabelValues(status).Observe(d.Seconds())
	}
}

// RecordFanoutStarted records a plan fan-out beginning.
func RecordFanoutStarted() {
	m := active()
	if m == nil {
		return
	}
	m.execution.fanoutStarted.Inc()
	m.execution.fanoutActive.Inc()
}

// RecordFanoutFinished records a plan fan-out reaching a terminal status.
func RecordFanoutFinished(status string, d time.Duration) {
	m := active()
	if m == nil {
		return
	}
	m.execution.fanoutFinished.WithLabelValues(status).Inc()
	m.execution.fanoutActive.Dec()
	if d > 0 {
		m.execution.fanoutDuration.WithLabelValues(status).Observe(d.Seconds())
	}
}

// RecordFanoutStage records one fan-out stage reaching a terminal status.
func RecordFanoutStage(kind, status string) {
	if m := active(); m != nil {
		m.execution.fanoutStages.WithLabelValues(kind, status).Inc()
	}
}

// RecordFanoutRejected records a fan-out request that was refused.
func RecordFanoutRejected(reason string) {
	if m := active(); m != nil {
		m.execution.fanoutRejections.WithLabelValues(reason).Inc()
	}
}

// AddFanoutBlockers records questions a fan-out suspended on.
func AddFanoutBlockers(n int) {
	m := active()
	if m == nil || n <= 0 {
		return
	}
	m.execution.fanoutBlockers.Add(float64(n))
}

// AddStuckRunsReconciled records runs closed at boot that a previous process
// left marked running.
func AddStuckRunsReconciled(actions, fanouts int) {
	m := active()
	if m == nil {
		return
	}
	if actions > 0 {
		m.execution.stuckReconciled.WithLabelValues("action").Add(float64(actions))
	}
	if fanouts > 0 {
		m.execution.stuckReconciled.WithLabelValues("fanout").Add(float64(fanouts))
	}
}

// RecordPlanStatusTransition records a plan moving between lifecycle statuses.
func RecordPlanStatusTransition(from, to string) {
	m := active()
	if m == nil || from == to {
		return
	}
	m.execution.planTransitions.WithLabelValues(from, to).Inc()
}
