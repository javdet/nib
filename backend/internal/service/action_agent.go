package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

const (
	defaultActionExecMaxIterations = 20
	defaultActionExecTimeout       = 30 * time.Minute
	// actionExecPromptName is the prompt appended to execute.md for a subagent
	// that owns a single action.
	actionExecPromptName = "execute_action"
)

// ActionExecConfig tunes the per-action subagents. The zero value is the
// shipped default.
type ActionExecConfig struct {
	// Concurrency is retained so an existing config.yaml still loads, and is
	// clamped to one: see ExecutionLease for why one execution at a time is a
	// policy rather than a capacity limit.
	Concurrency    int
	MaxIterations  int
	TimeoutMinutes int
}

func (c ActionExecConfig) withDefaults() ActionExecConfig {
	c.Concurrency = 1
	if c.MaxIterations <= 0 {
		c.MaxIterations = defaultActionExecMaxIterations
	}
	return c
}

func (c ActionExecConfig) timeout() time.Duration {
	if c.TimeoutMinutes <= 0 {
		return defaultActionExecTimeout
	}
	return time.Duration(c.TimeoutMinutes) * time.Minute
}

// StartActionAgent runs one non-code action in a subagent of its own and returns
// as soon as the execution lease is taken. The work continues in the background
// and reports over the plan dialog's SSE stream.
//
// The lease is the claim, and it is global: only one execution runs at a time,
// so a second request is refused with a sentence naming what holds it rather
// than queued behind work the operator cannot see. rerun is the one exception,
// and only for the same row -- restarting an action stops the attempt on it.
func (s *ChatService) StartActionAgent(ctx context.Context, planID uuid.UUID, key string, rerun bool) (ActionExecRun, error) {
	if s.dialogRepo == nil {
		return ActionExecRun{}, fmt.Errorf("start action agent: dialog repository is not configured")
	}

	// The run outlives the turn that started it, so it gets a deadline of its
	// own rather than inheriting one that is about to be cancelled.
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.actionExec.timeout())

	lease := ExecutionLease{
		PlanID:    planID,
		Key:       key,
		Number:    actionPlanNumberForKey(key),
		Kind:      ExecutionKindSubagent,
		StartedAt: time.Now().Unix(),
		cancel:    cancel,
	}

	held, ok := s.acquireExecutionLease(lease)
	if !ok {
		// rerun takes the same row over. It is not a licence to stop somebody
		// else's execution: that is what CancelExecution is for, and it is the
		// operator's call to make.
		if !rerun || held.PlanID != planID || held.Key != key {
			cancel()
			if !rerun && held.PlanID == planID && held.Key == key {
				return ActionExecRun{}, ErrActionAlreadyRunning
			}
			return ActionExecRun{}, newExecutionBusyError(held)
		}
		if err := s.stopLeaseHolder(ctx, held); err != nil {
			cancel()
			return ActionExecRun{}, fmt.Errorf("start action agent: %w", err)
		}
		s.releaseExecutionLease(held.token)
		if held, ok = s.acquireExecutionLease(lease); !ok {
			cancel()
			return ActionExecRun{}, newExecutionBusyError(held)
		}
	}
	token := held.token

	run, err := s.startActionExecRun(planID, key)
	if err != nil {
		cancel()
		s.releaseExecutionLease(token)
		return ActionExecRun{}, err
	}

	go func() {
		defer cancel()
		defer s.releaseExecutionLease(token)
		s.runActionAgent(runCtx, planID, key, token)
	}()

	return run, nil
}

// runActionAgent executes one action in a dialog of its own. Every failure is
// recorded against the run and reported into the plan chat rather than returned:
// nobody is waiting on this call.
func (s *ChatService) runActionAgent(ctx context.Context, planID uuid.UUID, key string, token uuid.UUID) {
	number := actionPlanNumberForKey(key)

	s.activity.Publish(planID, domain.AgentActivity{
		Kind:   domain.ActivityActionExecStarted,
		Action: key,
		Status: string(ActionExecRunning),
	})

	// The dialog is only known once it is created, so the reporter reads it
	// through a variable the happy path fills in; a failure before that still
	// reports, just without a transcript to link to.
	var dialogID uuid.UUID
	finish := func(status ActionExecStatus, errMsg, body string) {
		run := s.finishActionExecRun(planID, key, status, errMsg)
		s.reportActionResult(context.WithoutCancel(ctx), planID, key, run.Attempt, dialogID, status, body)
	}
	fail := func(err error) {
		// A run the operator stopped is not a failed one, and recording it as
		// failed hides the difference in the plan view. A run that ran out of
		// time is a failure: nobody asked for it to stop.
		switch {
		case errors.Is(ctx.Err(), context.Canceled):
			slog.Info("action agent stopped", "plan_id", planID, "key", key)
			finish(ActionExecCancelled, "stopped before it finished", "")
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			slog.Error("action agent timed out", "plan_id", planID, "key", key,
				"timeout", s.actionExec.timeout())
			finish(ActionExecFailed, fmt.Sprintf("timed out after %s", s.actionExec.timeout()), "")
		default:
			slog.Error("action agent failed", "plan_id", planID, "key", key, "error", err)
			finish(ActionExecFailed, err.Error(), "")
		}
	}

	step, err := s.readActionPlanStep(planID, key)
	if err != nil {
		fail(err)
		return
	}

	dialog, err := s.dialogRepo.CreateDialog(ctx, executeDialogMode, buildExecuteDialogTitle(step), &planID)
	if err != nil {
		fail(fmt.Errorf("create action dialog: %w", err))
		return
	}
	dialogID = dialog.ID
	s.stampExecutionLease(token, func(l *ExecutionLease) {
		l.DialogID = dialog.ID
	})
	// runs.json is the one key -> dialog map; the agent-runner webhook reverse
	// looks up in it, so a subagent registers there too rather than in a map of
	// its own.
	if err := s.recordActionPlanRun(planID, key, dialog.ID); err != nil {
		fail(err)
		return
	}

	sysPrompt, err := s.actionAgentSystemPrompt(ctx)
	if err != nil {
		fail(err)
		return
	}
	seed, err := s.buildActionSeed(planID, key, number, step)
	if err != nil {
		fail(err)
		return
	}

	if _, err := s.dialogRepo.AppendMessage(ctx, dialog.ID, domain.DialogMessage{
		Role:    "system",
		Content: sysPrompt,
	}); err != nil {
		fail(fmt.Errorf("append system: %w", err))
		return
	}
	if _, err := s.dialogRepo.AppendMessage(ctx, dialog.ID, domain.DialogMessage{
		Role:    "user",
		Content: seed,
	}); err != nil {
		fail(fmt.Errorf("append seed: %w", err))
		return
	}

	allow, err := s.actionAgentAllowSet(ctx, planID)
	if err != nil {
		fail(fmt.Errorf("action agent allow set: %w", err))
		return
	}
	catalog, err := s.buildToolCatalog(ctx, allow, toolBinding{
		dialogID: dialog.ID,
		planID:   planID,
	})
	if err != nil {
		fail(fmt.Errorf("build action catalog: %w", err))
		return
	}

	resp, err := s.runPersistingAgentLoop(ctx, dialog.ID, executeDialogMode, catalog, loopConfig{
		planID:        planID,
		maxIterations: s.actionExec.MaxIterations,
	})
	if err != nil {
		fail(err)
		return
	}

	// A subagent cannot suspend for an answer, so a question is where this run
	// ends. The loop returns it with a nil error, which would otherwise read as
	// success and mark the action done.
	if resp.Status == "awaiting_input" {
		question := firstQuestionText(resp.Questions)
		s.answerPendingAskForSubagent(ctx, dialog.ID, resp.ToolCallID)
		finish(ActionExecBlocked, question, question)
		return
	}

	finish(ActionExecDone, "", resp.Response)
}

// answerPendingAskForSubagent closes the ask_question the loop left unanswered.
// Without it the subagent's transcript keeps an assistant row whose tool call has
// no result, which the web interface renders as a live question an operator can
// answer -- resuming the run outside its semaphore and its deadline.
func (s *ChatService) answerPendingAskForSubagent(ctx context.Context, dialogID uuid.UUID, toolCallID string) {
	if strings.TrimSpace(toolCallID) == "" {
		return
	}
	if _, err := s.appendMessageLocked(ctx, dialogID, domain.DialogMessage{
		Role:       "tool",
		Content:    subagentCannotAskPayload(toolCallID),
		ToolCallID: toolCallID,
		Name:       AskQuestionToolName,
	}); err != nil {
		slog.Warn("close subagent question", "dialog_id", dialogID, "error", err)
	}
}

func firstQuestionText(questions []domain.Question) string {
	for _, q := range questions {
		if text := strings.TrimSpace(q.Question); text != "" {
			return text
		}
	}
	return "the sub-agent needed a decision from the operator"
}
