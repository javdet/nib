package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/toolcatalog"
	"github.com/google/uuid"
)

type allowSetDialogRepo struct {
	dialogs map[uuid.UUID]domain.Dialog
}

func (r *allowSetDialogRepo) CreateDialog(context.Context, string, string, *uuid.UUID) (domain.Dialog, error) {
	return domain.Dialog{}, nil
}
func (r *allowSetDialogRepo) ListRecentDialogs(context.Context, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *allowSetDialogRepo) CountDialogs(context.Context) (int, error) { return 0, nil }
func (r *allowSetDialogRepo) ListDialogsByMode(context.Context, string, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *allowSetDialogRepo) CountDialogsByMode(context.Context, string) (int, error) { return 0, nil }
func (r *allowSetDialogRepo) ListAllRecentDialogs(context.Context, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *allowSetDialogRepo) CountAllDialogs(context.Context) (int, error) { return 0, nil }
func (r *allowSetDialogRepo) SearchDialogs(context.Context, string, string, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *allowSetDialogRepo) CountDialogsSearch(context.Context, string, string) (int, error) {
	return 0, nil
}
func (r *allowSetDialogRepo) ListChildren(context.Context, uuid.UUID) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *allowSetDialogRepo) GetDialog(_ context.Context, id uuid.UUID) (domain.Dialog, error) {
	d, ok := r.dialogs[id]
	if !ok {
		return domain.Dialog{}, nil
	}
	return d, nil
}
func (r *allowSetDialogRepo) UpdateTitle(context.Context, uuid.UUID, string) error { return nil }
func (r *allowSetDialogRepo) SetDialogTaskID(context.Context, uuid.UUID, *string) error {
	return nil
}

func (r *allowSetDialogRepo) SetDialogCategories(context.Context, uuid.UUID, []string) error {
	return nil
}
func (r *allowSetDialogRepo) SetDialogPinned(context.Context, uuid.UUID, bool) error { return nil }
func (r *allowSetDialogRepo) ListPinnedDialogs(context.Context) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *allowSetDialogRepo) DeleteDialog(context.Context, uuid.UUID) error { return nil }
func (r *allowSetDialogRepo) ListMessages(context.Context, uuid.UUID) ([]domain.DialogMessage, error) {
	return nil, nil
}
func (r *allowSetDialogRepo) AppendMessage(context.Context, uuid.UUID, domain.DialogMessage) (domain.DialogMessage, error) {
	return domain.DialogMessage{}, nil
}
func (r *allowSetDialogRepo) DeleteMessagesAfterSeq(context.Context, uuid.UUID, int) error {
	return nil
}

func TestResolveDialogAllowSet(t *testing.T) {
	allowDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(allowDir, "plan.json"), []byte(`{"allow_tools":["ask_question"]}`), 0o644); err != nil {
		t.Fatalf("write plan allow list: %v", err)
	}
	if err := os.WriteFile(filepath.Join(allowDir, "decompose.json"), []byte(`{"allow_tools":["set_category"]}`), 0o644); err != nil {
		t.Fatalf("write decompose allow list: %v", err)
	}

	parentID := uuid.New()
	planID := uuid.New()
	repo := &allowSetDialogRepo{
		dialogs: map[uuid.UUID]domain.Dialog{
			parentID: {
				ID:         parentID,
				Mode:       "decompose",
				Categories: []string{"logging"},
			},
			planID: {
				ID:       planID,
				Mode:     "plan",
				ParentID: &parentID,
			},
		},
	}
	lister := &stubToolCategoryLister{
		byCategory: map[string][]toolcatalog.CatalogTool{
			"logging": {
				{Name: "ask_question"},
				{Name: "grafana_query_loki_logs"},
			},
		},
	}

	svc := &ChatService{
		dialogRepo:    repo,
		toolCategorySvc: lister,
		allowToolsDir: allowDir,
	}

	tests := []struct {
		name     string
		dialog   domain.Dialog
		want     []string
		notWant  []string
	}{
		{
			name:   "plan inherits parent categories",
			dialog: repo.dialogs[planID],
			want:   []string{"ask_question", "grafana_query_loki_logs"},
		},
		{
			name:   "decompose unchanged",
			dialog: repo.dialogs[parentID],
			want:   []string{"set_category"},
			notWant: []string{"grafana_query_loki_logs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allow, err := svc.resolveDialogAllowSet(context.Background(), tt.dialog)
			if err != nil {
				t.Fatalf("resolveDialogAllowSet: %v", err)
			}
			for _, name := range tt.want {
				if _, ok := allow[name]; !ok {
					t.Fatalf("expected %q in allow set, got %v", name, allow)
				}
			}
			for _, name := range tt.notWant {
				if _, ok := allow[name]; ok {
					t.Fatalf("did not expect %q in allow set, got %v", name, allow)
				}
			}
		})
	}
}

func TestResolveDialogAllowSet_MergesOverlappingTools(t *testing.T) {
	allowDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(allowDir, "plan.json"), []byte(`{"allow_tools":["ask_question"]}`), 0o644); err != nil {
		t.Fatalf("write plan allow list: %v", err)
	}

	parentID := uuid.New()
	planID := uuid.New()
	repo := &allowSetDialogRepo{
		dialogs: map[uuid.UUID]domain.Dialog{
			parentID: {ID: parentID, Mode: "decompose", Categories: []string{"monitoring"}},
			planID:   {ID: planID, Mode: "plan", ParentID: &parentID},
		},
	}
	lister := &stubToolCategoryLister{
		byCategory: map[string][]toolcatalog.CatalogTool{
			"monitoring": {
				{Name: "ask_question"},
				{Name: "grafana_list_dashboards"},
			},
		},
	}
	svc := &ChatService{
		dialogRepo:      repo,
		toolCategorySvc: lister,
		allowToolsDir:   allowDir,
	}

	allow, err := svc.resolveDialogAllowSet(context.Background(), repo.dialogs[planID])
	if err != nil {
		t.Fatalf("resolveDialogAllowSet: %v", err)
	}
	if len(allow) != 2 {
		t.Fatalf("expected 2 unique tools, got %d: %v", len(allow), allow)
	}
}

func TestResolveDialogAllowSet_ListToolsErrorIsNonFatal(t *testing.T) {
	allowDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(allowDir, "plan.json"), []byte(`{"allow_tools":["ask_question"]}`), 0o644); err != nil {
		t.Fatalf("write plan allow list: %v", err)
	}

	parentID := uuid.New()
	planID := uuid.New()
	repo := &allowSetDialogRepo{
		dialogs: map[uuid.UUID]domain.Dialog{
			parentID: {ID: parentID, Mode: "decompose", Categories: []string{"logging"}},
			planID:   {ID: planID, Mode: "plan", ParentID: &parentID},
		},
	}
	lister := &stubToolCategoryLister{
		toolsErr: context.Canceled,
	}
	svc := &ChatService{
		dialogRepo:      repo,
		toolCategorySvc: lister,
		allowToolsDir:   allowDir,
	}

	allow, err := svc.resolveDialogAllowSet(context.Background(), repo.dialogs[planID])
	if err != nil {
		t.Fatalf("resolveDialogAllowSet: %v", err)
	}
	if _, ok := allow["ask_question"]; !ok {
		t.Fatalf("expected base allow tool to remain, got %v", allow)
	}
}
