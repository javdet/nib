package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/executor"
	"github.com/javdet/nib/internal/llm"
)

const ExecuteActionToolName = "execute_action"

var executeActionParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "number": {
      "type": "string",
      "description": "The item's number as shown in the web interface: \"1.1\" for an action, \"1.C1\" for a verification check, \"R1\" for a rollback entry."
    },
    "rerun": {
      "type": "boolean",
      "description": "Run the action again from scratch, replacing any attempt still in progress. Use it when the operator asks to restart, repeat or retry an action."
    }
  },
  "required": ["number"]
}`)

// ExecuteActionToolDef returns the LLM tool definition for running one action of
// the plan bound to the conversation.
func ExecuteActionToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: ExecuteActionToolName,
		Description: "Carry out one item of the action plan bound to this conversation, named by the number the operator sees beside it. " +
			"A code action is handed to the coding agent, which opens a pull request; every other action, and every verification check, is given to a sub-agent that carries it out and reports back here when it finishes. " +
			"Returns as soon as the work is started, not when it is done.",
		Parameters: executeActionParameters,
	}
}

// executeActionHandler runs an action of the plan the dialog is bound to.
func (s *ChatService) executeActionHandler(b toolBinding) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		text, _, err := s.executePlanItem(ctx, b.planID, argString(args["number"]), argBool(args["rerun"]))
		return text, err
	}
}

// executePlanItem carries out one item of the plan owned by planOwnerID, named
// as the operator sees it. A code action goes to the coding agent in a container;
// anything else -- including a verification check, which is run rather than left
// for the operator to confirm by hand -- goes to a sub-agent of its own.
//
// It reports whether the work actually started, so a caller can tell a refusal
// from a launch. Everything an operator or a model can get wrong comes back as
// text with a nil error, so a mistyped number costs a sentence rather than the
// turn.
func (s *ChatService) executePlanItem(
	ctx context.Context,
	planOwnerID uuid.UUID,
	number string,
	rerun bool,
) (string, bool, error) {
	raw := strings.TrimSpace(number)
	if raw == "" {
		return "number is required: name the action as the web interface does, for example 1.1", false, nil
	}

	key, ok := parseActionPlanNumber(raw)
	if !ok {
		return fmt.Sprintf("%q is not a plan item number; use 1.1 for an action, 1.C1 for a check or R1 for a rollback entry", raw), false, nil
	}

	planID, err := s.resolveRootDialogID(ctx, planOwnerID)
	if err != nil {
		return "", false, err
	}

	display := actionPlanNumberForKey(key)
	if display == "" {
		display = raw
	}

	started, err := s.startPlanItem(ctx, planID, key, rerun)
	if err != nil {
		text, ok := planItemRefusal(err, display)
		if !ok {
			return "", false, err
		}
		return text, false, nil
	}

	if started.Code {
		return fmt.Sprintf(
			"handed %s to the coding agent (job %s, branch %s); the pull request will be attached to the action when it finishes, and its progress is in chat %s",
			display, started.Run.JobName, started.Run.TargetBranch, started.Dialog.ID,
		), true, nil
	}
	return fmt.Sprintf("started a sub-agent for %s; its result will be posted in the chat when it finishes", display), true, nil
}

// planItemStart is what launching one plan item produced. Dialog and Run are
// only set for a code action.
type planItemStart struct {
	Code   bool
	Dialog domain.Dialog
	Run    executor.ActionRunResult
}

// startPlanItem launches one item of the root plan planID by its row key. It is
// the typed half of executePlanItem, shared with the stage run, which has no
// model to hand a sentence to.
func (s *ChatService) startPlanItem(ctx context.Context, planID uuid.UUID, key string, rerun bool) (planItemStart, error) {
	step, err := s.readActionPlanStep(planID, key)
	if err != nil {
		return planItemStart{}, err
	}

	// A code action is built by the coding agent in a container of its own,
	// which reports back through the agent-runner webhook rather than a
	// sub-agent turn.
	if strings.EqualFold(strings.TrimSpace(step.Type), actionTypeCode) {
		dialog, run, err := s.ExecuteCodeAction(ctx, planID, key)
		if err != nil {
			return planItemStart{}, err
		}
		s.recordCodeActionRun(planID, key, run)
		return planItemStart{Code: true, Dialog: dialog, Run: run}, nil
	}

	if _, err := s.StartActionAgent(ctx, planID, key, rerun); err != nil {
		return planItemStart{}, err
	}
	return planItemStart{}, nil
}

// planItemRefusal turns a launch failure the operator can do something about
// into the sentence saying what. Its preconditions -- an enabled executor, a
// repository on the step, configured secrets, a free execution slot -- are all
// fixed in the web interface, so the agent has to be able to say which one is
// missing. It reports false for a failure that is not one of those.
func planItemRefusal(err error, number string) (string, bool) {
	switch {
	case errors.Is(err, ErrActionNotFound):
		return fmt.Sprintf("the plan has no item %s; read the action list to see the numbers it does have", number), true
	case errors.Is(err, ErrActionAlreadyRunning):
		return fmt.Sprintf("%s is already running; ask to restart it if you want a fresh attempt", number), true
	case errors.Is(err, ErrExecutionBusy):
		// The error already names what holds the slot.
		return executionBusyMessage(err), true
	case errors.Is(err, executor.ErrExecutorDisabled):
		return fmt.Sprintf("%s is a code action, but the executor is disabled; switch it on in executor settings to run code actions", number), true
	case errors.Is(err, ErrActionRepositoryRequired):
		return fmt.Sprintf("%s is a code action with no repository set; add one to the plan first", number), true
	case errors.Is(err, ErrExecutorTokenSecretRequired),
		errors.Is(err, ErrExecutorGitTokenSecretRequired),
		errors.Is(err, ErrExecutorSecretMissing):
		return fmt.Sprintf("%s could not start: %s", number, err.Error()), true
	}
	return "", false
}

// recordCodeActionRun writes the running record of a code action whose
// container has just been launched, and says so on the plan's event stream the
// way a sub-agent run does -- without it the row stayed idle until the result
// came back.
func (s *ChatService) recordCodeActionRun(planID uuid.UUID, key string, run executor.ActionRunResult) {
	if _, err := s.updateActionPlanExecRun(planID, key, func(r *ActionExecRun) {
		attempt := r.Attempt + 1
		*r = ActionExecRun{
			Status:      ActionExecRunning,
			StartedAt:   time.Now().Unix(),
			Attempt:     attempt,
			Kind:        ExecutionKindContainer,
			JobName:     run.JobName,
			ContainerID: run.ContainerID,
			Namespace:   run.Namespace,
		}
	}); err != nil {
		// The container is already building; a missing status row is a cosmetic
		// loss, not a reason to report the action as failed.
		slog.Warn("record code action run", "plan_id", planID, "key", key, "error", err)
	}

	if s.activity != nil {
		s.activity.Publish(planID, domain.AgentActivity{
			Kind:   domain.ActivityActionExecStarted,
			Action: key,
			Status: string(ActionExecRunning),
		})
	}
}
