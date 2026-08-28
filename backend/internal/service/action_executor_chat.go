package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
)

const executorDialogTitle = "Executor"

// EnsureExecutorDialog returns the single execute chat shared by all non-code
// actions of a plan, creating it on first use. created is true only when this
// call created the chat, which lets the caller seed it with plan context.
func (s *ChatService) EnsureExecutorDialog(ctx context.Context, planDialogID uuid.UUID) (domain.Dialog, bool, error) {
	if s.dialogRepo == nil {
		return domain.Dialog{}, false, fmt.Errorf("ensure executor dialog: dialog repository is not configured")
	}

	execID, found, err := s.ReadActionPlanExecutorDialog(planDialogID)
	if err != nil {
		return domain.Dialog{}, false, fmt.Errorf("ensure executor dialog: read pointer: %w", err)
	}
	if found {
		d, err := s.dialogRepo.GetDialog(ctx, execID)
		if err == nil {
			return d, false, nil
		}
		if !errors.Is(err, repository.ErrNotFound) {
			return domain.Dialog{}, false, fmt.Errorf("ensure executor dialog: get dialog: %w", err)
		}
	}

	dialog, err := s.dialogRepo.CreateDialog(ctx, executeDialogMode, executorDialogTitle, &planDialogID)
	if err != nil {
		return domain.Dialog{}, false, fmt.Errorf("ensure executor dialog: create dialog: %w", err)
	}

	if err := s.WriteActionPlanExecutorDialog(planDialogID, dialog.ID); err != nil {
		return domain.Dialog{}, false, fmt.Errorf("ensure executor dialog: write pointer: %w", err)
	}

	return dialog, true, nil
}
