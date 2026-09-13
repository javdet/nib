package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// responsesTestServer captures the request body and replies with body.
func responsesTestServer(t *testing.T, captured *map[string]any, body string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("request path = %q, want /v1/responses", r.URL.Path)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		if err := json.Unmarshal(raw, captured); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

const responseWithReasoningAndToolCall = `{
  "id": "resp_1",
  "object": "response",
  "created_at": 1,
  "model": "gpt-5.6",
  "status": "completed",
  "output": [
    {"type": "reasoning", "id": "rs_1", "encrypted_content": "cipher-blob",
     "summary": [{"type": "summary_text", "text": "checking the cluster"}]},
    {"type": "message", "id": "msg_1", "role": "assistant", "status": "completed",
     "content": [{"type": "output_text", "text": "Looking that up.", "annotations": []}]},
    {"type": "function_call", "id": "fc_1", "call_id": "call_1",
     "name": "list_pods", "arguments": "{\"namespace\":\"prod\"}"}
  ],
  "parallel_tool_calls": true,
  "tool_choice": "auto",
  "tools": []
}`

func newTestResponsesProvider(t *testing.T, baseURL string) *ResponsesProvider {
	t.Helper()
	return NewResponsesProvider(ClientOptions{
		APIKey:          "test-key",
		Model:           "gpt-5.6",
		BaseURL:         baseURL + "/v1",
		ReasoningEffort: "medium",
		TimeoutSeconds:  30,
	})
}

func TestResponsesCompleteWithToolsParsesReasoningAndToolCalls(t *testing.T) {
	t.Parallel()

	var got map[string]any
	srv := responsesTestServer(t, &got, responseWithReasoningAndToolCall)
	provider := newTestResponsesProvider(t, srv.URL)

	asst, err := provider.CompleteWithTools(context.Background(),
		[]Message{
			{Role: "system", Content: "you are an SRE"},
			{Role: "user", Content: "list prod pods"},
		},
		[]ToolDef{{
			Name:        "list_pods",
			Description: "List pods",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"namespace":{"type":"string"}}}`),
		}},
	)
	if err != nil {
		t.Fatalf("CompleteWithTools() error = %v", err)
	}

	if asst.Content != "Looking that up." {
		t.Errorf("Content = %q, want %q", asst.Content, "Looking that up.")
	}
	if len(asst.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(asst.ToolCalls))
	}
	if asst.ToolCalls[0].ID != "call_1" || asst.ToolCalls[0].Name != "list_pods" {
		t.Errorf("ToolCalls[0] = %+v, want call_1/list_pods", asst.ToolCalls[0])
	}
	if asst.ToolCalls[0].Arguments != `{"namespace":"prod"}` {
		t.Errorf("Arguments = %q, want %q", asst.ToolCalls[0].Arguments, `{"namespace":"prod"}`)
	}
	if len(asst.Reasoning) != 1 {
		t.Fatalf("Reasoning len = %d, want 1", len(asst.Reasoning))
	}
	if asst.Reasoning[0].ID != "rs_1" || asst.Reasoning[0].EncryptedContent != "cipher-blob" {
		t.Errorf("Reasoning[0] = %+v, want rs_1/cipher-blob", asst.Reasoning[0])
	}

	// Reasoning traces are only returned when explicitly requested, and only
	// replayable when the provider does not store the response itself.
	if store, ok := got["store"].(bool); !ok || store {
		t.Errorf("store = %v, want false", got["store"])
	}
	include, _ := got["include"].([]any)
	if len(include) != 1 || include[0] != "reasoning.encrypted_content" {
		t.Errorf("include = %v, want [reasoning.encrypted_content]", got["include"])
	}
	reasoning, _ := got["reasoning"].(map[string]any)
	if reasoning["effort"] != "medium" {
		t.Errorf("reasoning.effort = %v, want medium", reasoning["effort"])
	}

	// MCP schemas are not strict-mode compatible.
	tools, _ := got["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools len = %d, want 1", len(tools))
	}
	tool, _ := tools[0].(map[string]any)
	if tool["type"] != "function" || tool["name"] != "list_pods" {
		t.Errorf("tool = %v, want function/list_pods", tool)
	}
	if strict, ok := tool["strict"].(bool); !ok || strict {
		t.Errorf("tool.strict = %v, want false", tool["strict"])
	}
}

func TestResponsesReplaysReasoningBeforeFunctionCalls(t *testing.T) {
	t.Parallel()

	var got map[string]any
	srv := responsesTestServer(t, &got, responseWithReasoningAndToolCall)
	provider := newTestResponsesProvider(t, srv.URL)

	_, err := provider.CompleteWithTools(context.Background(),
		[]Message{
			{Role: "system", Content: "you are an SRE"},
			{Role: "user", Content: "list prod pods"},
			{
				Role:      "assistant",
				Content:   "Looking that up.",
				ToolCalls: []ToolCall{{ID: "call_1", Name: "list_pods", Arguments: `{"namespace":"prod"}`}},
				Reasoning: []ReasoningItem{{ID: "rs_1", EncryptedContent: "cipher-blob", Summary: []string{"checking"}}},
			},
			{Role: "tool", ToolCallID: "call_1", Content: "api-server-0"},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("CompleteWithTools() error = %v", err)
	}

	input, _ := got["input"].([]any)
	types := make([]string, 0, len(input))
	for _, raw := range input {
		item, _ := raw.(map[string]any)
		itemType, ok := item["type"].(string)
		if !ok {
			// Plain role messages may omit an explicit type.
			itemType = "message"
		}
		types = append(types, itemType)
	}

	want := []string{"message", "message", "reasoning", "message", "function_call", "function_call_output"}
	if len(types) != len(want) {
		t.Fatalf("input item types = %v, want %v", types, want)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("input item types = %v, want %v", types, want)
		}
	}

	reasoningItem, _ := input[2].(map[string]any)
	if reasoningItem["id"] != "rs_1" || reasoningItem["encrypted_content"] != "cipher-blob" {
		t.Errorf("reasoning item = %v, want id rs_1 with encrypted_content", reasoningItem)
	}
	if _, ok := reasoningItem["summary"]; !ok {
		t.Errorf("reasoning item = %v, want a summary field (required by the API)", reasoningItem)
	}

	output, _ := input[5].(map[string]any)
	if output["call_id"] != "call_1" || output["output"] != "api-server-0" {
		t.Errorf("function_call_output = %v, want call_1/api-server-0", output)
	}
}

func TestResponsesCompleteWithToolsRejectsUnusableResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "incomplete",
			body: `{
			  "id": "resp_3", "object": "response", "created_at": 1, "model": "gpt-5.6",
			  "status": "incomplete", "incomplete_details": {"reason": "max_output_tokens"},
			  "output": [{"type": "reasoning", "id": "rs_1", "encrypted_content": "cipher", "summary": []}],
			  "parallel_tool_calls": true, "tool_choice": "auto", "tools": []
			}`,
			wantErr: "response incomplete (max_output_tokens)",
		},
		{
			name: "failed",
			body: `{
			  "id": "resp_4", "object": "response", "created_at": 1, "model": "gpt-5.6",
			  "status": "failed", "error": {"code": "server_error", "message": "upstream exploded"},
			  "output": [], "parallel_tool_calls": true, "tool_choice": "auto", "tools": []
			}`,
			wantErr: "response failed: upstream exploded",
		},
		{
			name: "reasoning only",
			body: `{
			  "id": "resp_5", "object": "response", "created_at": 1, "model": "gpt-5.6",
			  "status": "completed",
			  "output": [{"type": "reasoning", "id": "rs_1", "encrypted_content": "cipher", "summary": []}],
			  "parallel_tool_calls": true, "tool_choice": "auto", "tools": []
			}`,
			wantErr: "no output text and no tool calls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got map[string]any
			srv := responsesTestServer(t, &got, tt.body)
			provider := newTestResponsesProvider(t, srv.URL)

			_, err := provider.CompleteWithTools(context.Background(),
				[]Message{{Role: "user", Content: "plan it"}}, nil)
			if err == nil {
				t.Fatalf("CompleteWithTools() error = nil, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestResponsesCompleteUsesInstructions(t *testing.T) {
	t.Parallel()

	const plainResponse = `{
	  "id": "resp_2", "object": "response", "created_at": 1, "model": "gpt-5.6", "status": "completed",
	  "output": [{"type": "message", "id": "msg_1", "role": "assistant", "status": "completed",
	    "content": [{"type": "output_text", "text": "pong", "annotations": []}]}],
	  "parallel_tool_calls": true, "tool_choice": "auto", "tools": []
	}`

	var got map[string]any
	srv := responsesTestServer(t, &got, plainResponse)
	provider := newTestResponsesProvider(t, srv.URL)

	out, err := provider.Complete(context.Background(), "be terse", "ping")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if out != "pong" {
		t.Errorf("Complete() = %q, want pong", out)
	}
	if got["instructions"] != "be terse" {
		t.Errorf("instructions = %v, want 'be terse'", got["instructions"])
	}
	if got["input"] != "ping" {
		t.Errorf("input = %v, want ping", got["input"])
	}
}

// An incomplete response hit the output cap: it was generated and billed, so
// its usage must reach the caller even though the call is reported as an error.
// Dropping it would hide exactly the spend the statistics exist to surface.
func TestCompleteWithToolsCarriesUsageOnIncompleteResponse(t *testing.T) {
	body := `{
		"id": "resp_1",
		"model": "gpt-5.6",
		"status": "incomplete",
		"incomplete_details": {"reason": "max_output_tokens"},
		"output": [],
		"usage": {
			"input_tokens": 1200,
			"input_tokens_details": {"cached_tokens": 400},
			"output_tokens": 800,
			"output_tokens_details": {"reasoning_tokens": 700},
			"total_tokens": 2000
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	provider := NewResponsesProvider(ClientOptions{
		APIKey:  "test",
		Model:   "gpt-5.6",
		BaseURL: srv.URL,
	})

	msg, err := provider.CompleteWithTools(context.Background(),
		[]Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("CompleteWithTools() error = nil, want an incomplete-response error")
	}
	if msg.Usage.TotalTokens != 2000 {
		t.Errorf("Usage.TotalTokens = %d, want 2000 carried out alongside the error", msg.Usage.TotalTokens)
	}
	if msg.Usage.PromptTokens != 1200 || msg.Usage.CompletionTokens != 800 {
		t.Errorf("Usage prompt/completion = %d/%d, want 1200/800",
			msg.Usage.PromptTokens, msg.Usage.CompletionTokens)
	}
	if msg.Usage.CachedPromptTokens != 400 || msg.Usage.ReasoningTokens != 700 {
		t.Errorf("Usage cached/reasoning = %d/%d, want 400/700",
			msg.Usage.CachedPromptTokens, msg.Usage.ReasoningTokens)
	}
	if msg.Usage.CostUSD != nil {
		t.Errorf("Usage.CostUSD = %v, want nil: this provider reports no cost", *msg.Usage.CostUSD)
	}
}
