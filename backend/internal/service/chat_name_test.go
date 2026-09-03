package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

type fakeDialogRepo struct {
	title    string
	taskID   *string
	subjects []string
	err      error
}

func (f *fakeDialogRepo) CreateDialog(_ context.Context, _, _ string, _ *uuid.UUID) (domain.Dialog, error) {
	return domain.Dialog{}, nil
}

func (f *fakeDialogRepo) ListChildren(_ context.Context, _ uuid.UUID) ([]domain.Dialog, error) {
	return nil, nil
}

func (f *fakeDialogRepo) ListRecentDialogs(_ context.Context, _, _ int) ([]domain.Dialog, error) {
	return nil, nil
}

func (f *fakeDialogRepo) CountDialogs(_ context.Context) (int, error) {
	return 0, nil
}

func (f *fakeDialogRepo) ListDialogsByMode(_ context.Context, _ string, _, _ int) ([]domain.Dialog, error) {
	return nil, nil
}

func (f *fakeDialogRepo) CountDialogsByMode(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (f *fakeDialogRepo) ListAllRecentDialogs(_ context.Context, _, _ int) ([]domain.Dialog, error) {
	return nil, nil
}

func (f *fakeDialogRepo) CountAllDialogs(_ context.Context) (int, error) {
	return 0, nil
}

func (f *fakeDialogRepo) SearchDialogs(_ context.Context, _, _ string, _, _ int) ([]domain.Dialog, error) {
	return nil, nil
}

func (f *fakeDialogRepo) CountDialogsSearch(_ context.Context, _, _ string) (int, error) {
	return 0, nil
}

func (f *fakeDialogRepo) GetDialog(_ context.Context, _ uuid.UUID) (domain.Dialog, error) {
	return domain.Dialog{}, nil
}

func (f *fakeDialogRepo) UpdateTitle(_ context.Context, _ uuid.UUID, title string) error {
	if f.err != nil {
		return f.err
	}
	f.title = title
	return nil
}

func (f *fakeDialogRepo) SetDialogTaskID(_ context.Context, _ uuid.UUID, taskID *string) error {
	if f.err != nil {
		return f.err
	}
	f.taskID = taskID
	return nil
}

func (f *fakeDialogRepo) SetDialogSubjects(_ context.Context, _ uuid.UUID, subjects []string) error {
	if f.err != nil {
		return f.err
	}
	f.subjects = subjects
	return nil
}

func (f *fakeDialogRepo) SetDialogCategories(_ context.Context, _ uuid.UUID, _ []string) error {
	return f.err
}

func (f *fakeDialogRepo) SetDialogPinned(_ context.Context, _ uuid.UUID, _ bool) error {
	return nil
}

func (f *fakeDialogRepo) ListPinnedDialogs(_ context.Context) ([]domain.Dialog, error) {
	return nil, nil
}

func (f *fakeDialogRepo) DeleteDialog(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (f *fakeDialogRepo) ListMessages(_ context.Context, _ uuid.UUID) ([]domain.DialogMessage, error) {
	return nil, nil
}

func (f *fakeDialogRepo) AppendMessage(_ context.Context, _ uuid.UUID, _ domain.DialogMessage) (domain.DialogMessage, error) {
	return domain.DialogMessage{}, nil
}

func (f *fakeDialogRepo) DeleteMessagesAfterSeq(_ context.Context, _ uuid.UUID, _ int) error {
	return nil
}

func TestChatNameToolDef(t *testing.T) {
	t.Parallel()
	def := ChatNameToolDef()
	if def.Name != ChatNameToolName {
		t.Fatalf("name = %q, want %q", def.Name, ChatNameToolName)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(def.Parameters, &schema); err != nil {
		t.Fatalf("unmarshal parameters: %v", err)
	}
	if len(schema.Required) != 1 {
		t.Fatalf("required = %#v, want [chat_name]", schema.Required)
	}
}

func TestChatNameHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dialogID := uuid.New()

	t.Run("valid input", func(t *testing.T) {
		t.Parallel()
		repo := &fakeDialogRepo{}
		svc := &ChatService{dialogRepo: repo}
		handler := svc.chatNameHandler(dialogID)

		out, err := handler(ctx, map[string]any{
			"task_id":   "DO-236",
			"chat_name": "Increase redpanda topics",
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		wantTitle := "DO-236: Increase redpanda topics"
		if repo.title != wantTitle {
			t.Fatalf("title = %q, want %q", repo.title, wantTitle)
		}
		if repo.taskID == nil || *repo.taskID != "DO-236" {
			t.Fatalf("taskID = %#v, want DO-236", repo.taskID)
		}
		if out != "Conversation named: "+wantTitle {
			t.Fatalf("out = %q", out)
		}
	})

	t.Run("truncates long chat_name", func(t *testing.T) {
		t.Parallel()
		repo := &fakeDialogRepo{}
		svc := &ChatService{dialogRepo: repo}
		handler := svc.chatNameHandler(dialogID)

		longName := strings.Repeat("a", chatNameMaxLen+20)
		out, err := handler(ctx, map[string]any{
			"task_id":   "DO-236",
			"chat_name": longName,
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		wantTitle := "DO-236: " + strings.Repeat("a", chatNameMaxLen)
		if repo.title != wantTitle {
			t.Fatalf("title = %q, want %q", repo.title, wantTitle)
		}
		if out != "Conversation named: "+wantTitle {
			t.Fatalf("out = %q", out)
		}
	})

	t.Run("cyrillic title under rune limit but over byte limit", func(t *testing.T) {
		t.Parallel()
		repo := &fakeDialogRepo{}
		svc := &ChatService{dialogRepo: repo}
		handler := svc.chatNameHandler(dialogID)

		chatName := "Адаптация iOS-сборки фронтенда под macos-26 раннеры"
		out, err := handler(ctx, map[string]any{
			"task_id":   "OPS-2018",
			"chat_name": chatName,
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		wantTitle := "OPS-2018: " + chatName
		if repo.title != wantTitle {
			t.Fatalf("title = %q, want %q", repo.title, wantTitle)
		}
		if !utf8.ValidString(repo.title) {
			t.Fatalf("title is not valid UTF-8: %q", repo.title)
		}
		if out != "Conversation named: "+wantTitle {
			t.Fatalf("out = %q", out)
		}
	})

	t.Run("truncates long cyrillic chat_name by runes", func(t *testing.T) {
		t.Parallel()
		repo := &fakeDialogRepo{}
		svc := &ChatService{dialogRepo: repo}
		handler := svc.chatNameHandler(dialogID)

		longName := strings.Repeat("А", chatNameMaxLen+20)
		out, err := handler(ctx, map[string]any{
			"task_id":   "OPS-2018",
			"chat_name": longName,
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		wantName := strings.Repeat("А", chatNameMaxLen)
		wantTitle := "OPS-2018: " + wantName
		if repo.title != wantTitle {
			t.Fatalf("title = %q, want %q", repo.title, wantTitle)
		}
		if utf8.RuneCountInString(wantName) != chatNameMaxLen {
			t.Fatalf("truncated name runes = %d, want %d", utf8.RuneCountInString(wantName), chatNameMaxLen)
		}
		if !utf8.ValidString(repo.title) {
			t.Fatalf("title is not valid UTF-8: %q", repo.title)
		}
		if out != "Conversation named: "+wantTitle {
			t.Fatalf("out = %q", out)
		}
	})

	t.Run("missing chat_name", func(t *testing.T) {
		t.Parallel()
		repo := &fakeDialogRepo{}
		svc := &ChatService{dialogRepo: repo}
		handler := svc.chatNameHandler(dialogID)

		out, err := handler(ctx, map[string]any{"task_id": "DO-236"})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if out != "chat_name is required" {
			t.Fatalf("out = %q", out)
		}
		if repo.title != "" {
			t.Fatalf("title = %q, want empty", repo.title)
		}
		if repo.taskID != nil {
			t.Fatalf("taskID = %#v, want nil", repo.taskID)
		}
	})

	t.Run("chat_name without task_id", func(t *testing.T) {
		t.Parallel()
		repo := &fakeDialogRepo{}
		svc := &ChatService{dialogRepo: repo}
		handler := svc.chatNameHandler(dialogID)

		out, err := handler(ctx, map[string]any{
			"chat_name": "Increase redpanda topics",
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		wantTitle := "Increase redpanda topics"
		if repo.title != wantTitle {
			t.Fatalf("title = %q, want %q", repo.title, wantTitle)
		}
		if repo.taskID != nil {
			t.Fatalf("taskID = %#v, want nil", repo.taskID)
		}
		if out != "Conversation named: "+wantTitle {
			t.Fatalf("out = %q", out)
		}
	})

	t.Run("task_id none leaves task_id unset", func(t *testing.T) {
		t.Parallel()
		repo := &fakeDialogRepo{}
		svc := &ChatService{dialogRepo: repo}
		handler := svc.chatNameHandler(dialogID)

		out, err := handler(ctx, map[string]any{
			"task_id":   "None",
			"chat_name": "Increase redpanda topics",
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		wantTitle := "Increase redpanda topics"
		if repo.title != wantTitle {
			t.Fatalf("title = %q, want %q", repo.title, wantTitle)
		}
		if repo.taskID != nil {
			t.Fatalf("taskID = %#v, want nil", repo.taskID)
		}
		if out != "Conversation named: "+wantTitle {
			t.Fatalf("out = %q", out)
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		t.Parallel()
		repo := &fakeDialogRepo{}
		svc := &ChatService{dialogRepo: repo}
		handler := svc.chatNameHandler(dialogID)

		out, err := handler(ctx, map[string]any{
			"task_id":   "  DO-236  ",
			"chat_name": "  Increase redpanda topics  ",
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		wantTitle := "DO-236: Increase redpanda topics"
		if repo.title != wantTitle {
			t.Fatalf("title = %q, want %q", repo.title, wantTitle)
		}
		if out != "Conversation named: "+wantTitle {
			t.Fatalf("out = %q", out)
		}
	})
}

func TestAddLocalTools_includesChatNameForDialog(t *testing.T) {
	t.Parallel()
	repo := &fakeDialogRepo{}
	svc := &ChatService{dialogRepo: repo}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}

	svc.addLocalTools(&catalog, nil, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[ChatNameToolName]; !ok {
		t.Fatal("expected chat_name handler")
	}
	found := false
	for _, def := range catalog.tools {
		if def.Name == ChatNameToolName {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected chat_name in tool defs")
	}
}

func TestAddLocalTools_excludesChatNameWithoutDialog(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}

	svc.addLocalTools(&catalog, nil, newToolBinding(uuid.Nil))

	if _, ok := catalog.localHandlers[ChatNameToolName]; ok {
		t.Fatal("did not expect chat_name handler without dialog")
	}
}
