package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
)

type retryDialogRepo struct {
	dialog   domain.Dialog
	messages []domain.DialogMessage
	nextID   int64
}

func (r *retryDialogRepo) CreateDialog(_ context.Context, mode, title string, parentID *uuid.UUID) (domain.Dialog, error) {
	r.dialog = domain.Dialog{ID: uuid.New(), Mode: mode, Title: title, ParentID: parentID}
	return r.dialog, nil
}

func (r *retryDialogRepo) ListChildren(_ context.Context, _ uuid.UUID) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *retryDialogRepo) ListPlanChildren(_ context.Context, _ []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	return map[uuid.UUID]uuid.UUID{}, nil
}

func (r *retryDialogRepo) ListRecentDialogs(_ context.Context, _, _ int) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *retryDialogRepo) CountDialogs(_ context.Context) (int, error) {
	return 0, nil
}

func (r *retryDialogRepo) ListDialogsByMode(_ context.Context, _ string, _, _ int) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *retryDialogRepo) CountDialogsByMode(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (r *retryDialogRepo) ListAllRecentDialogs(_ context.Context, _, _ int) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *retryDialogRepo) CountAllDialogs(_ context.Context) (int, error) {
	return 0, nil
}

func (r *retryDialogRepo) SearchDialogs(_ context.Context, _, _ string, _, _ int) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *retryDialogRepo) CountDialogsSearch(_ context.Context, _, _ string) (int, error) {
	return 0, nil
}

func (r *retryDialogRepo) GetDialog(_ context.Context, id uuid.UUID) (domain.Dialog, error) {
	if r.dialog.ID != id {
		return domain.Dialog{}, repository.ErrNotFound
	}
	return r.dialog, nil
}

func (r *retryDialogRepo) UpdateTitle(_ context.Context, _ uuid.UUID, title string) error {
	r.dialog.Title = title
	return nil
}

func (r *retryDialogRepo) SetDialogTaskID(_ context.Context, _ uuid.UUID, taskID *string) error {
	r.dialog.TaskID = taskID
	return nil
}

func (r *retryDialogRepo) SetDialogSubjects(_ context.Context, _ uuid.UUID, subjects []string) error {
	r.dialog.Subjects = subjects
	return nil
}

func (r *retryDialogRepo) SetDialogCategories(_ context.Context, _ uuid.UUID, categories []string) error {
	r.dialog.Categories = categories
	return nil
}

func (r *retryDialogRepo) SetDialogPinned(_ context.Context, _ uuid.UUID, pinned bool) error {
	r.dialog.Pinned = pinned
	return nil
}

func (r *retryDialogRepo) ListPinnedDialogs(_ context.Context) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *retryDialogRepo) DeleteDialog(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (r *retryDialogRepo) ListMessages(_ context.Context, _ uuid.UUID) ([]domain.DialogMessage, error) {
	out := make([]domain.DialogMessage, len(r.messages))
	copy(out, r.messages)
	return out, nil
}

func (r *retryDialogRepo) AppendMessage(_ context.Context, dialogID uuid.UUID, msg domain.DialogMessage) (domain.DialogMessage, error) {
	r.nextID++
	seq := len(r.messages)
	msg.ID = r.nextID
	msg.DialogID = dialogID
	msg.Seq = seq
	r.messages = append(r.messages, msg)
	return msg, nil
}

func (r *retryDialogRepo) DeleteMessagesAfterSeq(_ context.Context, _ uuid.UUID, afterSeq int) error {
	filtered := r.messages[:0]
	for _, m := range r.messages {
		if m.Seq <= afterSeq {
			filtered = append(filtered, m)
		}
	}
	r.messages = filtered
	return nil
}

type retryLLMProvider struct {
	responses []llm.AssistantMessage
	calls     int
	seen      [][]llm.Message
}

func (p *retryLLMProvider) Complete(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

func (p *retryLLMProvider) CompleteWithTools(_ context.Context, messages []llm.Message, _ []llm.ToolDef) (llm.AssistantMessage, error) {
	p.seen = append(p.seen, messages)
	if p.calls >= len(p.responses) {
		return llm.AssistantMessage{Content: "fallback"}, nil
	}
	resp := p.responses[p.calls]
	p.calls++
	return resp, nil
}

func TestRunPersistingAgentLoop_remindsPlanTurnToCreateActionPlan(t *testing.T) {
	t.Parallel()

	dialogID := uuid.New()
	repo := &retryDialogRepo{
		dialog: domain.Dialog{ID: dialogID, Mode: "plan"},
		messages: []domain.DialogMessage{
			{DialogID: dialogID, Seq: 0, Role: "system", Content: "You plan infrastructure work."},
			{DialogID: dialogID, Seq: 1, Role: "user", Content: "roll out centrifugo proxying"},
		},
	}

	provider := &retryLLMProvider{
		responses: []llm.AssistantMessage{
			{Content: "<thinking>I will call list_variables next.</thinking>"},
			{ToolCalls: []llm.ToolCall{{
				ID:        "call_1",
				Name:      CreateActionPlanToolName,
				Arguments: `{"plan":{"stages":[],"rollback":[]}}`,
			}}},
			{Content: "Plan saved."},
		},
	}

	dataDir := t.TempDir()
	svc := NewChatService(provider, nil, nil, nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", dataDir, "", 10, PlanFanoutConfig{})

	catalog := newToolCatalog()
	catalog.localHandlers[CreateActionPlanToolName] = svc.createActionPlanHandler(dialogID)

	resp, err := svc.runPersistingAgentLoop(context.Background(), dialogID, "plan", catalog, loopConfig{})
	if err != nil {
		t.Fatalf("runPersistingAgentLoop: %v", err)
	}
	if !resp.ActionPlanUpdated {
		t.Error("ActionPlanUpdated = false, want true")
	}
	if resp.Response != "Plan saved." {
		t.Errorf("Response = %q, want %q", resp.Response, "Plan saved.")
	}

	if provider.calls != 3 {
		t.Fatalf("completion rounds = %d, want 3", provider.calls)
	}
	second := provider.seen[1]
	last := second[len(second)-1]
	if last.Role != "user" || !strings.Contains(last.Content, CreateActionPlanToolName) {
		t.Errorf("second round did not end with the reminder: %+v", last)
	}

	// The discarded narration must not reach the transcript: the stored turn is
	// the plan tool call plus the final answer.
	msgs, err := repo.ListMessages(context.Background(), dialogID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	for _, m := range msgs {
		if strings.Contains(m.Content, "<thinking>") {
			t.Fatalf("narration was persisted: %+v", m)
		}
	}
	if _, ok, err := svc.ReadActionPlan(dialogID); err != nil || !ok {
		t.Fatalf("ReadActionPlan() ok = %v, err = %v, want stored plan", ok, err)
	}
}

func TestRunPersistingAgentLoop_noReminderOutsidePlanMode(t *testing.T) {
	t.Parallel()

	dialogID := uuid.New()
	repo := &retryDialogRepo{
		dialog: domain.Dialog{ID: dialogID, Mode: "discuss"},
		messages: []domain.DialogMessage{
			{DialogID: dialogID, Seq: 0, Role: "user", Content: "hi"},
		},
	}

	provider := &retryLLMProvider{responses: []llm.AssistantMessage{{Content: "hello"}}}
	svc := NewChatService(provider, nil, nil, nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", t.TempDir(), "", 10, PlanFanoutConfig{})

	resp, err := svc.runPersistingAgentLoop(context.Background(), dialogID, "discuss", newToolCatalog(), loopConfig{})
	if err != nil {
		t.Fatalf("runPersistingAgentLoop: %v", err)
	}
	if resp.Response != "hello" || provider.calls != 1 {
		t.Fatalf("response = %q after %d rounds, want %q after 1", resp.Response, provider.calls, "hello")
	}
}

func TestRetryLastResponse_truncatesAndRegenerates(t *testing.T) {
	t.Parallel()

	dialogID := uuid.New()
	repo := &retryDialogRepo{
		dialog: domain.Dialog{
			ID:       dialogID,
			Mode:     "",
			Title:    "old title",
			Subjects: []string{"infra"},
		},
		messages: []domain.DialogMessage{
			{DialogID: dialogID, Seq: 0, Role: "system", Content: "You are helpful."},
			{DialogID: dialogID, Seq: 1, Role: "user", Content: "hello"},
			{DialogID: dialogID, Seq: 2, Role: "assistant", Content: "old answer"},
		},
	}

	dataDir := t.TempDir()
	for _, sub := range []string{"dags", "summaries", "action_plans"} {
		if err := os.MkdirAll(filepath.Join(dataDir, sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	id := dialogID.String()
	for _, path := range []string{
		filepath.Join(dataDir, "dags", id+".md"),
		filepath.Join(dataDir, "summaries", id+".txt"),
		filepath.Join(dataDir, "action_plans", id+".json"),
		filepath.Join(dataDir, "action_plans", id+".checks.json"),
	} {
		if err := os.WriteFile(path, []byte("artifact"), 0o644); err != nil {
			t.Fatalf("write artifact %s: %v", path, err)
		}
	}

	provider := &retryLLMProvider{
		responses: []llm.AssistantMessage{
			{Content: "new answer"},
		},
	}
	svc := NewChatService(provider, nil, nil, nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", dataDir, "", 10, PlanFanoutConfig{})

	resp, err := svc.RetryLastResponse(context.Background(), dialogID)
	if err != nil {
		t.Fatalf("RetryLastResponse: %v", err)
	}
	if resp.Response != "new answer" {
		t.Fatalf("response = %q, want %q", resp.Response, "new answer")
	}

	msgs, err := repo.ListMessages(context.Background(), dialogID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("len(messages) = %d, want 3", len(msgs))
	}
	if msgs[2].Role != "assistant" || msgs[2].Content != "new answer" {
		t.Fatalf("final message = %+v", msgs[2])
	}

	if repo.dialog.Title != "" {
		t.Fatalf("title = %q, want empty", repo.dialog.Title)
	}
	if len(repo.dialog.Subjects) != 0 {
		t.Fatalf("subjects = %v, want empty", repo.dialog.Subjects)
	}
	for _, path := range []string{
		filepath.Join(dataDir, "dags", id+".md"),
		filepath.Join(dataDir, "summaries", id+".txt"),
		filepath.Join(dataDir, "action_plans", id+".json"),
		filepath.Join(dataDir, "action_plans", id+".checks.json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("artifact %s should be removed, stat err = %v", path, err)
		}
	}
}

func TestRetryLastResponse_noUserMessage(t *testing.T) {
	t.Parallel()

	dialogID := uuid.New()
	repo := &retryDialogRepo{
		dialog: domain.Dialog{ID: dialogID},
		messages: []domain.DialogMessage{
			{DialogID: dialogID, Seq: 0, Role: "system", Content: "You are helpful."},
		},
	}
	svc := NewChatService(&retryLLMProvider{}, nil, nil, nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", t.TempDir(), "", 10, PlanFanoutConfig{})

	_, err := svc.RetryLastResponse(context.Background(), dialogID)
	if err == nil {
		t.Fatal("expected error when no user message exists")
	}
}

func TestDialogMessagesToLLM_roundTripToolCalls(t *testing.T) {
	t.Parallel()

	calls := []llm.ToolCall{
		{ID: "call_1", Name: "search", Arguments: `{"q":"pods"}`},
	}
	raw, err := marshalToolCalls(calls)
	if err != nil {
		t.Fatalf("marshalToolCalls: %v", err)
	}

	dialogID := uuid.New()
	msgs := []domain.DialogMessage{
		{DialogID: dialogID, Seq: 0, Role: "system", Content: "You are helpful."},
		{DialogID: dialogID, Seq: 1, Role: "user", Content: "hi"},
		{DialogID: dialogID, Seq: 2, Role: "assistant", Content: "", ToolCalls: raw},
		{DialogID: dialogID, Seq: 3, Role: "tool", Content: "ok", ToolCallID: "call_1", Name: "search"},
	}

	got, err := dialogMessagesToLLM(msgs)
	if err != nil {
		t.Fatalf("dialogMessagesToLLM: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	if len(got[2].ToolCalls) != 1 {
		t.Fatalf("assistant tool_calls: %+v", got[2].ToolCalls)
	}
	if got[2].ToolCalls[0].Name != "search" {
		t.Fatalf("tool name = %q", got[2].ToolCalls[0].Name)
	}
	if got[3].ToolCallID != "call_1" || got[3].Content != "ok" {
		t.Fatalf("tool message: %+v", got[3])
	}

	again, err := marshalToolCalls(got[2].ToolCalls)
	if err != nil {
		t.Fatalf("marshal again: %v", err)
	}
	if !jsonEqual(raw, again) {
		t.Fatalf("round-trip JSON mismatch:\n  got  %s\n  want %s", again, raw)
	}
}

func jsonEqual(a, b json.RawMessage) bool {
	var va, vb any
	if json.Unmarshal(a, &va) != nil || json.Unmarshal(b, &vb) != nil {
		return false
	}
	ab, _ := json.Marshal(va)
	bb, _ := json.Marshal(vb)
	return string(ab) == string(bb)
}
