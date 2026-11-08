package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
)

// actionPlanRebuildNext is what the orchestrator is told once a rebuild has
// started. Without it the model sees a stale plan in its transcript and a DAG
// that has moved on, and launches plan itself.
const actionPlanRebuildNext = "Relay the decompose summary, then tell the operator the action list is being " +
	"rebuilt from scratch for the new DAG. It reports into this chat as stages land; do not launch plan yourself."

// actionPlanRebuildOwedNext is what the orchestrator is told when the rebuild
// could not start, and the plan still describes the old DAG.
const actionPlanRebuildOwedNext = "Relay the decompose summary and tell the operator the action list is out of " +
	"date. Once nothing of this plan is running, a plan launch with no stages discards it and rebuilds it from " +
	"scratch; launch it when the operator asks, passing their words in operator_request."

// markActionPlanForRebuild records that the DAG is being rewritten under an
// existing action plan. It reports whether there was one to mark: a DAG
// revised before anything was planned leaves nothing stale behind.
func (s *ChatService) markActionPlanForRebuild(planID uuid.UUID) (bool, error) {
	_, found, err := s.ReadActionPlan(planID)
	if err != nil {
		return false, fmt.Errorf("mark action plan for rebuild: read action plan: %w", err)
	}
	if !found {
		return false, nil
	}
	if err := s.setActionPlanRebuild(planID, true); err != nil {
		return false, fmt.Errorf("mark action plan for rebuild: %w", err)
	}
	return true, nil
}

func (s *ChatService) setActionPlanRebuild(planID uuid.UUID, owed bool) error {
	_, err := s.updateSubagentState(planID, func(state *SubagentState) {
		state.RebuildActionPlan = owed
	})
	return err
}

// actionPlanRebuildOwed reports whether the DAG has changed since the action
// plan was built.
func (s *ChatService) actionPlanRebuildOwed(planID uuid.UUID) (bool, error) {
	state, _, err := s.ReadSubagentState(planID)
	if err != nil {
		return false, err
	}
	return state.RebuildActionPlan, nil
}

// takeActionPlanRebuild clears the mark and reports whether it was set. Taking
// it in one step is what keeps two callers from both discarding the plan: the
// second would wipe the stages the first one's fan-out has already written.
func (s *ChatService) takeActionPlanRebuild(planID uuid.UUID) (bool, error) {
	owed, err := s.actionPlanRebuildOwed(planID)
	if err != nil || !owed {
		return false, err
	}

	taken := false
	if _, err := s.updateSubagentState(planID, func(state *SubagentState) {
		taken = state.RebuildActionPlan
		state.RebuildActionPlan = false
	}); err != nil {
		return false, err
	}
	return taken, nil
}

// rebuildActionPlanAfterDAGChange plans a rewritten DAG from scratch when an
// action plan was built for the one before it: every stage, then the rollback.
//
// A stage-by-stage replan is not enough. Stages the new DAG dropped would stay
// on the plan, upstream stages would be read back in their old form by the
// planners seeding off them, and checks, runs and notes would sit on rows that
// now describe different work.
//
// It returns a notice for the operator and what the orchestrator should do next,
// both empty when there was nothing to rebuild.
func (s *ChatService) rebuildActionPlanAfterDAGChange(ctx context.Context, rootID uuid.UUID) (string, string) {
	owed, err := s.actionPlanRebuildOwed(rootID)
	if err != nil {
		slog.Warn("rebuild action plan: read mark", "root_id", rootID, "error", err)
		return "", ""
	}
	if !owed {
		return "", ""
	}

	if reason := s.actionPlanRebuildBlocker(rootID); reason != "" {
		return fmt.Sprintf("The action list was not rebuilt for the new DAG, because %s. "+
			"It still describes the previous DAG.", reason), actionPlanRebuildOwedNext
	}

	run, err := s.StartPlanFanout(ctx, rootID, AllFanoutTargets())
	switch {
	case err == nil:
		summary := plannedStagesSummary(run)
		return strings.ToUpper(summary[:1]) + summary[1:] + ".", actionPlanRebuildNext
	case errors.Is(err, ErrNoDAGStages):
		return "The new DAG names no stages, so the action list could not be rebuilt for it. " +
			"It still describes the previous DAG.", actionPlanRebuildOwedNext
	default:
		slog.Error("rebuild action plan: start fan-out", "root_id", rootID, "error", err)
		return fmt.Sprintf("The action list could not be rebuilt for the new DAG (%v). "+
			"It still describes the previous DAG.", err), actionPlanRebuildOwedNext
	}
}

// actionPlanRebuildBlocker names what keeps the plan from being discarded right
// now, or returns "" when nothing does. A code fix holding the lease does not
// count: it is tied to a branch, not to a row.
func (s *ChatService) actionPlanRebuildBlocker(rootID uuid.UUID) string {
	run, found, err := s.ReadFanoutRun(rootID)
	if err != nil {
		return "its planning record could not be read"
	}
	if found && run.Active() {
		return "planning is still running for this plan"
	}
	return s.rowExecutionBlocker(rootID)
}

// rowExecutionBlocker names an item of this plan that is running, whose result
// would land on whatever row ends up at its position were the plan discarded.
func (s *ChatService) rowExecutionBlocker(rootID uuid.UUID) string {
	if lease, held := s.ExecutionInProgress(); held && !lease.Fix && lease.PlanID == rootID {
		return leaseSubject(lease) + " of this plan is still running"
	}
	return ""
}

// discardStaleActionPlan discards the action plan when a rebuild is owed, ahead
// of a fan-out of every stage. It reports whether it did. An item of the plan
// still running holds it off: the run then replans over the old plan as it
// always has, and the rebuild stays owed for the next one.
func (s *ChatService) discardStaleActionPlan(rootID uuid.UUID) (bool, error) {
	taken, err := s.takeActionPlanRebuild(rootID)
	if err != nil {
		return false, fmt.Errorf("read rebuild mark: %w", err)
	}
	if !taken {
		return false, nil
	}

	if reason := s.rowExecutionBlocker(rootID); reason != "" {
		slog.Warn("rebuild action plan: held off", "root_id", rootID, "reason", reason)
		if err := s.setActionPlanRebuild(rootID, true); err != nil {
			slog.Warn("rebuild action plan: restore mark", "root_id", rootID, "error", err)
		}
		return false, nil
	}

	if err := s.discardActionPlan(rootID); err != nil {
		if markErr := s.setActionPlanRebuild(rootID, true); markErr != nil {
			slog.Warn("rebuild action plan: restore mark", "root_id", rootID, "error", markErr)
		}
		return false, err
	}
	slog.Info("action plan discarded for a new DAG", "root_id", rootID)
	return true, nil
}

// discardActionPlan removes the action plan, everything recorded against its
// rows and the fan-out record that produced it. The fan-out record goes too
// because its answered blockers are replayed to a stage asking the same
// question, and those answers were given about the old DAG.
//
// A plan that had started moving goes back to draft, since none of its rows has
// been carried out any more. A scheduled, rolled back or cancelled one keeps its
// status: those are the operator's decisions, not checkbox progress.
func (s *ChatService) discardActionPlan(rootID uuid.UUID) error {
	mu := s.planMutex(rootID)
	mu.Lock()
	err := os.Remove(filepath.Join(s.actionPlansDir, rootID.String()+".json"))
	mu.Unlock()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove action plan: %w", err)
	}
	s.clearActionPlanItemState(rootID)

	fmu := s.fanoutMutex(rootID)
	fmu.Lock()
	err = s.dropRunState(repository.RunStateFanout, rootID)
	fmu.Unlock()
	if err != nil {
		return fmt.Errorf("remove fan-out record: %w", err)
	}

	state, err := s.ReadPlanState(rootID)
	if err != nil {
		return fmt.Errorf("read plan state: %w", err)
	}
	switch state.Status {
	case ActionPlanStatusInProgress, ActionPlanStatusDone, ActionPlanStatusReopened:
		state.Status = ActionPlanStatusDraft
		if err := s.WritePlanState(rootID, state); err != nil {
			return fmt.Errorf("reset plan state: %w", err)
		}
	}

	s.activity.Publish(rootID, domain.AgentActivity{Kind: domain.ActivityActionPlanUpdated})
	return nil
}
