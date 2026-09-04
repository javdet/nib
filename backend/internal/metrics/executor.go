package metrics

import (
	"math"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Executor entry points. Run is driven by the LLM tool, RunAction by an
// operator pressing Execute action on a code step.
const (
	ExecutorEntrypointRun    = "run"
	ExecutorEntrypointAction = "run_action"
)

// Webhook outcomes.
const (
	WebhookAccepted     = "accepted"
	WebhookUnauthorized = "unauthorized"
	WebhookBadRequest   = "bad_request"
	WebhookError        = "error"
)

type executorMetrics struct {
	runs        *prometheus.CounterVec
	runDuration *prometheus.HistogramVec
	stops       *prometheus.CounterVec

	webhooks      *prometheus.CounterVec
	results       *prometheus.CounterVec
	agentDuration prometheus.Histogram
	agentTurns    prometheus.Histogram
	agentCost     prometheus.Counter
}

func newExecutorMetrics() executorMetrics {
	return executorMetrics{
		runs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "executor",
			Name:      "runs_total",
			Help:      "Agent container launches by entry point, executor type, platform and outcome.",
		}, []string{"entrypoint", "type", "platform", "outcome"}),
		// Launch latency, not the container's lifetime: RunAction returns as
		// soon as the job is created and the container reports back by webhook.
		runDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace,
			Subsystem: "executor",
			Name:      "run_duration_seconds",
			Help:      "Time to launch an agent container, by entry point.",
			Buckets:   llmBuckets,
		}, []string{"entrypoint"}),
		stops: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "executor",
			Name:      "stops_total",
			Help:      "Attempts to stop a running agent container.",
		}, []string{"outcome"}),
		webhooks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "agent_runner",
			Name:      "webhooks_total",
			Help:      "Agent-runner webhook calls by outcome.",
		}, []string{"outcome"}),
		results: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "agent_runner",
			Name:      "results_total",
			Help:      "Agent-runner results by reported status and whether a branch was pushed.",
		}, []string{"status", "pushed"}),
		agentDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: Namespace,
			Subsystem: "agent_runner",
			Name:      "run_duration_seconds",
			Help:      "Duration a container agent reported for its own run.",
			Buckets:   runBuckets,
		}),
		agentTurns: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: Namespace,
			Subsystem: "agent_runner",
			Name:      "turns",
			Help:      "Turns a container agent reported for its own run.",
			Buckets:   turnBuckets,
		}),
		agentCost: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "agent_runner",
			Name:      "cost_usd_total",
			Help:      "Cumulative USD cost reported by container agents.",
		}),
	}
}

func (e executorMetrics) collectors() []prometheus.Collector {
	return []prometheus.Collector{
		e.runs, e.runDuration, e.stops,
		e.webhooks, e.results, e.agentDuration, e.agentTurns, e.agentCost,
	}
}

// RecordExecutorRun records one attempt to launch an agent container.
func RecordExecutorRun(entrypoint, execType, platform string, err error, d time.Duration) {
	m := active()
	if m == nil {
		return
	}
	outcome := OutcomeSuccess
	if err != nil {
		outcome = OutcomeError
	}
	m.executor.runs.WithLabelValues(entrypoint, execType, platform, outcome).Inc()
	m.executor.runDuration.WithLabelValues(entrypoint).Observe(d.Seconds())
}

// RecordExecutorStop records an attempt to stop a running container.
func RecordExecutorStop(err error) {
	m := active()
	if m == nil {
		return
	}
	outcome := OutcomeSuccess
	if err != nil {
		outcome = OutcomeError
	}
	m.executor.stops.WithLabelValues(outcome).Inc()
}

// RecordAgentRunnerWebhook records a webhook call by outcome.
func RecordAgentRunnerWebhook(outcome string) {
	if m := active(); m != nil {
		m.executor.webhooks.WithLabelValues(outcome).Inc()
	}
}

// RecordAgentRunnerResult records the usage a container agent reported.
//
// Every value here arrives from a container over the network, so each is
// range-checked: Counter.Add panics on a negative, and a NaN would poison the
// series for the life of the process.
func RecordAgentRunnerResult(status, pushed string, durationMS, turns int, costUSD float64) {
	m := active()
	if m == nil {
		return
	}
	m.executor.results.WithLabelValues(status, pushed).Inc()
	if durationMS > 0 {
		m.executor.agentDuration.Observe(float64(durationMS) / 1000)
	}
	if turns > 0 {
		m.executor.agentTurns.Observe(float64(turns))
	}
	if costUSD > 0 && !math.IsNaN(costUSD) && !math.IsInf(costUSD, 0) {
		m.executor.agentCost.Add(costUSD)
	}
}

// NormalizeAgentRunnerStatus maps the status a container reported onto a known
// set. It arrives as free text over the network, so it must never reach a label
// unfiltered.
func NormalizeAgentRunnerStatus(status string) string {
	switch status {
	case "success", "failed", "timeout", "error":
		return status
	default:
		return "other"
	}
}
