package llm

import (
	"context"
	"errors"
	"time"

	"github.com/javdet/nib/internal/metrics"
)

// MeteredClient records request counts, latency and outcomes for every LLM call.
//
// It is a decorator rather than instrumentation inside the providers because
// main.go hands one client to four consumers -- the chat service, the knowledge
// service, the tool-catalog indexer and tool search -- so wrapping at that seam
// covers all of them without touching either provider implementation.
type MeteredClient struct {
	inner Client
	model string
}

// NewMetered wraps inner so its calls are recorded. model is the label value;
// it comes from config rather than from the response, which does not carry it
// on every path.
func NewMetered(inner Client, model string) Client {
	return &MeteredClient{inner: inner, model: model}
}

func (m *MeteredClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	start := time.Now()
	out, err := m.inner.Complete(ctx, systemPrompt, userPrompt)
	metrics.RecordLLMRequest(metrics.LLMOpComplete, m.model, outcomeFor(err), time.Since(start))
	return out, err
}

func (m *MeteredClient) CompleteWithTools(ctx context.Context, messages []Message, tools []ToolDef) (AssistantMessage, error) {
	start := time.Now()
	msg, err := m.inner.CompleteWithTools(ctx, messages, tools)
	metrics.RecordLLMRequest(metrics.LLMOpCompleteWithTools, m.model, outcomeFor(err), time.Since(start))
	if err == nil {
		metrics.AddLLMToolCallsReturned(m.model, len(msg.ToolCalls))
	}
	return msg, err
}

func (m *MeteredClient) Embed(ctx context.Context, texts []string) ([][]float32, int, error) {
	start := time.Now()
	vectors, tokens, err := m.inner.Embed(ctx, texts)
	metrics.RecordLLMRequest(metrics.LLMOpEmbed, m.model, outcomeFor(err), time.Since(start))
	if err == nil {
		metrics.AddLLMEmbeddingTokens(m.model, tokens)
	}
	return vectors, tokens, err
}

// outcomeFor classifies a provider error into a bounded label. The provider's
// own message is deliberately unused: it is free text and would be unbounded.
func outcomeFor(err error) string {
	if err == nil {
		return metrics.OutcomeSuccess
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return metrics.LLMOutcomeForStatus(apiErr.StatusCode)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	return metrics.OutcomeError
}
