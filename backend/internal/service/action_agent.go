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
	defaultActionExecConcurrency   = 4
	defaultActionExecMaxIterations = 20
	defaultActionExecTimeout       = 30 * time.Minute
	// actionExecPromptName is the prompt appended to execute.md for a subagent
	// that owns a single action.
	actionExecPromptName = "execute_action"
)

// ActionExecConfig tunes the per-action subagents. The zero value is the
// shipped default.
type ActionExecConfig struct {
	Concurrency    int
	MaxIterations  int
	TimeoutMinutes int
}

func (c ActionExecConfig) withDefaults() ActionExecConfig {
	if c.Concurrency <= 0 {
		c.Concurrency = defaultActionExecConcurrency
	}
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

// execClaimKey identifies one action row of one plan across every in-flight run.
func execClaimKey(planID uuid.UUID, key string) string {
	return planID.String() + "|" + key
}

// StartActionAgent runs one non-code action in a subagent of its own and returns
// as soon as the run is claimed. The work continues in the background and
// reports over the plan dialog's SSE stream.
//
// The claim is the entry in execCancels, not the record on disk: two calls in a
// single round would both read a free row before either could write it. rerun
// stops whatever holds the row and takes it over.
func (s *ChatService) StartActionAgent(ctx context.Context, planID uuid.UUID, key string, rerun bool) (ActionExecRun, error) {
	if s.dialogRepo == nil {
		return ActionExecRun{}, fmt.Errorf("start action agent: dialog repository is not configured")
	}

	claim := execClaimKey(planID, key)
	// The run outlives the turn that started it, so it gets a deadline of its
	// own rather than inheriting one that is about to be cancelled.
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.actionExec.timeout())

	if previous, loaded := s.execCancels.LoadOrStore(claim, cancel); loaded {
		if !rerun {
			cancel()
			return ActionExecRun{}, ErrActionAlreadyRunning
		}
		// Take the row over: stop the attempt holding it, then claim it.
		previous.(context.CancelFunc)()
		s.execCancels.Store(claim, cancel)
	}

	run, err := s.startActionExecRun(planID, key)
	if err != nil {
		cancel()
		s.execCancels.Delete(claim)
		return ActionExecRun{}, err
	}

	go func() {
		defer cancel()
		defer s.execCancels.Delete(claim)
		s.runActionAgent(runCtx, planID, key)
	}()

	return run, nil
}

// CancelActionAgent stops the subagent working an action row. It reports whether
// one was running.
func (s *ChatService) CancelActionAgent(planID uuid.UUID, key string) bool {
	cancel, ok := s.execCancels.LoadAndDelete(execClaimKey(planID, key))
	if !ok {
		return false
	}
	cancel.(context.CancelFunc)()
	return true
}

// runActionAgent executes one action in a dialog of its own. Every failure is
// recorded against the run and reported into the plan chat rather than returned:
// nobody is waiting on this call.
func (s *ChatService) runActionAgent(ctx context.Context, planID uuid.UUID, key string) {
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
		slog.Error("action agent failed", "plan_id", planID, "key", key, "error", err)
		finish(ActionExecFailed, err.Error(), "")
	}

	// Queueing happens here rather than in the tool handler: the handler runs
	// inside the operator's turn, and blocking it would hold the whole chat
	// behind another action's work.
	select {
	case s.execSem <- struct{}{}:
	case <-ctx.Done():
		s.finishActionExecRun(planID, key, ActionExecCancelled, "cancelled before it started")
		s.activity.Publish(planID, domain.AgentActivity{
			Kind:   domain.ActivityActionExecFailed,
			Action: key,
			Status: string(ActionExecCancelled),
		})
		return
	}
	defer func() { <-s.execSem }()

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
	seed, err := s.buildActionSeed(ctx, planID, key, number, step)
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
