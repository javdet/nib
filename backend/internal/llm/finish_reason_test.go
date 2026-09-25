package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestFinishReasonHandling covers the stop reasons that used to be invisible.
// The agent loops read a reply with no tool calls as the turn's final answer,
// so a truncated or filtered round was persisted as a finished one.
func TestFinishReasonHandling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantErr  error
		wantOK   bool
		wantText string
	}{
		{
			name: "length truncation is an error",
			body: `{"id":"c1","model":"m","choices":[{"index":0,"finish_reason":"length",` +
				`"message":{"role":"assistant","content":"half a pl"}}],` +
				`"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
			wantErr: ErrTruncated,
		},
		{
			name: "content filter is a refusal",
			body: `{"id":"c1","model":"m","choices":[{"index":0,"finish_reason":"content_filter",` +
				`"message":{"role":"assistant","content":""}}]}`,
			wantErr: ErrRefused,
		},
		{
			name: "explicit refusal text",
			body: `{"id":"c1","model":"m","choices":[{"index":0,"finish_reason":"stop",` +
				`"message":{"role":"assistant","content":"","refusal":"I cannot help with that"}}]}`,
			wantErr: ErrRefused,
		},
		{
			name: "no content and no tool calls",
			body: `{"id":"c1","model":"m","choices":[{"index":0,"finish_reason":"stop",` +
				`"message":{"role":"assistant","content":""}}]}`,
			wantErr: ErrNoOutput,
		},
		{
			name: "uppercase STOP is accepted",
			body: `{"id":"c1","model":"m","choices":[{"index":0,"finish_reason":"STOP",` +
				`"message":{"role":"assistant","content":"fine"}}]}`,
			wantOK:   true,
			wantText: "fine",
		},
		{
			name: "absent finish_reason is accepted",
			body: `{"id":"c1","model":"m","choices":[{"index":0,` +
				`"message":{"role":"assistant","content":"fine"}}]}`,
			wantOK:   true,
			wantText: "fine",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := newChatFake(t, tc.body)

			got, err := fake.provider().CompleteWithTools(context.Background(),
				[]Message{{Role: "user", Content: "hi"}}, nil)

			if tc.wantOK {
				if err != nil {
					t.Fatalf("CompleteWithTools() error = %v, want nil", err)
				}
				if got.Content != tc.wantText {
					t.Errorf("Content = %q, want %q", got.Content, tc.wantText)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("CompleteWithTools() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// TestTruncationCarriesUsage keeps the billed tokens attributable. They were
// generated whether or not the answer was usable, and the responses endpoint
// already reports them the same way.
func TestTruncationCarriesUsage(t *testing.T) {
	t.Parallel()

	fake := newChatFake(t, `{"id":"c1","model":"m","choices":[{"index":0,"finish_reason":"length",`+
		`"message":{"role":"assistant","content":"cut"}}],`+
		`"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)

	got, err := fake.provider().CompleteWithTools(context.Background(),
		[]Message{{Role: "user", Content: "hi"}}, nil)
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("error = %v, want ErrTruncated", err)
	}
	if got.Usage.TotalTokens != 15 {
		t.Errorf("Usage.TotalTokens = %d, want 15", got.Usage.TotalTokens)
	}
}

// TestCompleteChecksStopReason covers the plain completion too: it backs chat
// naming and summaries, where a truncated answer is silently wrong.
func TestCompleteChecksStopReason(t *testing.T) {
	t.Parallel()

	fake := newChatFake(t, `{"id":"c1","model":"m","choices":[{"index":0,"finish_reason":"length",`+
		`"message":{"role":"assistant","content":"tru"}}]}`)

	_, err := fake.provider().Complete(context.Background(), "sys", "user")
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("Complete() error = %v, want ErrTruncated", err)
	}
	if !strings.Contains(err.Error(), "openai chat completion") {
		t.Errorf("error = %v, want it to name the operation", err)
	}
}
