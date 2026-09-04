package repository

import (
	"context"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

// DialogRepository defines the data-access contract for persisted chat dialogs.
type DialogRepository interface {
	CreateDialog(ctx context.Context, mode, title string, parentID *uuid.UUID) (domain.Dialog, error)
	ListRecentDialogs(ctx context.Context, limit, offset int) ([]domain.Dialog, error)
	CountDialogs(ctx context.Context) (int, error)
	ListDialogsByMode(ctx context.Context, mode string, limit, offset int) ([]domain.Dialog, error)
	CountDialogsByMode(ctx context.Context, mode string) (int, error)
	ListAllRecentDialogs(ctx context.Context, limit, offset int) ([]domain.Dialog, error)
	CountAllDialogs(ctx context.Context) (int, error)
	SearchDialogs(ctx context.Context, mode, query string, limit, offset int) ([]domain.Dialog, error)
	CountDialogsSearch(ctx context.Context, mode, query string) (int, error)
	ListChildren(ctx context.Context, parentID uuid.UUID) ([]domain.Dialog, error)
	// ListPlanDialogIDs returns the root dialogs that are plans, matching the
	// predicate the dialog list uses (see handler.enrichDialogsWithPlanStatus).
	ListPlanDialogIDs(ctx context.Context) ([]uuid.UUID, error)
	GetDialog(ctx context.Context, id uuid.UUID) (domain.Dialog, error)
	UpdateTitle(ctx context.Context, id uuid.UUID, title string) error
	SetDialogTaskID(ctx context.Context, id uuid.UUID, taskID *string) error
	SetDialogCategories(ctx context.Context, id uuid.UUID, categories []string) error
	SetDialogPinned(ctx context.Context, id uuid.UUID, pinned bool) error
	ListPinnedDialogs(ctx context.Context) ([]domain.Dialog, error)
	DeleteDialog(ctx context.Context, id uuid.UUID) error
	ListMessages(ctx context.Context, dialogID uuid.UUID) ([]domain.DialogMessage, error)
	AppendMessage(ctx context.Context, dialogID uuid.UUID, msg domain.DialogMessage) (domain.DialogMessage, error)
	DeleteMessagesAfterSeq(ctx context.Context, dialogID uuid.UUID, afterSeq int) error
}
