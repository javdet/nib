package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/mcpclient"
)

// multiDialogRepo is an in-memory dialog repository that holds more than one
// dialog, which is what the orchestrator's tests need: every interesting
// assertion is about which of two dialogs something landed on.
type multiDialogRepo struct {
	dialogs  map[uuid.UUID]*domain.Dialog
	messages map[uuid.UUID][]domain.DialogMessage
	nextID   int64
}

func newMultiDialogRepo() *multiDialogRepo {
	return &multiDialogRepo{
		dialogs:  make(map[uuid.UUID]*domain.Dialog),
		messages: make(map[uuid.UUID][]domain.DialogMessage),
	}
}

// add inserts a dialog directly, for the root a test starts from.
func (r *multiDialogRepo) add(d domain.Dialog) uuid.UUID {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	copied := d
	r.dialogs[d.ID] = &copied
	return d.ID
}

func (r *multiDialogRepo) CreateDialog(_ context.Context, mode, title string, parentID *uuid.UUID) (domain.Dialog, error) {
	d := domain.Dialog{ID: uuid.New(), Mode: mode, Title: title, ParentID: parentID}
	r.dialogs[d.ID] = &d
	return d, nil
}

func (r *multiDialogRepo) GetDialog(_ context.Context, id uuid.UUID) (domain.Dialog, error) {
	d, ok := r.dialogs[id]
	if !ok {
		return domain.Dialog{}, fmt.Errorf("dialog %s not found", id)
	}
	return *d, nil
}

func (r *multiDialogRepo) ListChildren(_ context.Context, parentID uuid.UUID) ([]domain.Dialog, error) {
	var out []domain.Dialog
	for _, d := range r.dialogs {
		if d.ParentID != nil && *d.ParentID == parentID {
			out = append(out, *d)
		}
	}
	return out, nil
}

func (r *multiDialogRepo) ListMessages(_ context.Context, dialogID uuid.UUID) ([]domain.DialogMessage, error) {
	return append([]domain.DialogMessage(nil), r.messages[dialogID]...), nil
}

func (r *multiDialogRepo) AppendMessage(_ context.Context, dialogID uuid.UUID, msg domain.DialogMessage) (domain.DialogMessage, error) {
	r.nextID++
	msg.ID = r.nextID
	msg.DialogID = dialogID
	msg.Seq = len(r.messages[dialogID])
	r.messages[dialogID] = append(r.messages[dialogID], msg)
	return msg, nil
}

func (r *multiDialogRepo) UpdateTitle(_ context.Context, id uuid.UUID, title string) error {
	if d, ok := r.dialogs[id]; ok {
		d.Title = title
	}
	return nil
}

func (r *multiDialogRepo) SetDialogTaskID(_ context.Context, id uuid.UUID, taskID *string) error {
	if d, ok := r.dialogs[id]; ok {
		d.TaskID = taskID
	}
	return nil
}

func (r *multiDialogRepo) SetDialogSubjects(_ context.Context, id uuid.UUID, subjects []string) error {
	if d, ok := r.dialogs[id]; ok {
		d.Subjects = subjects
	}
	return nil
}

func (r *multiDialogRepo) SetDialogCategories(_ context.Context, id uuid.UUID, categories []string) error {
	if d, ok := r.dialogs[id]; ok {
		d.Categories = categories
	}
	return nil
}

func (r *multiDialogRepo) DeleteMessagesAfterSeq(_ context.Context, dialogID uuid.UUID, afterSeq int) error {
	kept := r.messages[dialogID][:0]
	for _, m := range r.messages[dialogID] {
		if m.Seq <= afterSeq {
			kept = append(kept, m)
		}
	}
	r.messages[dialogID] = kept
	return nil
}

func (r *multiDialogRepo) SetDialogPinned(context.Context, uuid.UUID, bool) error { return nil }
func (r *multiDialogRepo) ListPinnedDialogs(context.Context) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *multiDialogRepo) DeleteDialog(context.Context, uuid.UUID) error { return nil }
func (r *multiDialogRepo) ListRecentDialogs(context.Context, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *multiDialogRepo) CountDialogs(context.Context) (int, error) { return 0, nil }
func (r *multiDialogRepo) ListDialogsByMode(context.Context, string, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *multiDialogRepo) CountDialogsByMode(context.Context, string) (int, error) { return 0, nil }
func (r *multiDialogRepo) ListAllRecentDialogs(context.Context, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *multiDialogRepo) CountAllDialogs(context.Context) (int, error) { return 0, nil }
func (r *multiDialogRepo) SearchDialogs(context.Context, string, string, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *multiDialogRepo) CountDialogsSearch(context.Context, string, string) (int, error) {
	return 0, nil
}

// scriptedProvider answers each round from a fixed script, so a test can drive a
// sub-agent to a tool call, to a question, or to a plain answer.
type scriptedProvider struct {
	turns []llm.AssistantMessage
	calls int
}

func (p *scriptedProvider) Complete(context.Context, string, string) (string, error) {
	return "", nil
}

func (p *scriptedProvider) CompleteWithTools(_ context.Context, _ []llm.Message, _ []llm.ToolDef) (llm.AssistantMessage, error) {
	if p.calls >= len(p.turns) {
		return llm.AssistantMessage{Content: "done"}, nil
	}
	msg := p.turns[p.calls]
	p.calls++
	return msg, nil
}

// askQuestionTurn is an assistant round that suspends on one question.
func askQuestionTurn(callID, question string) llm.AssistantMessage {
	return llm.AssistantMessage{
		ToolCalls: []llm.ToolCall{{
			ID:        callID,
			Name:      AskQuestionToolName,
			Arguments: fmt.Sprintf(`{"questions":[{"question":%q,"options":["a","b"]}]}`, question),
		}},
	}
}

// emptyMCPRepo is an MCP repository with nothing in it, so buildToolCatalog can
// run in a test without a database: the local tools are what these tests are
// about.
type emptyMCPRepo struct{}

func (emptyMCPRepo) List(context.Context) ([]domain.MCPConnection, error) { return nil, nil }
func (emptyMCPRepo) GetByID(context.Context, uuid.UUID) (domain.MCPConnection, error) {
	return domain.MCPConnection{}, nil
}
func (emptyMCPRepo) Create(_ context.Context, conn domain.MCPConnection) (domain.MCPConnection, error) {
	return conn, nil
}
func (emptyMCPRepo) Update(_ context.Context, _ uuid.UUID, conn domain.MCPConnection) (domain.MCPConnection, error) {
	return conn, nil
}
func (emptyMCPRepo) UpdateTokens(context.Context, uuid.UUID, string, string, *time.Time) error {
	return nil
}
func (emptyMCPRepo) UpdateStatus(context.Context, uuid.UUID, string) error { return nil }
func (emptyMCPRepo) Delete(context.Context, uuid.UUID) error               { return nil }

func stubMCPService() *MCPService {
	return NewMCPService(emptyMCPRepo{}, mcpclient.NewManager(), nil, nil)
}
