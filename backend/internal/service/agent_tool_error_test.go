package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
	"github.com/google/uuid"
)

func TestRepairOrphanToolCalls(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    []llm.Message
		wantLen  int
		wantIDs  []string
		wantText string
	}{
		{
			name: "missing result",
			input: []llm.Message{
				{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "search"}}},
				{Role: "user", Content: "next"},
			},
			wantLen:  3,
			wantIDs:  []string{"call_1"},
			wantText: "tool result missing",
		},
		{
			name: "partial batch",
			input: []llm.Message{
				{Role: "assistant", ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: "search"},
					{ID: "call_2", Name: "read"},
				}},
				{Role: "tool", ToolCallID: "call_1", Content: "ok"},
				{Role: "user", Content: "next"},
			},
			wantLen:  4,
			wantIDs:  []string{"call_2"},
			wantText: "tool result missing",
		},
		{
			name: "complete transcript",
			input: []llm.Message{
				{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "search"}}},
				{Role: "tool", ToolCallID: "call_1", Content: "ok"},
			},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := repairOrphanToolCalls(tt.input)
			if len(got) != tt.wantLen {
				t.Fatalf("len = %d, want %d: %+v", len(got), tt.wantLen, got)
			}
			if len(tt.wantIDs) == 0 {
				return
			}
			seen := make(map[string]string)
			for _, m := range got {
				if m.Role == "tool" && strings.Contains(m.Content, tt.wantText) {
					seen[m.ToolCallID] = m.Content
				}
			}
			for _, id := range tt.wantIDs {
				if _, ok := seen[id]; !ok {
					t.Fatalf("missing synthetic tool result for %q in %+v", id, got)
				}
			}
		})
	}
}

func TestDialogMessagesToLLM_repairsOrphanToolCalls(t *testing.T) {
	t.Parallel()

	calls, err := marshalToolCalls([]llm.ToolCall{{ID: "call_1", Name: "search", Arguments: `{}`}})
	if err != nil {
		t.Fatalf("marshalToolCalls: %v", err)
	}

	msgs := []domain.DialogMessage{
		{Seq: 0, Role: "assistant", ToolCalls: calls},
		{Seq: 1, Role: "user", Content: "continue"},
	}

	got, err := dialogMessagesToLLM(msgs)
	if err != nil {
		t.Fatalf("dialogMessagesToLLM: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[1].Role != "tool" || got[1].ToolCallID != "call_1" {
		t.Fatalf("expected synthetic tool message, got %+v", got[1])
	}
	if !strings.Contains(got[1].Content, "tool result missing") {
		t.Fatalf("content = %q, want missing-result text", got[1].Content)
	}
}

func TestRunPersistingAgentLoop_recoversFromToolFailure(t *testing.T) {
	t.Parallel()

	dialogID := uuid.New()
	repo := &retryDialogRepo{
		dialog: domain.Dialog{ID: dialogID, Mode: "decompose"},
		messages: []domain.DialogMessage{
			{DialogID: dialogID, Seq: 0, Role: "system", Content: "You decompose tasks."},
			{DialogID: dialogID, Seq: 1, Role: "user", Content: "plan OPS-2039"},
		},
	}

	provider := &retryLLMProvider{
		responses: []llm.AssistantMessage{
			{ToolCalls: []llm.ToolCall{{
				ID:        "call_fail",
				Name:      "failing_tool",
				Arguments: `{}`,
			}}},
			{Content: "Recovered after tool failure."},
		},
	}

	svc := NewChatService(provider, nil, nil, nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", t.TempDir(), "", 10, PlanFanoutConfig{}, ActionExecConfig{})

	catalog := newToolCatalog()
	catalog.localHandlers["failing_tool"] = func(_ context.Context, _ map[string]any) (string, error) {
		return "", fmt.Errorf("404: Page not found")
	}

	resp, err := svc.runPersistingAgentLoop(context.Background(), dialogID, "decompose", catalog, loopConfig{})
	if err != nil {
		t.Fatalf("runPersistingAgentLoop: %v", err)
	}
	if resp.Response != "Recovered after tool failure." {
		t.Fatalf("response = %q", resp.Response)
	}

	msgs, err := repo.ListMessages(context.Background(), dialogID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}

	var toolMsg *domain.DialogMessage
	for i := range msgs {
		if msgs[i].Role == "tool" && msgs[i].ToolCallID == "call_fail" {
			toolMsg = &msgs[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatal("expected persisted tool message for failed call")
	}
	if !strings.Contains(toolMsg.Content, `"isError":true`) || !strings.Contains(toolMsg.Content, "failing_tool") {
		t.Fatalf("tool content = %q", toolMsg.Content)
	}

	secondRound := provider.seen[1]
	found := false
	for _, m := range secondRound {
		if m.Role == "tool" && m.ToolCallID == "call_fail" && strings.Contains(m.Content, "404: Page not found") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("provider did not see tool error in second round: %+v", secondRound)
	}
}

func TestRunPersistingAgentLoop_tooManyToolFailures(t *testing.T) {
	t.Parallel()

	dialogID := uuid.New()
	repo := &retryDialogRepo{
		dialog: domain.Dialog{ID: dialogID, Mode: "decompose"},
		messages: []domain.DialogMessage{
			{DialogID: dialogID, Seq: 0, Role: "user", Content: "go"},
		},
	}

	responses := make([]llm.AssistantMessage, maxToolFailuresPerTurn+1)
	for i := range responses {
		responses[i] = llm.AssistantMessage{ToolCalls: []llm.ToolCall{{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      "failing_tool",
			Arguments: `{}`,
		}}}
	}

	provider := &retryLLMProvider{responses: responses}
	svc := NewChatService(provider, nil, nil, nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", t.TempDir(), "", 20, PlanFanoutConfig{}, ActionExecConfig{})

	catalog := newToolCatalog()
	catalog.localHandlers["failing_tool"] = func(_ context.Context, _ map[string]any) (string, error) {
		return "", errors.New("tool broke")
	}

	_, err := svc.runPersistingAgentLoop(context.Background(), dialogID, "decompose", catalog, loopConfig{})
	if !errors.Is(err, ErrTooManyToolFailures) {
		t.Fatalf("err = %v, want ErrTooManyToolFailures", err)
	}

	msgs, err := repo.ListMessages(context.Background(), dialogID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}

	toolIDs := make(map[string]struct{})
	for _, m := range msgs {
		if m.Role == "tool" {
			toolIDs[m.ToolCallID] = struct{}{}
		}
	}
	for i := 0; i < maxToolFailuresPerTurn+1; i++ {
		id := fmt.Sprintf("call_%d", i)
		if _, ok := toolIDs[id]; !ok {
			t.Fatalf("missing persisted tool result for %s", id)
		}
	}
}

func TestRunPersistingAgentLoop_fatalOnCancelledContext(t *testing.T) {
	t.Parallel()

	dialogID := uuid.New()
	repo := &retryDialogRepo{
		dialog: domain.Dialog{ID: dialogID, Mode: "decompose"},
		messages: []domain.DialogMessage{
			{DialogID: dialogID, Seq: 0, Role: "user", Content: "go"},
		},
	}

	provider := &retryLLMProvider{
		responses: []llm.AssistantMessage{{
			ToolCalls: []llm.ToolCall{{
				ID:        "call_cancel",
				Name:      "failing_tool",
				Arguments: `{}`,
			}},
		}},
	}
	svc := NewChatService(provider, nil, nil, nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", t.TempDir(), "", 10, PlanFanoutConfig{}, ActionExecConfig{})

	catalog := newToolCatalog()
	catalog.localHandlers["failing_tool"] = func(_ context.Context, _ map[string]any) (string, error) {
		return "", context.Canceled
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.runPersistingAgentLoop(ctx, dialogID, "decompose", catalog, loopConfig{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if errors.Is(err, ErrTooManyToolFailures) {
		t.Fatalf("unexpected ErrTooManyToolFailures: %v", err)
	}
}
