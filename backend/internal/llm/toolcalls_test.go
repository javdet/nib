package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// chatFake serves one canned /chat/completions body and records the request
// bodies it received, so a test can assert both what was parsed out of a
// provider's dialect and what was sent back on the following round.
type chatFake struct {
	srv    *httptest.Server
	bodies []map[string]any
}

func newChatFake(t *testing.T, responseBody string) *chatFake {
	t.Helper()
	f := &chatFake{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.bodies = append(f.bodies, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *chatFake) provider() *OpenAIProvider {
	return NewCompatProvider(ClientOptions{
		APIKey:  "test-key",
		Model:   "test-model",
		BaseURL: f.srv.URL,
	})
}

func completionBody(toolCallJSON string) string {
	return `{"id":"c1","model":"test-model","choices":[{"index":0,"finish_reason":"tool_calls",` +
		`"message":{"role":"assistant","content":"","tool_calls":[` + toolCallJSON + `]}}],` +
		`"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
}

// TestToolCallDialects covers the response shapes the OpenAI-compatible
// gateways actually emit. Before the extraction stopped switching on "type",
// every one of the type-less variants produced zero tool calls and an empty
// answer, which the agent loops read as a finished turn.
func TestToolCallDialects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		toolCall  string
		wantName  string
		wantArgs  string
		wantID    string
		idIsExact bool
	}{
		{
			name:      "openai regression: fully typed",
			toolCall:  `{"id":"call_abc","type":"function","function":{"name":"get_skill","arguments":"{\"name\":\"go\"}"}}`,
			wantName:  "get_skill",
			wantArgs:  `{"name":"go"}`,
			wantID:    "call_abc",
			idIsExact: true,
		},
		{
			name:      "gemini: no type field",
			toolCall:  `{"id":"call_g1","function":{"name":"get_skill","arguments":"{\"name\":\"go\"}"}}`,
			wantName:  "get_skill",
			wantArgs:  `{"name":"go"}`,
			wantID:    "call_g1",
			idIsExact: true,
		},
		{
			name:      "qwen/kimi: no type and arguments as an object",
			toolCall:  `{"id":"call_q1","function":{"name":"get_skill","arguments":{"name":"go"}}}`,
			wantName:  "get_skill",
			wantArgs:  `{"name":"go"}`,
			wantID:    "call_q1",
			idIsExact: true,
		},
		{
			name:     "grok: no id",
			toolCall: `{"type":"function","function":{"name":"get_skill","arguments":"{\"name\":\"go\"}"}}`,
			wantName: "get_skill",
			wantArgs: `{"name":"go"}`,
		},
		{
			name:      "deepseek: index field alongside the call",
			toolCall:  `{"index":0,"id":"call_d1","type":"function","function":{"name":"get_skill","arguments":"{\"name\":\"go\"}"}}`,
			wantName:  "get_skill",
			wantArgs:  `{"name":"go"}`,
			wantID:    "call_d1",
			idIsExact: true,
		},
		{
			name:     "missing arguments defaults to an empty object",
			toolCall: `{"id":"call_e1","type":"function","function":{"name":"get_skill"}}`,
			wantName: "get_skill",
			wantArgs: "{}",
			wantID:   "call_e1", idIsExact: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := newChatFake(t, completionBody(tc.toolCall))

			got, err := fake.provider().CompleteWithTools(context.Background(),
				[]Message{{Role: "user", Content: "hi"}}, nil)
			if err != nil {
				t.Fatalf("CompleteWithTools() error = %v", err)
			}
			if len(got.ToolCalls) != 1 {
				t.Fatalf("got %d tool calls, want 1 (dialect was dropped)", len(got.ToolCalls))
			}
			call := got.ToolCalls[0]
			if call.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", call.Name, tc.wantName)
			}
			if call.Arguments != tc.wantArgs {
				t.Errorf("Arguments = %q, want %q", call.Arguments, tc.wantArgs)
			}
			if tc.idIsExact && call.ID != tc.wantID {
				t.Errorf("ID = %q, want %q", call.ID, tc.wantID)
			}
			// Whatever the provider sent, the id has to be usable as a
			// tool_call_id on the next round.
			if strings.TrimSpace(call.ID) == "" {
				t.Error("ID is empty; the tool result would have nothing to pair with")
			}
		})
	}
}

// TestToolCallCustomTypeSkipped guards the one variant that is genuinely not a
// function call. nib publishes no custom tools, so it must be ignored rather
// than mistaken for one.
func TestToolCallCustomTypeSkipped(t *testing.T) {
	t.Parallel()

	fake := newChatFake(t, `{"id":"c1","model":"m","choices":[{"index":0,"finish_reason":"stop",`+
		`"message":{"role":"assistant","content":"done","tool_calls":[`+
		`{"id":"call_c","type":"custom","custom":{"name":"x","input":"y"}}]}}]}`)

	got, err := fake.provider().CompleteWithTools(context.Background(),
		[]Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("CompleteWithTools() error = %v", err)
	}
	if len(got.ToolCalls) != 0 {
		t.Errorf("got %d tool calls, want 0", len(got.ToolCalls))
	}
	if got.Content != "done" {
		t.Errorf("Content = %q, want %q", got.Content, "done")
	}
}

// TestSyntheticToolCallIDIsEchoed checks the whole round trip for a provider
// that omits ids: the synthesized id must come back as the tool_call_id, or
// the follow-up request pairs a result with nothing.
func TestSyntheticToolCallIDIsEchoed(t *testing.T) {
	t.Parallel()

	fake := newChatFake(t, completionBody(
		`{"type":"function","function":{"name":"get_skill","arguments":"{}"}}`))
	provider := fake.provider()

	first, err := provider.CompleteWithTools(context.Background(),
		[]Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("CompleteWithTools() error = %v", err)
	}
	call := first.ToolCalls[0]

	_, err = provider.CompleteWithTools(context.Background(), []Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", ToolCalls: first.ToolCalls},
		{Role: "tool", ToolCallID: call.ID, Content: "result"},
	}, nil)
	if err != nil {
		t.Fatalf("second CompleteWithTools() error = %v", err)
	}

	messages, _ := fake.bodies[1]["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("got %d messages in the replay, want 3", len(messages))
	}
	toolMsg, _ := messages[2].(map[string]any)
	if got := toolMsg["tool_call_id"]; got != call.ID {
		t.Errorf("tool_call_id = %v, want %q", got, call.ID)
	}

	// The assistant message must carry a content key even though it is empty.
	asst, _ := messages[1].(map[string]any)
	if _, has := asst["content"]; !has {
		t.Error("assistant message has no content key; gateways that require one would reject it")
	}
}

// TestReasoningContentCapturedNotReplayed covers both halves of the DeepSeek
// and Qwen reasoning channel: it must be read off the reply, and it must not
// be sent back, which DeepSeek rejects.
func TestReasoningContentCapturedNotReplayed(t *testing.T) {
	t.Parallel()

	fake := newChatFake(t, `{"id":"c1","model":"m","choices":[{"index":0,"finish_reason":"stop",`+
		`"message":{"role":"assistant","content":"the answer","reasoning_content":"step one, step two"}}]}`)
	provider := fake.provider()

	got, err := provider.CompleteWithTools(context.Background(),
		[]Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("CompleteWithTools() error = %v", err)
	}
	if len(got.Reasoning) != 1 || len(got.Reasoning[0].Summary) != 1 {
		t.Fatalf("Reasoning = %+v, want one item with one summary", got.Reasoning)
	}
	if got.Reasoning[0].Summary[0] != "step one, step two" {
		t.Errorf("Summary = %q, want %q", got.Reasoning[0].Summary[0], "step one, step two")
	}

	_, err = provider.CompleteWithTools(context.Background(), []Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "the answer", Reasoning: got.Reasoning},
	}, nil)
	if err != nil {
		t.Fatalf("second CompleteWithTools() error = %v", err)
	}
	if raw, _ := json.Marshal(fake.bodies[1]); strings.Contains(string(raw), "step one") {
		t.Errorf("reasoning was replayed into the request body: %s", raw)
	}
}

// TestOpenRouterReasoningFieldCaptured covers the other spelling of the same
// channel.
func TestOpenRouterReasoningFieldCaptured(t *testing.T) {
	t.Parallel()

	fake := newChatFake(t, `{"id":"c1","model":"m","choices":[{"index":0,"finish_reason":"stop",`+
		`"message":{"role":"assistant","content":"answer","reasoning":"thought"}}]}`)

	got, err := fake.provider().CompleteWithTools(context.Background(),
		[]Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("CompleteWithTools() error = %v", err)
	}
	if len(got.Reasoning) != 1 {
		t.Fatalf("Reasoning = %+v, want one item", got.Reasoning)
	}
}

// TestReasoningEffortOmittedWhenUnset guards the parameter that several models
// reject outright.
func TestReasoningEffortOmittedWhenUnset(t *testing.T) {
	t.Parallel()

	body := `{"id":"c1","model":"m","choices":[{"index":0,"finish_reason":"stop",` +
		`"message":{"role":"assistant","content":"hi"}}]}`

	t.Run("unset", func(t *testing.T) {
		t.Parallel()
		fake := newChatFake(t, body)
		if _, err := fake.provider().CompleteWithTools(context.Background(),
			[]Message{{Role: "user", Content: "hi"}}, nil); err != nil {
			t.Fatalf("CompleteWithTools() error = %v", err)
		}
		if _, has := fake.bodies[0]["reasoning_effort"]; has {
			t.Error("reasoning_effort was sent although none is configured")
		}
	})

	t.Run("set", func(t *testing.T) {
		t.Parallel()
		fake := newChatFake(t, body)
		provider := NewCompatProvider(ClientOptions{
			APIKey: "k", Model: "m", BaseURL: fake.srv.URL, ReasoningEffort: "high",
		})
		if _, err := provider.CompleteWithTools(context.Background(),
			[]Message{{Role: "user", Content: "hi"}}, nil); err != nil {
			t.Fatalf("CompleteWithTools() error = %v", err)
		}
		if got := fake.bodies[0]["reasoning_effort"]; got != "high" {
			t.Errorf("reasoning_effort = %v, want %q", got, "high")
		}
	})
}

// TestReasoningEffortHintOnBadRequest checks the operator gets told which knob
// to clear when a model rejects the parameter.
func TestReasoningEffortHintOnBadRequest(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"unsupported parameter reasoning_effort"}}`))
	}))
	t.Cleanup(srv.Close)

	provider := NewCompatProvider(ClientOptions{
		APIKey: "k", Model: "m", BaseURL: srv.URL, ReasoningEffort: "high",
	})
	_, err := provider.CompleteWithTools(context.Background(),
		[]Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("CompleteWithTools() error = nil, want a 400")
	}
	if !strings.Contains(err.Error(), "llm.reasoningEffort") {
		t.Errorf("error = %v, want it to name llm.reasoningEffort", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Errorf("error does not unwrap to *APIError, which the HTTP layer maps on")
	}
}
