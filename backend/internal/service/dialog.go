package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/mode"
	"github.com/javdet/nib/internal/repository"
)

const maxDialogTagCount = 32

// ErrEmptyDialogTitle is returned when a dialog title is empty after trimming.
var ErrEmptyDialogTitle = errors.New("title is required")

// ErrTooManyDialogTags is returned when a tag list exceeds the allowed size.
var ErrTooManyDialogTags = errors.New("too many tags")

// ErrInvalidDialogMode is returned when a dialog is created in a mode that has
// no system prompt and no tool catalog behind it.
var ErrInvalidDialogMode = errors.New("invalid dialog mode")

// ErrInvalidDialogParent is returned when a dialog is given a parent that is
// itself a child. Dialog trees are one level deep.
var ErrInvalidDialogParent = errors.New("invalid dialog parent")

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

func (s *DialogService) ListByMode(ctx context.Context, dialogMode string, limit, offset int) ([]domain.Dialog, error) {
	dialogs, err := s.repo.ListDialogsByMode(ctx, dialogMode, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list dialogs by mode: %w", err)
	}
	return dialogs, nil
}

func (s *DialogService) CountByMode(ctx context.Context, dialogMode string) (int, error) {
	count, err := s.repo.CountDialogsByMode(ctx, dialogMode)
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

func (s *DialogService) Search(ctx context.Context, dialogMode, query string, limit, offset int) ([]domain.Dialog, error) {
	dialogs, err := s.repo.SearchDialogs(ctx, dialogMode, strings.TrimSpace(query), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("search dialogs: %w", err)
	}
	return dialogs, nil
}

func (s *DialogService) CountSearch(ctx context.Context, dialogMode, query string) (int, error) {
	count, err := s.repo.CountDialogsSearch(ctx, dialogMode, strings.TrimSpace(query))
	if err != nil {
		return 0, fmt.Errorf("count search dialogs: %w", err)
	}
	return count, nil
}

func (s *DialogService) Create(ctx context.Context, dialogMode, title string, parentID *uuid.UUID) (domain.Dialog, error) {
	// The API accepts a body with no mode at all, and an unchecked mode used to
	// be stored verbatim: '' passed the old blacklist predicate and was listed as
	// a plan, while mode.IsValid('') is false so the dialog got no system prompt
	// and an empty tool catalog.
	dialogMode = strings.TrimSpace(dialogMode)
	if dialogMode == "" {
		dialogMode = mode.Default
	}
	if !mode.IsValid(dialogMode) {
		return domain.Dialog{}, fmt.Errorf("%w: %q", ErrInvalidDialogMode, dialogMode)
	}

	// One level is the contract every internal caller already keeps by resolving
	// a root first, but this one takes the parent straight from the request. The
	// schema cannot express a depth limit, so it is checked here; without it the
	// API can build an arbitrarily deep chain that resolveRootDialogID then has
	// to walk under its cycle guard.
	if parentID != nil {
		parent, err := s.repo.GetDialog(ctx, *parentID)
		if err != nil {
			return domain.Dialog{}, fmt.Errorf("create dialog: resolve parent: %w", err)
		}
		if parent.ParentID != nil {
			return domain.Dialog{}, fmt.Errorf("%w: parent %s is not a root dialog", ErrInvalidDialogParent, parent.ID)
		}
	}

	d, err := s.repo.CreateDialog(ctx, dialogMode, strings.TrimSpace(title), parentID)
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
