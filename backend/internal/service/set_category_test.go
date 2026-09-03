package service

import (
	"context"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/toolcatalog"
	"github.com/google/uuid"
)

type setCategoryDialogRepo struct {
	categories []string
}

func (r *setCategoryDialogRepo) CreateDialog(context.Context, string, string, *uuid.UUID) (domain.Dialog, error) {
	return domain.Dialog{}, nil
}
func (r *setCategoryDialogRepo) ListRecentDialogs(context.Context, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *setCategoryDialogRepo) CountDialogs(context.Context) (int, error) { return 0, nil }
func (r *setCategoryDialogRepo) ListDialogsByMode(context.Context, string, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *setCategoryDialogRepo) CountDialogsByMode(context.Context, string) (int, error) { return 0, nil }
func (r *setCategoryDialogRepo) ListAllRecentDialogs(context.Context, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *setCategoryDialogRepo) CountAllDialogs(context.Context) (int, error) { return 0, nil }
func (r *setCategoryDialogRepo) SearchDialogs(context.Context, string, string, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *setCategoryDialogRepo) CountDialogsSearch(context.Context, string, string) (int, error) {
	return 0, nil
}
func (r *setCategoryDialogRepo) ListChildren(context.Context, uuid.UUID) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *setCategoryDialogRepo) GetDialog(context.Context, uuid.UUID) (domain.Dialog, error) {
	return domain.Dialog{}, nil
}
func (r *setCategoryDialogRepo) UpdateTitle(context.Context, uuid.UUID, string) error { return nil }
func (r *setCategoryDialogRepo) SetDialogTaskID(context.Context, uuid.UUID, *string) error {
	return nil
}

func (r *setCategoryDialogRepo) SetDialogCategories(_ context.Context, _ uuid.UUID, categories []string) error {
	r.categories = categories
	return nil
}
func (r *setCategoryDialogRepo) SetDialogPinned(context.Context, uuid.UUID, bool) error { return nil }
func (r *setCategoryDialogRepo) ListPinnedDialogs(context.Context) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *setCategoryDialogRepo) DeleteDialog(context.Context, uuid.UUID) error { return nil }
func (r *setCategoryDialogRepo) ListMessages(context.Context, uuid.UUID) ([]domain.DialogMessage, error) {
	return nil, nil
}
func (r *setCategoryDialogRepo) AppendMessage(context.Context, uuid.UUID, domain.DialogMessage) (domain.DialogMessage, error) {
	return domain.DialogMessage{}, nil
}
func (r *setCategoryDialogRepo) DeleteMessagesAfterSeq(context.Context, uuid.UUID, int) error {
	return nil
}

type stubToolCategoryLister struct {
	categories []toolcatalog.CategoryWithPatterns
	byCategory map[string][]toolcatalog.CatalogTool
	listErr    error
	toolsErr   error
}

func (s *stubToolCategoryLister) List(_ context.Context) ([]toolcatalog.CategoryWithPatterns, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.categories, nil
}

func (s *stubToolCategoryLister) ListToolsByCategory(_ context.Context, name string) ([]toolcatalog.CatalogTool, error) {
	if s.toolsErr != nil {
		return nil, s.toolsErr
	}
	if s.byCategory == nil {
		return nil, nil
	}
	return s.byCategory[name], nil
}

func TestSetCategoryHandler_SavesNormalizedCategories(t *testing.T) {
	repo := &setCategoryDialogRepo{}
	lister := &stubToolCategoryLister{
		categories: []toolcatalog.CategoryWithPatterns{
			{Name: "logging"},
			{Name: "monitoring"},
		},
	}
	svc := &ChatService{
		dialogRepo:              repo,
		toolCategorySvc:         lister,
	}
	dialogID := uuid.New()
	handler := svc.setCategoryHandler(dialogID)

	out, err := handler(context.Background(), map[string]any{
		"categories": []any{"Logging", "monitoring"},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if out != "Categories saved: logging, monitoring" {
		t.Fatalf("unexpected output: %q", out)
	}
	if len(repo.categories) != 2 || repo.categories[0] != "logging" || repo.categories[1] != "monitoring" {
		t.Fatalf("unexpected saved categories: %v", repo.categories)
	}
}

func TestSetCategoryHandler_RejectsUnknownCategories(t *testing.T) {
	repo := &setCategoryDialogRepo{}
	lister := &stubToolCategoryLister{
		categories: []toolcatalog.CategoryWithPatterns{
			{Name: "logging"},
		},
	}
	svc := &ChatService{
		dialogRepo:              repo,
		toolCategorySvc:         lister,
	}
	handler := svc.setCategoryHandler(uuid.New())

	out, err := handler(context.Background(), map[string]any{
		"categories": []any{"unknown"},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !strings.Contains(out, "unknown categories: unknown") {
		t.Fatalf("expected unknown categories message, got %q", out)
	}
	if repo.categories != nil {
		t.Fatalf("expected no categories saved, got %v", repo.categories)
	}
}
