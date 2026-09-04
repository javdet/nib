package llm

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/javdet/nib/internal/metrics"
)

type fakeClient struct {
	assistant AssistantMessage
	tokens    int
	err       error
}

func (f *fakeClient) Complete(context.Context, string, string) (string, error) {
	return "done", f.err
}

func (f *fakeClient) CompleteWithTools(context.Context, []Message, []ToolDef) (AssistantMessage, error) {
	if f.err != nil {
		return AssistantMessage{}, f.err
	}
	return f.assistant, nil
}

func (f *fakeClient) Embed(context.Context, []string) ([][]float32, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	return [][]float32{{0.1}}, f.tokens, nil
}

func setupMetrics(t *testing.T) *metrics.Metrics {
	t.Helper()

	m, err := metrics.Setup(metrics.Config{Enabled: true, Port: 9090, Path: "/metrics", RefreshSeconds: 30})
	if err != nil {
		t.Fatalf("metrics.Setup: %v", err)
	}
	prev := metrics.Default()
	t.Cleanup(func() { metrics.SetDefault(prev) })
	metrics.SetDefault(m)
	return m
}

func llmRequests(t *testing.T, m *metrics.Metrics, operation, model, outcome string) float64 {
	t.Helper()

	families, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, f := range families {
		if f.GetName() != "nib_llm_requests_total" {
			continue
		}
		for _, metric := range f.GetMetric() {
			labels := map[string]string{}
			for _, l := range metric.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			if labels["operation"] == operation && labels["model"] == model && labels["outcome"] == outcome {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func TestMeteredClient_recordsEachOperation(t *testing.T) {
	m := setupMetrics(t)

	inner := &fakeClient{
		assistant: AssistantMessage{Content: "hi", ToolCalls: []ToolCall{{Name: "a"}, {Name: "b"}}},
		tokens:    42,
	}
	client := NewMetered(inner, "gpt-5.6")
	ctx := context.Background()

	if _, err := client.Complete(ctx, "sys", "user"); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if _, err := client.CompleteWithTools(ctx, nil, nil); err != nil {
		t.Fatalf("CompleteWithTools: %v", err)
	}
	if _, _, err := client.Embed(ctx, []string{"text"}); err != nil {
		t.Fatalf("Embed: %v", err)
	}

	for _, op := range []string{metrics.LLMOpComplete, metrics.LLMOpCompleteWithTools, metrics.LLMOpEmbed} {
		if got := llmRequests(t, m, op, "gpt-5.6", metrics.OutcomeSuccess); got != 1 {
			t.Errorf("%s requests = %v, want 1", op, got)
		}
	}
}

func TestMeteredClient_classifiesProviderErrors(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantOutcome string
	}{
		{
			name:        "rate limited",
			err:         &APIError{StatusCode: 429, Op: "openai chat completion"},
			wantOutcome: "rate_limited",
		},
		{
			name:        "server error",
			err:         &APIError{StatusCode: 503, Op: "openai chat completion"},
			wantOutcome: "server_error",
		},
		{
			name:        "client error",
			err:         &APIError{StatusCode: 400, Op: "openai chat completion"},
			wantOutcome: "client_error",
		},
		{
			name:        "deadline exceeded",
			err:         context.DeadlineExceeded,
			wantOutcome: "timeout",
		},
		{
			name:        "canceled",
			err:         context.Canceled,
			wantOutcome: "canceled",
		},
		{
			name: "a plain error is not classified further",
			// The provider message is free text, so it must never become part
			// of a label.
			err:         errors.New("something went wrong: unexpected token at line 4"),
			wantOutcome: metrics.OutcomeError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			m := setupMetrics(t)
			client := NewMetered(&fakeClient{err: tt.err}, "model-x")

			_, err := client.CompleteWithTools(context.Background(), nil, nil)
			if !errors.Is(err, tt.err) {
				t.Fatalf("error = %v, want it to wrap %v", err, tt.err)
			}

			if got := llmRequests(t, m, metrics.LLMOpCompleteWithTools, "model-x", tt.wantOutcome); got != 1 {
				t.Errorf("outcome %q count = %v, want 1", tt.wantOutcome, got)
			}
		})
	}
}

func TestMeteredClient_returnsInnerValuesUnchanged(t *testing.T) {
	setupMetrics(t)

	inner := &fakeClient{
		assistant: AssistantMessage{Content: "answer", ToolCalls: []ToolCall{{ID: "1", Name: "tool"}}},
		tokens:    7,
	}
	client := NewMetered(inner, "model-x")

	msg, err := client.CompleteWithTools(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("CompleteWithTools: %v", err)
	}
	if msg.Content != "answer" || len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Name != "tool" {
		t.Errorf("assistant message was altered: %+v", msg)
	}

	vectors, tokens, err := client.Embed(context.Background(), []string{"a"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vectors) != 1 || tokens != 7 {
		t.Errorf("Embed returned %d vectors and %d tokens, want 1 and 7", len(vectors), tokens)
	}
}

func TestMeteredClient_recordsEmbeddingTokens(t *testing.T) {
	m := setupMetrics(t)

	client := NewMetered(&fakeClient{tokens: 120}, "text-embedding-3-small")
	if _, _, err := client.Embed(context.Background(), []string{"a", "b"}); err != nil {
		t.Fatalf("Embed: %v", err)
	}

	want := `
# HELP nib_llm_embedding_tokens_total Tokens consumed by embedding requests, by model.
# TYPE nib_llm_embedding_tokens_total counter
nib_llm_embedding_tokens_total{model="text-embedding-3-small"} 120
`
	if err := testutil.GatherAndCompare(m.Registry(), stringsReader(want), "nib_llm_embedding_tokens_total"); err != nil {
		t.Error(err)
	}
}

func TestMeteredClient_isANoopWithoutSetup(t *testing.T) {
	prev := metrics.Default()
	t.Cleanup(func() { metrics.SetDefault(prev) })
	metrics.SetDefault(nil)

	client := NewMetered(&fakeClient{tokens: 1}, "model-x")
	if _, err := client.Complete(context.Background(), "s", "u"); err != nil {
		t.Fatalf("Complete without metrics set up: %v", err)
	}
}

func stringsReader(s string) io.Reader { return strings.NewReader(s) }
