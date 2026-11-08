package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/metrics"
	"github.com/javdet/nib/internal/repository"
)

// PlanState holds lifecycle status and schedule for a plan (decompose dialog).
type PlanState struct {
	Status      ActionPlanStatus `json:"status"`
	ScheduledAt int64            `json:"scheduledAt"`
}

// ReadPlanState returns persisted plan state for a dialog, defaulting to draft.
func (s *ChatService) ReadPlanState(dialogID uuid.UUID) (PlanState, error) {
	var state PlanState
	found, err := s.loadRunState(repository.RunStatePlanState, dialogID, &state)
	if err != nil {
		return PlanState{}, fmt.Errorf("read plan state: %w", err)
	}
	if !found {
		return PlanState{Status: ActionPlanStatusDraft, ScheduledAt: 0}, nil
	}
	return normalizePlanState(state), nil
}

func normalizePlanState(state PlanState) PlanState {
	if !isValidActionPlanStatus(state.Status) {
		state.Status = ActionPlanStatusDraft
	}
	if state.ScheduledAt < 0 {
		state.ScheduledAt = 0
	}
	return state
}

// WritePlanState persists plan state for a dialog.
func (s *ChatService) WritePlanState(dialogID uuid.UUID, state PlanState) error {
	if !isValidActionPlanStatus(state.Status) {
		return fmt.Errorf("invalid action plan status: %q", state.Status)
	}
	if state.ScheduledAt < 0 {
		state.ScheduledAt = 0
	}

	// The previous status is read back rather than passed in: both callers
	// already hold it, but recording here means a third one cannot forget to.
	// A status write is an operator action or a checkbox change, so the extra
	// read is not on any hot path.
	if previous, err := s.ReadPlanState(dialogID); err == nil {
		metrics.RecordPlanStatusTransition(string(previous.Status), string(state.Status))
		// The same transition, persisted. The Prometheus counter answers "how
		// often" for alerting; this answers "when", which is the only thing the
		// statistics page can build a status-over-time series from -- plan state
		// keeps just the current value.
		s.recordPlanStatusTransition(dialogID, string(previous.Status), string(state.Status))
	}

	if err := s.saveRunState(repository.RunStatePlanState, dialogID, state); err != nil {
		return fmt.Errorf("write plan state: %w", err)
	}
	return nil
}

// ReadPlanStates returns plan state per dialog id, defaulting to draft when missing.
func (s *ChatService) ReadPlanStates(ids []uuid.UUID) map[uuid.UUID]PlanState {
	if len(ids) == 0 {
		return nil
	}

	result := make(map[uuid.UUID]PlanState, len(ids))
	for _, id := range ids {
		result[id] = PlanState{Status: ActionPlanStatusDraft, ScheduledAt: 0}
	}

	// One query for the whole list rather than one per plan: the plans page
	// asks for every plan at once.
	ctx, cancel := context.WithTimeout(context.Background(), runStateTimeout)
	defer cancel()
	docs, err := s.runStateStore().GetRunStates(ctx, repository.RunStatePlanState, ids)
	if err != nil {
		slog.Warn("read plan states", "error", err)
		return result
	}
	for id, b := range docs {
		var state PlanState
		if err := json.Unmarshal(b, &state); err != nil {
			continue
		}
		result[id] = normalizePlanState(state)
	}
	return result
}

// planProgress counts how many of the plan's stage steps and checks are marked
// executed. Rollback items are excluded: they are the undo path and are normally
// never executed, so counting them would make a plan impossible to finish.
func planProgress(plan storedActionPlan, checked []string) (done, total int) {
	checkedSet := make(map[string]struct{}, len(checked))
	for _, key := range checked {
		checkedSet[key] = struct{}{}
	}

	count := func(stageIdx, itemCount int, scope ActionPlanScope) {
		for idx := 0; idx < itemCount; idx++ {
			total++
			if _, ok := checkedSet[actionPlanItemKey(stageIdx, scope, idx)]; ok {
				done++
			}
		}
	}

	for stageIdx, stage := range plan.Stages {
		count(stageIdx, len(stage.Steps), ActionPlanScopeSteps)
		count(stageIdx, len(stage.Checks), ActionPlanScopeChecks)
	}
	return done, total
}

// nextPlanStatusForChecks resolves the plan status implied by checkbox progress.
//
// Completing every stage step and check finishes the plan (done); unchecking an
// item afterwards moves it to reopened. Setting the first checkbox still promotes
// a draft or scheduled plan to in_progress, and a plan that has been rolled back
// or cancelled is never moved automatically.
func nextPlanStatusForChecks(current ActionPlanStatus, done, total int) ActionPlanStatus {
	if current == ActionPlanStatusRolledBack || current == ActionPlanStatusCancelled {
		return current
	}

	if total > 0 && done == total {
		return ActionPlanStatusDone
	}

	switch current {
	case ActionPlanStatusDone:
		return ActionPlanStatusReopened
	case ActionPlanStatusDraft, ActionPlanStatusScheduled:
		if done > 0 {
			return ActionPlanStatusInProgress
		}
	}
	return current
}

// SyncPlanStatusForChecks updates plan status to match action plan checkbox
// progress and returns the resulting status. Plan content is read from the
// action plan dialog, while status is stored against its parent dialog.
func (s *ChatService) SyncPlanStatusForChecks(
	ctx context.Context,
	actionPlanDialogID uuid.UUID,
	checked []string,
) (ActionPlanStatus, error) {
	planID, err := s.resolveRootDialogID(ctx, actionPlanDialogID)
	if err != nil {
		return "", err
	}

	state, err := s.ReadPlanState(planID)
	if err != nil {
		return "", err
	}

	var plan storedActionPlan
	raw, found, err := s.ReadActionPlan(actionPlanDialogID)
	if err != nil {
		return "", fmt.Errorf("read action plan: %w", err)
	}
	if found {
		if err := json.Unmarshal(raw, &plan); err != nil {
			plan = storedActionPlan{}
		}
	}

	done, total := planProgress(plan, checked)
	if total == 0 {
		// The plan is missing, empty or malformed, so completion cannot be
		// determined. Fall back to counting raw checks, which keeps the
		// draft/scheduled -> in_progress promotion working.
		done = len(checked)
	}
	next := nextPlanStatusForChecks(state.Status, done, total)
	if next == state.Status {
		return state.Status, nil
	}

	state.Status = next
	if err := s.WritePlanState(planID, state); err != nil {
		return "", err
	}
	return state.Status, nil
}
