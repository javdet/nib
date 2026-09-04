package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// LLM operation labels.
const (
	LLMOpComplete          = "complete"
	LLMOpCompleteWithTools = "complete_with_tools"
	LLMOpEmbed             = "embed"
)

type llmMetrics struct {
	requests          *prometheus.CounterVec
	duration          *prometheus.HistogramVec
	toolCallsReturned *prometheus.CounterVec
	embeddingTokens   *prometheus.CounterVec
}

func newLLMMetrics() llmMetrics {
	return llmMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "llm",
			Name:      "requests_total",
			Help:      "LLM provider requests by operation, model and outcome.",
		}, []string{"operation", "model", "outcome"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace,
			Subsystem: "llm",
			Name:      "request_duration_seconds",
			Help:      "LLM provider request latency by operation and model.",
			Buckets:   llmBuckets,
		}, []string{"operation", "model"}),
		toolCallsReturned: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "llm",
			Name:      "tool_calls_returned_total",
			Help:      "Tool calls the model asked for, by model.",
		}, []string{"model"}),
		embeddingTokens: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "llm",
			Name:      "embedding_tokens_total",
			Help:      "Tokens consumed by embedding requests, by model.",
		}, []string{"model"}),
	}
}

func (l llmMetrics) collectors() []prometheus.Collector {
	return []prometheus.Collector{l.requests, l.duration, l.toolCallsReturned, l.embeddingTokens}
}

// RecordLLMRequest records one provider call. outcome should come from
// ClassifyLLMError so the label stays bounded.
func RecordLLMRequest(operation, model, outcome string, d time.Duration) {
	m := active()
	if m == nil {
		return
	}
	m.llm.requests.WithLabelValues(operation, model, outcome).Inc()
	m.llm.duration.WithLabelValues(operation, model).Observe(d.Seconds())
}

// AddLLMToolCallsReturned records how many tool calls a completion asked for.
func AddLLMToolCallsReturned(model string, n int) {
	m := active()
	if m == nil || n <= 0 {
		return
	}
	m.llm.toolCallsReturned.WithLabelValues(model).Add(float64(n))
}

// AddLLMEmbeddingTokens records tokens billed by an embedding request.
func AddLLMEmbeddingTokens(model string, tokens int) {
	m := active()
	if m == nil || tokens <= 0 {
		return
	}
	m.llm.embeddingTokens.WithLabelValues(model).Add(float64(tokens))
}

// LLMOutcomeForStatus buckets an HTTP status from the provider into a label.
// The provider's own message is never used: it is free text and would be
// unbounded.
func LLMOutcomeForStatus(status int) string {
	switch {
	case status == 0:
		return OutcomeError
	case status == 408 || status == 504:
		return "timeout"
	case status == 429:
		return "rate_limited"
	case status >= 500:
		return "server_error"
	case status >= 400:
		return "client_error"
	default:
		return OutcomeError
	}
}
