package service

import (
	"context"
	"testing"
	"time"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
	"github.com/google/uuid"
)

func TestRunPersistingAgentLoopPublishesToolActivity(t *testing.T) {
	t.Parallel()

	dialogID := uuid.New()
	repo := &retryDialogRepo{
		dialog: domain.Dialog{ID: dialogID, Mode: "discuss"},
		messages: []domain.DialogMessage{
			{DialogID: dialogID, Seq: 0, Role: "system", Content: "You are helpful."},
			{DialogID: dialogID, Seq: 1, Role: "user", Content: "run tool"},
		},
	}
	provider := &retryLLMProvider{
		responses: []llm.AssistantMessage{
			{
				ToolCalls: []llm.ToolCall{{
					ID:        "tc-1",
					Name:      "echo",
					Arguments: `{}`,
				}},
			},
			{Content: "done"},
		},
	}
	svc := NewChatService(provider, nil, nil, nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", t.TempDir(), "", 10, PlanFanoutConfig{}, ActionExecConfig{})

	catalog := newToolCatalog()
	catalog.localHandlers["echo"] = func(_ context.Context, _ map[string]any) (string, error) {
		return `{"ok":true}`, nil
	}

	events, unsub := svc.SubscribeActivity(dialogID)
	defer unsub()

	resp, err := svc.runPersistingAgentLoop(context.Background(), dialogID, "discuss", catalog, loopConfig{})
	if err != nil {
		t.Fatalf("runPersistingAgentLoop: %v", err)
	}
	if resp.Response != "done" {
		t.Fatalf("response = %q, want %q", resp.Response, "done")
	}

	want := []domain.AgentActivityKind{
		domain.ActivityToolsStart,
		domain.ActivityToolsEnd,
		domain.ActivityTurnEnd,
	}
	got := make([]domain.AgentActivityKind, 0, len(want))
	deadline := time.After(2 * time.Second)
	for len(got) < len(want) {
		select {
		case ev, open := <-events:
			if !open {
				t.Fatalf("events closed early with %v", got)
			}
			got = append(got, ev.Kind)
		case <-deadline:
			t.Fatalf("timeout waiting for activity events, got %v", got)
		}
	}

	for i, kind := range want {
		if got[i] != kind {
			t.Fatalf("event %d = %q, want %q (all: %v)", i, got[i], kind, got)
		}
	}
}
