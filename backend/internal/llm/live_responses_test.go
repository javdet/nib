package llm

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveResponsesToolLoop exercises a two-round tool loop against the real
// OpenAI API, including replaying reasoning items. Mocked tests cannot catch
// item-ordering or required-field rejections, which is exactly what breaks when
// the Responses schema changes.
//
// Skipped unless NIB_LIVE_OPENAI=1; it spends tokens on a paid API.
//
//	NIB_LIVE_OPENAI=1 go test ./internal/llm -run TestLiveResponses -v
func TestLiveResponsesToolLoop(t *testing.T) {
	if os.Getenv("NIB_LIVE_OPENAI") != "1" {
		t.Skip("set NIB_LIVE_OPENAI=1 to run the live OpenAI test")
	}
	apiKey := strings.TrimSpace(os.Getenv("LLM_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if apiKey == "" {
		t.Skip("LLM_API_KEY / OPENAI_API_KEY not set")
	}

	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "gpt-5.6"
	}

	// High effort keeps reasoning items in the reply: the model omits them for
	// questions it considers trivial, which would silently skip the replay check.
	provider := NewResponsesProvider(ClientOptions{
		APIKey:          apiKey,
		Model:           model,
		BaseURL:         "https://api.openai.com/v1",
		ReasoningEffort: "high",
		TimeoutSeconds:  300,
	})

	tools := []ToolDef{{
		Name:        "list_pods",
		Description: "List Kubernetes pods in a namespace, with their restart counts and phase.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {"namespace": {"type": "string", "description": "Namespace name"}},
			"required": ["namespace"]
		}`),
	}}

	messages := []Message{
		{Role: "system", Content: "You are an SRE assistant. Use tools when they can answer the question."},
		{Role: "user", Content: "Something is wrong in the prod namespace and users report intermittent 502s. " +
			"Investigate with the tools available and tell me which workload is the likely cause and why."},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	first, err := provider.CompleteWithTools(ctx, messages, tools)
	if err != nil {
		t.Fatalf("round 1: %v", err)
	}
	if len(first.ToolCalls) == 0 {
		t.Fatalf("round 1 returned no tool calls, content = %q", first.Content)
	}
	if len(first.Reasoning) == 0 {
		t.Fatal("round 1 returned no reasoning items; reasoning replay cannot be verified")
	}
	for i, r := range first.Reasoning {
		if r.EncryptedContent == "" {
			t.Errorf("reasoning[%d] has no encrypted content; it cannot be replayed", i)
		}
	}

	messages = append(messages, Message{
		Role:      "assistant",
		Content:   first.Content,
		ToolCalls: first.ToolCalls,
		Reasoning: first.Reasoning,
	})
	for _, tc := range first.ToolCalls {
		messages = append(messages, Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    `{"pods":["api-0","api-1","worker-0"]}`,
		})
	}

	second, err := provider.CompleteWithTools(ctx, messages, tools)
	if err != nil {
		t.Fatalf("round 2 (reasoning replay rejected?): %v", err)
	}
	if second.Content == "" && len(second.ToolCalls) == 0 {
		t.Fatal("round 2 returned neither content nor tool calls")
	}
	t.Logf("round 1 tool call: %s(%s)", first.ToolCalls[0].Name, first.ToolCalls[0].Arguments)
	t.Logf("round 2 answer: %s", second.Content)
}
