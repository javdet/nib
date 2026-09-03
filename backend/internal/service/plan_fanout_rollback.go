package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

// rollbackStageTitle is what the rollback agent is displayed under in the run's
// progress list. It is only a label: the entry is addressed by its Kind, so a DAG
// stage of the same name is a cosmetic collision rather than a correctness one.
const rollbackStageTitle = "Rollback plan"

// runRollbackAgent works out the plan's rollback list once every stage has been
// written. It runs alone, after the last wave, because the rollback undoes the
// plan as a whole: it can only be ordered, and its per-repository code reverts
// only merged, by an agent that sees every stage at once.
//
// Every failure is recorded against the run rather than returned, the way a stage
// failure is: the fan-out reports what landed.
func (s *ChatService) runRollbackAgent(ctx context.Context, rootID uuid.UUID) {
	if ctx.Err() != nil {
		return
	}

	fail := func(err error) {
		slog.Error("plan fanout: rollback failed", "dialog_id", rootID, "error", err)
		_ = s.setFanoutRollback(rootID, func(st *FanoutStage) {
			st.Status = FanoutStageFailed
			st.Error = err.Error()
		})
		s.activity.Publish(rootID, domain.AgentActivity{
			Kind:   domain.ActivityPlanStageFailed,
			Stage:  rollbackStageTitle,
			Status: string(FanoutStageFailed),
		})
	}

	skip := func(reason string) {
		slog.Info("plan fanout: rollback skipped", "dialog_id", rootID, "reason", reason)
		_ = s.setFanoutRollback(rootID, func(st *FanoutStage) {
			st.Status = FanoutStageDone
			st.Error = reason
		})
		s.activity.Publish(rootID, domain.AgentActivity{
			Kind:   domain.ActivityPlanStageDone,
			Stage:  rollbackStageTitle,
			Status: string(FanoutStageDone),
		})
	}

	// A plan whose stages all failed has nothing to undo, and writing a rollback
	// would create the plan document for work that was never planned.
	stages, err := s.plannedStages(rootID)
	if err != nil {
		fail(err)
		return
	}
	if len(stages) == 0 {
		skip("no stages were written, so there is nothing to roll back")
		return
	}

	// An operator part-way through executing the rollback is relying on the list
	// in front of them; rewriting it under their hands is worse than leaving it
	// slightly behind the stages.
	if state, err := s.ReadPlanState(rootID); err != nil {
		slog.Warn("plan fanout: read plan state", "dialog_id", rootID, "error", err)
	} else if state.Status == ActionPlanStatusRolledBack {
		skip("the plan is being rolled back, so its rollback list was left as it is")
		return
	}

	s.activity.Publish(rootID, domain.AgentActivity{
		Kind:  domain.ActivityPlanStageStarted,
		Stage: rollbackStageTitle,
	})

	rollbackDialog, err := s.dialogRepo.CreateDialog(ctx, stagePlanMode, rollbackStageTitle, &rootID)
	if err != nil {
		fail(fmt.Errorf("create rollback dialog: %w", err))
		return
	}
	if err := s.setFanoutRollback(rootID, func(st *FanoutStage) {
		st.Status = FanoutStageRunning
		st.DialogID = rollbackDialog.ID.String()
	}); err != nil {
		fail(err)
		return
	}

	sysPrompt, err := s.fanoutSystemPrompt(ctx, rollbackPromptName)
	if err != nil {
		fail(err)
		return
	}
	seed, err := s.buildRollbackSeed(ctx, rootID, stages)
	if err != nil {
		fail(err)
		return
	}

	if _, err := s.dialogRepo.AppendMessage(ctx, rollbackDialog.ID, domain.DialogMessage{
		Role:    "system",
		Content: sysPrompt,
	}); err != nil {
		fail(fmt.Errorf("append system: %w", err))
		return
	}
	if _, err := s.dialogRepo.AppendMessage(ctx, rollbackDialog.ID, domain.DialogMessage{
		Role:    "user",
		Content: seed,
	}); err != nil {
		fail(fmt.Errorf("append seed: %w", err))
		return
	}

	allow, err := s.rollbackAllowSet(ctx, rootID)
	if err != nil {
		fail(fmt.Errorf("rollback allow set: %w", err))
		return
	}
	catalog, err := s.buildToolCatalog(ctx, allow, toolBinding{
		dialogID: rollbackDialog.ID,
		planID:   rootID,
		stage:    rollbackStageTitle,
		kind:     FanoutStageKindRollback,
	})
	if err != nil {
		fail(fmt.Errorf("build rollback catalog: %w", err))
		return
	}

	if _, err := s.runPersistingAgentLoop(ctx, rollbackDialog.ID, stagePlanMode, catalog, loopConfig{
		planID:        rootID,
		stage:         rollbackStageTitle,
		kind:          FanoutStageKindRollback,
		maxIterations: s.fanout.StageMaxIterations,
	}); err != nil {
		fail(err)
		return
	}

	_ = s.setFanoutRollback(rootID, func(st *FanoutStage) {
		st.Status = FanoutStageDone
	})
	s.activity.Publish(rootID, domain.AgentActivity{
		Kind:   domain.ActivityPlanStageDone,
		Stage:  rollbackStageTitle,
		Status: string(FanoutStageDone),
	})
}

// rollbackAllowSet is the fan-out allow set narrowed for the rollback agent: it
// writes the rollback and nothing else, so update_action_plan comes out and
// update_rollback_plan goes in.
func (s *ChatService) rollbackAllowSet(ctx context.Context, rootID uuid.UUID) (map[string]struct{}, error) {
	allow, err := s.planFanoutAllowSet(ctx, rootID)
	if err != nil {
		return nil, err
	}

	delete(allow, UpdateActionPlanToolName)
	// Injected rather than left to seed/tools/plan.json, which an upgraded
	// install already has on disk and never has a tool added to.
	allow[UpdateRollbackPlanToolName] = struct{}{}
	return allow, nil
}

// plannedStages returns the stages of the stored plan, typed, so unmodelled keys
// and the pull-request urls execution stamps on do not reach the seed.
func (s *ChatService) plannedStages(rootID uuid.UUID) ([]storedActionStage, error) {
	raw, found, err := s.ReadActionPlan(rootID)
	if err != nil {
		return nil, fmt.Errorf("read action plan: %w", err)
	}
	if !found {
		return nil, nil
	}

	var plan storedActionPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil, fmt.Errorf("unmarshal action plan: %w", err)
	}
	return plan.Stages, nil
}

// buildRollbackSeed composes the first user message of the rollback agent: the
// same shared context every stage subagent got, plus the plan those subagents
// actually wrote.
func (s *ChatService) buildRollbackSeed(
	ctx context.Context,
	rootID uuid.UUID,
	stages []storedActionStage,
) (string, error) {
	parts, contract, err := s.sharedSeedSections(ctx, rootID)
	if err != nil {
		return "", err
	}
	if section := sharedContractSection(contract); section != "" {
		parts = append(parts, section)
	}

	data, err := json.MarshalIndent(stages, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal stages: %w", err)
	}
	parts = append(parts, "## The plan as written\n\nEvery stage below is finished and settled. "+
		"Undo them in reverse. A stage of the DAG that is missing here is one whose planning failed: "+
		"do not invent an undo for it.\n\n```json\n"+string(data)+"\n```")

	if answers := s.rollbackAnswersSection(rootID); answers != "" {
		parts = append(parts, answers)
	}
	return strings.Join(parts, "\n\n"), nil
}

// rollbackAnswersSection replays the user's answers to questions an earlier
// attempt at the rollback raised.
func (s *ChatService) rollbackAnswersSection(rootID uuid.UUID) string {
	run, found, err := s.ReadFanoutRun(rootID)
	if err != nil || !found {
		return ""
	}
	return answersSection(run.Blockers, func(b PlanBlocker) bool {
		return b.Kind == FanoutStageKindRollback
	})
}
