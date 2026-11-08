package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

// CommandRunRepository is the audit trail of agent commands.
type CommandRunRepository interface {
	InsertCommandRun(ctx context.Context, run domain.CommandRun) error
	FinishCommandRun(ctx context.Context, run domain.CommandRun) error
	// ListCommandRuns returns a plan's runs, newest first, optionally only one
	// action row's.
	ListCommandRuns(ctx context.Context, planID uuid.UUID, actionKey string, limit int) ([]domain.CommandRun, error)
	GetCommandRun(ctx context.Context, id uuid.UUID) (domain.CommandRun, error)
	// MarkRunningCommandRunsLost closes every row still running, which at boot
	// can only be a run the previous process took down with it.
	MarkRunningCommandRunsLost(ctx context.Context, reason string) (int64, error)
}
