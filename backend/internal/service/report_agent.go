package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

const (
	// reportAgentMaxIterations is deliberately far below the action agent's: this
	// subagent reads one tool and writes one, so a run that has not finished in
	// ten rounds is looping rather than working.
	reportAgentMaxIterations = 10
	reportAgentTimeout       = 10 * time.Minute
	// reportPromptName is the overlay appended to execute.md for the subagent
	// that writes a finished plan's report.
	reportPromptName = "execute_report"
	// reportSubagentName keys the launch claim. It is deliberately not an entry
	// in subagentSpecs: the orchestrator never launches this one, the operator
	// does by finishing the plan.
	reportSubagentName SubagentName = "report"
	reportDialogTitle               = "Plan report"
)

// StartReportAgent writes a finished plan's closing report in a subagent of its
// own and returns as soon as that run is on its way.
//
// It deliberately does not take the ExecutionLease. The lease exists because only
// one thing may change managed infrastructure at a time; this agent is handed
// get_action_list and set_report and nothing else, so it changes none. Holding it
// to that rule would mean a plan finished while one last action was still running
// silently loses its report -- the Finish button is not disabled on a running
// action -- which is a worse outcome than two read-only runs overlapping.
//
// Writing the report twice is the one race worth preventing, and the per-plan
// claim below plus the "already written" check are what prevent it.
func (s *ChatService) StartReportAgent(ctx context.Context, planID uuid.UUID) error {
	if s.dialogRepo == nil {
		return fmt.Errorf("start report agent: dialog repository is not configured")
	}

	rootID, err := s.resolveRootDialogID(ctx, planID)
	if err != nil {
		return fmt.Errorf("start report agent: %w", err)
	}

	// The report is written once. A second press of Finish, a replayed request or
	// a double-mounted effect finds the file already there and does nothing --
	// which is also why a failed run leaves none: it has to stay retryable.
	if _, found, err := s.ReadReport(rootID); err != nil {
		return fmt.Errorf("start report agent: read report: %w", err)
	} else if found {
		return nil
	}

	claim := s.subagentClaim(rootID, reportSubagentName)
	if !claim.tryLock() {
		// A run is already in flight for this plan. Nobody is waiting on its
		// result, so there is nothing to report back.
		return nil
	}

	// The run outlives the request that started it, so it gets a deadline of its
	// own rather than inheriting one that is about to be cancelled.
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), reportAgentTimeout)
	go func() {
		defer cancel()
		defer claim.unlock()
		s.runReportAgent(runCtx, rootID)
	}()

	return nil
}

// runReportAgent composes the report in a dialog of its own.
//
// Every failure is logged and goes no further: the operator pressed Finish, that
// succeeded, and nobody is waiting on this. A run that fails writes no report, so
// the plan simply shows no report card.
func (s *ChatService) runReportAgent(ctx context.Context, rootID uuid.UUID) {
	dialog, err := s.dialogRepo.CreateDialog(ctx, executeDialogMode, reportDialogTitle, &rootID)
	if err != nil {
		slog.Error("report agent: create dialog", "plan_id", rootID, "error", err)
		return
	}

	sysPrompt, err := s.reportAgentSystemPrompt(ctx)
	if err != nil {
		slog.Error("report agent: resolve prompt", "plan_id", rootID, "error", err)
		return
	}
	seed, err := s.buildReportSeed(rootID)
	if err != nil {
		slog.Error("report agent: build seed", "plan_id", rootID, "error", err)
		return
	}

	if _, err := s.dialogRepo.AppendMessage(ctx, dialog.ID, domain.DialogMessage{
		Role:    "system",
		Content: sysPrompt,
	}); err != nil {
		slog.Error("report agent: append system", "plan_id", rootID, "error", err)
		return
	}
	if _, err := s.dialogRepo.AppendMessage(ctx, dialog.ID, domain.DialogMessage{
		Role:    "user",
		Content: seed,
	}); err != nil {
		slog.Error("report agent: append seed", "plan_id", rootID, "error", err)
		return
	}

	catalog, err := s.buildToolCatalog(ctx, reportAgentAllowSet(), toolBinding{
		dialogID: dialog.ID,
		planID:   rootID,
		mode:     executeDialogMode,
	})
	if err != nil {
		slog.Error("report agent: build catalog", "plan_id", rootID, "error", err)
		return
	}

	if _, err := s.runPersistingAgentLoop(ctx, dialog.ID, executeDialogMode, catalog, loopConfig{
		planID:        rootID,
		maxIterations: reportAgentMaxIterations,
	}); err != nil {
		slog.Error("report agent failed", "plan_id", rootID, "dialog_id", dialog.ID, "error", err)
		return
	}

	// The loop finishing is not the same as the report existing: the tool call is
	// the deliverable, and a model can end its turn without making it.
	if _, found, err := s.ReadReport(rootID); err == nil && !found {
		slog.Warn("report agent finished without calling set_report", "plan_id", rootID, "dialog_id", dialog.ID)
	}
}

// reportAgentSystemPrompt is the execute prompt plus the overlay that turns it
// from carrying work out into writing up what was carried out.
func (s *ChatService) reportAgentSystemPrompt(ctx context.Context) (string, error) {
	return s.subagentSystemPrompt(ctx, executeDialogMode, reportPromptName)
}

// reportAgentAllowSet is written out here rather than resolved from the execute
// mode list on purpose.
//
// This agent reads a finished plan and writes prose about it. Every other tool
// the execute list carries -- execute_command, api_call, the MCP catalog, the
// secrets -- is a way for a report to have side effects, and there is no reading
// of "summarise what happened" that needs one. Keeping the set literal also means
// an operator's edit to data/tools/execute.json cannot widen it.
func reportAgentAllowSet() map[string]struct{} {
	return map[string]struct{}{
		GetActionListToolName: {},
		SetReportToolName:     {},
	}
}

// buildReportSeed is the first user message of the report subagent: the plan
// context every subagent gets, then what it is being asked for.
//
// The action list itself stays out of it. The agent pulls that with
// get_action_list, the same way the per-action executors do, so there is one
// shape of that data in the codebase rather than two.
func (s *ChatService) buildReportSeed(rootID uuid.UUID) (string, error) {
	parts, _, err := s.sharedSeedSections(rootID)
	if err != nil {
		return "", err
	}

	parts = append(parts, strings.TrimRight(`## Your task

This plan is finished. Call `+"`get_action_list`"+` to see what it contained and what each
action actually did, then write the closing report and store it with `+"`set_report`"+`.`, "\n"))

	return strings.Join(parts, "\n\n"), nil
}
