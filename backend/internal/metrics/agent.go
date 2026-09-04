package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Tool sources. A call is local when the turn's catalog held a Go handler for
// the name, mcp when it resolved to an MCP route, and unknown when the model
// invented a name that resolved to neither.
const (
	ToolSourceLocal   = "local"
	ToolSourceMCP     = "mcp"
	ToolSourceUnknown = "unknown"
)

type agentMetrics struct {
	turns         *prometheus.CounterVec
	turnDuration  *prometheus.HistogramVec
	rounds        *prometheus.HistogramVec
	toolFailures  *prometheus.CounterVec
	turnsInFlight *prometheus.GaugeVec

	toolCalls        *prometheus.CounterVec
	localToolCalls   *prometheus.CounterVec
	toolCallDuration *prometheus.HistogramVec
}

func newAgentMetrics() agentMetrics {
	return agentMetrics{
		turns: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "agent",
			Name:      "turns_total",
			Help:      "Agent turns by mode and how they ended.",
		}, []string{"mode", "outcome"}),
		turnDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace,
			Subsystem: "agent",
			Name:      "turn_duration_seconds",
			Help:      "Wall-clock duration of an agent turn by mode.",
			Buckets:   runBuckets,
		}, []string{"mode"}),
		rounds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace,
			Subsystem: "agent",
			Name:      "rounds",
			Help:      "Completion rounds a turn burned out of its maxIterations budget.",
			Buckets:   roundBuckets,
		}, []string{"mode"}),
		toolFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "agent",
			Name:      "tool_failures_total",
			Help:      "Tool calls that returned an error back to the model, by mode.",
		}, []string{"mode"}),
		turnsInFlight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: "agent",
			Name:      "turns_in_flight",
			Help:      "Agent turns currently running, by mode.",
		}, []string{"mode"}),
		toolCalls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "agent",
			Name:      "tool_calls_total",
			Help:      "Tool calls by source and outcome. Deliberately carries no tool name.",
		}, []string{"source", "outcome"}),
		// The tool name is safe here and only here: it is emitted solely for a
		// hit in the turn's local handler map, so the value space is the
		// compiled-in localToolRegistrars list. MCP names come from
		// operator-configured servers and a hallucinated name comes from the
		// model, so neither may ever reach a label.
		localToolCalls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "agent",
			Name:      "local_tool_calls_total",
			Help:      "Calls to built-in Go tools by name and outcome.",
		}, []string{"tool", "outcome"}),
		toolCallDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace,
			Subsystem: "agent",
			Name:      "tool_call_duration_seconds",
			Help:      "Tool call latency by source.",
			Buckets:   llmBuckets,
		}, []string{"source"}),
	}
}

func (a agentMetrics) collectors() []prometheus.Collector {
	return []prometheus.Collector{
		a.turns, a.turnDuration, a.rounds, a.toolFailures, a.turnsInFlight,
		a.toolCalls, a.localToolCalls, a.toolCallDuration,
	}
}

// RecordAgentTurn records one finished agent turn. rounds is how many completion
// rounds it ran, outcome one of the Outcome constants.
func RecordAgentTurn(mode, outcome string, rounds int, d time.Duration) {
	m := active()
	if m == nil {
		return
	}
	m.agent.turns.WithLabelValues(mode, outcome).Inc()
	m.agent.turnDuration.WithLabelValues(mode).Observe(d.Seconds())
	if rounds > 0 {
		m.agent.rounds.WithLabelValues(mode).Observe(float64(rounds))
	}
}

// AddAgentToolFailures records tool calls that were handed back to the model as
// errors during one turn.
func AddAgentToolFailures(mode string, n int) {
	m := active()
	if m == nil || n <= 0 {
		return
	}
	m.agent.toolFailures.WithLabelValues(mode).Add(float64(n))
}

// IncAgentTurnsInFlight marks a turn as started.
func IncAgentTurnsInFlight(mode string) {
	if m := active(); m != nil {
		m.agent.turnsInFlight.WithLabelValues(mode).Inc()
	}
}

// DecAgentTurnsInFlight marks a turn as finished.
func DecAgentTurnsInFlight(mode string) {
	if m := active(); m != nil {
		m.agent.turnsInFlight.WithLabelValues(mode).Dec()
	}
}

// RecordToolCall records one tool invocation. tool is recorded by name only when
// source is ToolSourceLocal; for every other source it is ignored.
func RecordToolCall(source, tool string, err error, d time.Duration) {
	m := active()
	if m == nil {
		return
	}
	outcome := OutcomeSuccess
	if err != nil {
		outcome = OutcomeError
	}
	m.agent.toolCalls.WithLabelValues(source, outcome).Inc()
	m.agent.toolCallDuration.WithLabelValues(source).Observe(d.Seconds())
	if source == ToolSourceLocal && tool != "" {
		m.agent.localToolCalls.WithLabelValues(tool, outcome).Inc()
	}
}
