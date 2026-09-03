package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
)

const maxDialogTagCount = 32

// ErrEmptyDialogTitle is returned when a dialog title is empty after trimming.
var ErrEmptyDialogTitle = errors.New("title is required")

// ErrTooManyDialogTags is returned when a tag list exceeds the allowed size.
var ErrTooManyDialogTags = errors.New("too many tags")

// DialogService implements business logic for persisted chat dialogs.
type DialogService struct {
	repo repository.DialogRepository
}

func NewDialogService(repo repository.DialogRepository) *DialogService {
	return &DialogService{repo: repo}
}

func (s *DialogService) List(ctx context.Context, limit, offset int) ([]domain.Dialog, error) {
	dialogs, err := s.repo.ListRecentDialogs(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list dialogs: %w", err)
	}
	return dialogs, nil
}

func (s *DialogService) Count(ctx context.Context) (int, error) {
	count, err := s.repo.CountDialogs(ctx)
	if err != nil {
		return 0, fmt.Errorf("count dialogs: %w", err)
	}
	return count, nil
}

func (s *DialogService) ListByMode(ctx context.Context, mode string, limit, offset int) ([]domain.Dialog, error) {
	dialogs, err := s.repo.ListDialogsByMode(ctx, mode, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list dialogs by mode: %w", err)
	}
	return dialogs, nil
}

func (s *DialogService) CountByMode(ctx context.Context, mode string) (int, error) {
	count, err := s.repo.CountDialogsByMode(ctx, mode)
	if err != nil {
		return 0, fmt.Errorf("count dialogs by mode: %w", err)
	}
	return count, nil
}

func (s *DialogService) ListAll(ctx context.Context, limit, offset int) ([]domain.Dialog, error) {
	dialogs, err := s.repo.ListAllRecentDialogs(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list all dialogs: %w", err)
	}
	return dialogs, nil
}

func (s *DialogService) CountAll(ctx context.Context) (int, error) {
	count, err := s.repo.CountAllDialogs(ctx)
	if err != nil {
		return 0, fmt.Errorf("count all dialogs: %w", err)
	}
	return count, nil
}

func (s *DialogService) Search(ctx context.Context, mode, query string, limit, offset int) ([]domain.Dialog, error) {
	dialogs, err := s.repo.SearchDialogs(ctx, mode, strings.TrimSpace(query), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("search dialogs: %w", err)
	}
	return dialogs, nil
}

func (s *DialogService) CountSearch(ctx context.Context, mode, query string) (int, error) {
	count, err := s.repo.CountDialogsSearch(ctx, mode, strings.TrimSpace(query))
	if err != nil {
		return 0, fmt.Errorf("count search dialogs: %w", err)
	}
	return count, nil
}

func (s *DialogService) Create(ctx context.Context, mode, title string, parentID *uuid.UUID) (domain.Dialog, error) {
	d, err := s.repo.CreateDialog(ctx, strings.TrimSpace(mode), strings.TrimSpace(title), parentID)
	if err != nil {
		return domain.Dialog{}, fmt.Errorf("create dialog: %w", err)
	}
	return d, nil
}

func (s *DialogService) Get(ctx context.Context, id uuid.UUID) (domain.Dialog, error) {
	d, err := s.repo.GetDialog(ctx, id)
	if err != nil {
		return domain.Dialog{}, fmt.Errorf("get dialog: %w", err)
	}
	return d, nil
}

func (s *DialogService) UpdateTitle(ctx context.Context, id uuid.UUID, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return ErrEmptyDialogTitle
	}
	if err := s.repo.UpdateTitle(ctx, id, title); err != nil {
		return fmt.Errorf("update dialog title: %w", err)
	}
	return nil
}

func (s *DialogService) SetCategories(ctx context.Context, id uuid.UUID, categories []string) error {
	list, err := normalizeTagList(categories)
	if err != nil {
		return err
	}
	if err := s.repo.SetDialogCategories(ctx, id, list); err != nil {
		return fmt.Errorf("set dialog categories: %w", err)
	}
	return nil
}

func normalizeTagList(items []string) ([]string, error) {
	if len(items) == 0 {
		return []string{}, nil
	}

	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		for _, part := range strings.FieldsFunc(item, func(r rune) bool {
			return unicode.IsSpace(r) || r == ','
		}) {
			normalized := strings.ToLower(strings.TrimSpace(part))
			if normalized == "" {
				continue
			}
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			out = append(out, normalized)
			if len(out) > maxDialogTagCount {
				return nil, ErrTooManyDialogTags
			}
		}
	}
	return out, nil
}

func (s *DialogService) SetPinned(ctx context.Context, id uuid.UUID, pinned bool) error {
	if err := s.repo.SetDialogPinned(ctx, id, pinned); err != nil {
		return fmt.Errorf("set dialog pinned: %w", err)
	}
	return nil
}

func (s *DialogService) ListPinned(ctx context.Context) ([]domain.Dialog, error) {
	dialogs, err := s.repo.ListPinnedDialogs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pinned dialogs: %w", err)
	}
	return dialogs, nil
}

func (s *DialogService) ListChildren(ctx context.Context, parentID uuid.UUID) ([]domain.Dialog, error) {
	dialogs, err := s.repo.ListChildren(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("list child dialogs: %w", err)
	}
	return dialogs, nil
}

func (s *DialogService) GetMessages(ctx context.Context, dialogID uuid.UUID) ([]domain.DialogMessage, error) {
	msgs, err := s.repo.ListMessages(ctx, dialogID)
	if err != nil {
		return nil, fmt.Errorf("list dialog messages: %w", err)
	}
	return msgs, nil
}

func (s *DialogService) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.DeleteDialog(ctx, id); err != nil {
		return fmt.Errorf("delete dialog: %w", err)
	}
	return nil
}

