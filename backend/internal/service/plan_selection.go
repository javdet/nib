package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/domain"
)

// planSelectionPath is the project/environment/cloud/location a plan was started
// under, keyed by its root dialog.
//
// The selection itself is global and in-memory, so reading it when a plan ends
// would answer with whatever the operator has selected by then. Anything that
// acts on a finished plan -- today the knowledge-base update -- needs the
// selection the plan was actually planned under.
func planSelectionPath(dir string, dialogID uuid.UUID) string {
	return filepath.Join(dir, dialogID.String()+".json")
}

// WritePlanSelection records the selection a plan belongs to. It is written once,
// on the plan's first turn, and never revised: a plan does not change project.
func (s *ChatService) WritePlanSelection(dialogID uuid.UUID, sel domain.Selection) error {
	if err := os.MkdirAll(s.planSelectionDir, 0o755); err != nil {
		return fmt.Errorf("create plan selection directory: %w", err)
	}

	b, err := json.MarshalIndent(sel, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plan selection: %w", err)
	}
	if err := atomicfile.Write(planSelectionPath(s.planSelectionDir, dialogID), b); err != nil {
		return fmt.Errorf("write plan selection: %w", err)
	}
	return nil
}

// ReadPlanSelection returns the recorded selection for a plan. A plan created
// before this file existed reports found=false, and callers fall back to the
// live selection.
func (s *ChatService) ReadPlanSelection(dialogID uuid.UUID) (domain.Selection, bool, error) {
	b, err := os.ReadFile(planSelectionPath(s.planSelectionDir, dialogID))
	if errors.Is(err, os.ErrNotExist) {
		return domain.Selection{}, false, nil
	}
	if err != nil {
		return domain.Selection{}, false, fmt.Errorf("read plan selection: %w", err)
	}

	var sel domain.Selection
	if err := json.Unmarshal(b, &sel); err != nil {
		return domain.Selection{}, false, fmt.Errorf("unmarshal plan selection: %w", err)
	}
	return sel, true, nil
}

// planSelection returns the selection a plan was started under, falling back to
// the current one for plans that predate the snapshot.
func (s *ChatService) planSelection(rootID uuid.UUID) domain.Selection {
	if sel, found, err := s.ReadPlanSelection(rootID); err == nil && found {
		return sel
	}
	if s.selection == nil {
		return domain.Selection{}
	}
	return s.selection.Get()
}
