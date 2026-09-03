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
	"github.com/javdet/nib/internal/executor"
	"github.com/javdet/nib/internal/llm"
)

const ExecuteActionToolName = "execute_action"

var executeActionParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "number": {
      "type": "string",
      "description": "The action's number as shown in the web interface: \"1.1\" for an action, \"R1\" for a rollback entry."
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
		Description: "Carry out one action of the action plan bound to this conversation, named by the number the operator sees beside it. " +
			"A code action is handed to the coding agent, which opens a pull request; every other action is given to a sub-agent that executes it and reports back here when it finishes. " +
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
// anything else goes to a sub-agent of its own.
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

	// A check is a verification the operator ticks, not work to hand out.
	if strings.Contains(key, ".check") {
		return fmt.Sprintf("%s is a verification check, not an action; carry out the check yourself and report what you found", display), false, nil
	}

	step, err := s.readActionPlanStep(planID, key)
	if err != nil {
		if errors.Is(err, ErrActionNotFound) {
			return fmt.Sprintf("the plan has no action %s; read the action list to see the numbers it does have", display), false, nil
		}
		return "", false, err
	}

	// A code action is built by the coding agent in a container of its own,
	// which reports back through the agent-runner webhook rather than a
	// sub-agent turn.
	if strings.EqualFold(strings.TrimSpace(step.Type), actionTypeCode) {
		text, started, err := s.executeCodeActionFromTool(ctx, planID, key, display)
		return text, started, err
	}

	if _, err := s.StartActionAgent(ctx, planID, key, rerun); err != nil {
		switch {
		case errors.Is(err, ErrActionAlreadyRunning):
			return fmt.Sprintf("%s is already running; ask to restart it if you want a fresh attempt", display), false, nil
		case errors.Is(err, ErrExecutionBusy):
			// The error already names what holds the slot.
			return executionBusyMessage(err), false, nil
		}
		return "", false, err
	}
	return fmt.Sprintf("started a sub-agent for %s; its result will be posted in the chat when it finishes", display), true, nil
}

// executeCodeActionFromTool adapts ExecuteCodeAction's failures into tool output.
// Its preconditions -- an enabled executor, a repository on the step, configured
// secrets -- are all things the operator fixes in the web interface, so the agent
// has to be able to say which one is missing.
func (s *ChatService) executeCodeActionFromTool(ctx context.Context, planID uuid.UUID, key, number string) (string, bool, error) {
	dialog, run, err := s.ExecuteCodeAction(ctx, planID, key)
	switch {
	case err == nil:
	case errors.Is(err, executor.ErrExecutorDisabled):
		return fmt.Sprintf("%s is a code action, but the executor is disabled; switch it on in executor settings to run code actions", number), false, nil
	case errors.Is(err, ErrActionRepositoryRequired):
		return fmt.Sprintf("%s is a code action with no repository set; add one to the plan first", number), false, nil
	case errors.Is(err, ErrExecutorTokenSecretRequired), errors.Is(err, ErrExecutorSecretMissing):
		return fmt.Sprintf("%s could not start: %s", number, err.Error()), false, nil
	case errors.Is(err, ErrExecutionBusy):
		return executionBusyMessage(err), false, nil
	default:
		return "", false, err
	}

	if _, recErr := s.updateActionPlanExecRun(planID, key, func(r *ActionExecRun) {
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
	}); recErr != nil {
		// The container is already building; a missing status row is a cosmetic
		// loss, not a reason to tell the agent the action failed.
		slog.Warn("record code action run", "plan_id", planID, "key", key, "error", recErr)
	}

	return fmt.Sprintf(
		"handed %s to the coding agent (job %s, branch %s); the pull request will be attached to the action when it finishes, and its progress is in chat %s",
		number, run.JobName, run.TargetBranch, dialog.ID,
	), true, nil
}
