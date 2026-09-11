package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/javdet/nib/internal/llm"
	"github.com/google/uuid"
)

const GetActionListToolName = "get_action_list"

var getActionListParameters = json.RawMessage(`{"type":"object","properties":{}}`)

// GetActionListToolDef returns the LLM tool definition for reading the action plan with execution status.
func GetActionListToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: GetActionListToolName,
		Description: "Return the action plan bound to the current conversation. " +
			"Includes stages with actions, verification checks, and rollback steps, each with type and executed status. " +
			"Each action and check carries the `number` the operator sees next to it in the web interface. " +
			"`executed` is the operator's checkbox; `run` is the last sub-agent attempt, which is a weaker claim. " +
			"`notes` is what the last run of that action reported: the concrete values it produced -- an id, a name, " +
			"an address, a branch -- which is where to look for anything an earlier action created.",
		Parameters: getActionListParameters,
	}
}

type actionListResponse struct {
	Stages   []actionListStage  `json:"stages"`
	Rollback []actionListAction `json:"rollback"`
}

type actionListStage struct {
	Number  int                `json:"number"`
	Title   string             `json:"title"`
	Actions []actionListAction `json:"actions"`
	Checks  []actionListCheck  `json:"checks"`
}

type actionListAction struct {
	Number  string `json:"number"`
	Type    string `json:"type"`
	Action  string `json:"action"`
	Command string `json:"command,omitempty"`
	PRTitle string `json:"pr_title,omitempty"`
	PRURL   string `json:"pr_url,omitempty"`
	// Executed is the operator's checkbox: their judgement that the action is
	// genuinely done. Nothing sets it automatically.
	Executed bool `json:"executed"`
	// Run is the last sub-agent attempt at this action, when there has been one.
	// It is deliberately separate from Executed: a sub-agent finishing is not
	// the same claim as the operator accepting the result.
	Run *actionListRun `json:"run,omitempty"`
	// Notes is what the last run of this action reported. It is the sub-agents'
	// working memory and reaches nothing else: not the plan chat, not the web
	// interface. It is here because a value one action produces -- a resource id,
	// a generated name, a branch -- is usually knowable only from the run that
	// produced it, and the action that needs it runs in a conversation of its own.
	Notes string `json:"notes,omitempty"`
}

type actionListRun struct {
	Status  string `json:"status"`
	Attempt int    `json:"attempt"`
	Error   string `json:"error,omitempty"`
}

func actionListRunFor(runs ActionExecRuns, key string) *actionListRun {
	run, ok := runs[key]
	if !ok {
		return nil
	}
	return &actionListRun{
		Status:  string(run.Status),
		Attempt: run.Attempt,
		Error:   run.Error,
	}
}

type actionListCheck struct {
	Number      string `json:"number"`
	Check       string `json:"check"`
	Expectation string `json:"expectation"`
	Executed    bool   `json:"executed"`
}

type storedActionPlan struct {
	Stages   []storedActionStage `json:"stages"`
	Rollback []storedActionStep  `json:"rollback"`
}

type storedActionStage struct {
	Number      int                 `json:"number"`
	Title       string              `json:"title"`
	Description string              `json:"description,omitempty"`
	Steps       []storedActionStep  `json:"steps"`
	Checks      []storedActionCheck `json:"checks"`
}

type storedActionStep struct {
	// Number is the operator-facing label of the step ("1.2", "R1"), derived
	// from its position on every write and never trusted from input.
	Number string `json:"number,omitempty"`
	Type   string `json:"type"`
	Action string `json:"action"`
	// Command holds the verbatim commands of a shell or curl step, kept apart
	// from Action so it can be run without parsing prose.
	Command    string `json:"command,omitempty"`
	Repository string `json:"repository,omitempty"`
	PRTitle    string `json:"pr_title,omitempty"`
	PRURL      string `json:"pr_url,omitempty"`
	// Categories names the tool categories the sub-agent that executes this step
	// needs. It decides that sub-agent's MCP tools and nothing else: no view
	// renders it, and it is never shown to the operator.
	Categories []string `json:"categories,omitempty"`
}

type storedActionCheck struct {
	Number      string `json:"number,omitempty"`
	Check       string `json:"check"`
	Expectation string `json:"expectation"`
}

func (s *ChatService) getActionListHandler(dialogID uuid.UUID) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		planID, err := s.resolveRootDialogID(ctx, dialogID)
		if err != nil {
			return "", err
		}

		raw, found, err := s.ReadActionPlan(planID)
		if err != nil {
			return "", fmt.Errorf("read action plan: %w", err)
		}
		if !found {
			return "no action plan found for this dialog", nil
		}

		checked, err := s.ReadActionPlanChecks(planID)
		if err != nil {
			return "", fmt.Errorf("read action plan checks: %w", err)
		}
		checkedSet := make(map[string]struct{}, len(checked))
		for _, key := range checked {
			checkedSet[key] = struct{}{}
		}

		var plan storedActionPlan
		if err := json.Unmarshal(raw, &plan); err != nil {
			return "action plan contains invalid JSON", nil
		}

		// A failed read costs the run column, not the plan: the agent can still
		// see what it is meant to do.
		execRuns, err := s.ReadActionPlanExecRuns(planID)
		if err != nil {
			slog.Warn("read action plan exec runs", "plan_id", planID, "error", err)
			execRuns = ActionExecRuns{}
		}

		// Same bargain as the run column: a notes file that cannot be read costs
		// the agent its predecessors' results, not the plan it came here to read.
		notes, err := s.readActionPlanNotes(planID)
		if err != nil {
			slog.Warn("read action plan notes", "plan_id", planID, "error", err)
			notes = map[string]string{}
		}

		resp := buildActionListResponse(plan, checkedSet, execRuns, notes)
		out, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshal action list: %w", err)
		}
		return string(out), nil
	}
}

// maxDialogAncestorDepth bounds the parent walk. Real lineage is one hop -- every
// subagent is parented straight to the plan root -- and the cap exists because
// parent_id carries no constraint against a cycle, so a self- or mutually-parented
// row would otherwise spin forever.
const maxDialogAncestorDepth = 8

// resolveRootDialogID walks parent_id up to the dialog that owns the plan
// artifacts. It walks rather than taking one hop because the orchestrator is the
// root now: a single hop from a plan stage subagent would land on the root only
// as long as nothing is ever parented two deep, and the walk costs one query
// for a dialog that is already the root.
func (s *ChatService) resolveRootDialogID(ctx context.Context, dialogID uuid.UUID) (uuid.UUID, error) {
	if s.dialogRepo == nil {
		return dialogID, nil
	}
	seen := make(map[uuid.UUID]struct{}, maxDialogAncestorDepth)
	current := dialogID
	for i := 0; i < maxDialogAncestorDepth; i++ {
		if _, dup := seen[current]; dup {
			slog.Warn("dialog lineage has a cycle, treating this dialog as the plan root",
				"dialog_id", dialogID, "at", current)
			return current, nil
		}
		seen[current] = struct{}{}

		d, err := s.dialogRepo.GetDialog(ctx, current)
		if err != nil {
			return uuid.Nil, fmt.Errorf("get dialog: %w", err)
		}
		if d.ParentID == nil {
			return current, nil
		}
		current = *d.ParentID
	}
	slog.Warn("dialog lineage deeper than the ancestor cap, treating this dialog as the plan root",
		"dialog_id", dialogID, "depth", maxDialogAncestorDepth)
	return current, nil
}

func buildActionListResponse(
	plan storedActionPlan,
	checked map[string]struct{},
	execRuns ActionExecRuns,
	notes map[string]string,
) actionListResponse {
	resp := actionListResponse{
		Stages:   make([]actionListStage, 0, len(plan.Stages)),
		Rollback: make([]actionListAction, 0, len(plan.Rollback)),
	}

	for stageIdx, stage := range plan.Stages {
		stageResp := actionListStage{
			// The stage number is derived here too, so a legacy plan carrying a
			// stale stored value cannot desync from its own item numbers.
			Number:  actionPlanStageNumber(stageIdx),
			Title:   stage.Title,
			Actions: make([]actionListAction, 0, len(stage.Steps)),
			Checks:  make([]actionListCheck, 0, len(stage.Checks)),
		}
		for stepIdx, step := range stage.Steps {
			key := actionPlanItemKey(stageIdx, ActionPlanScopeSteps, stepIdx)
			_, executed := checked[key]
			stageResp.Actions = append(stageResp.Actions, actionListAction{
				Number:   actionPlanItemNumber(stageIdx, ActionPlanScopeSteps, stepIdx),
				Type:     step.Type,
				Action:   step.Action,
				Command:  step.Command,
				PRTitle:  step.PRTitle,
				PRURL:    step.PRURL,
				Executed: executed,
				Run:      actionListRunFor(execRuns, key),
				Notes:    notes[key],
			})
		}
		for checkIdx, check := range stage.Checks {
			key := actionPlanItemKey(stageIdx, ActionPlanScopeChecks, checkIdx)
			_, executed := checked[key]
			stageResp.Checks = append(stageResp.Checks, actionListCheck{
				Number:      actionPlanItemNumber(stageIdx, ActionPlanScopeChecks, checkIdx),
				Check:       check.Check,
				Expectation: check.Expectation,
				Executed:    executed,
			})
		}
		resp.Stages = append(resp.Stages, stageResp)
	}
	for idx, step := range plan.Rollback {
		key := fmt.Sprintf("rollback.%d", idx)
		_, executed := checked[key]
		resp.Rollback = append(resp.Rollback, actionListAction{
			Number:   actionPlanRollbackNumber(idx),
			Type:     step.Type,
			Action:   step.Action,
			Command:  step.Command,
			PRTitle:  step.PRTitle,
			PRURL:    step.PRURL,
			Executed: executed,
			Run:      actionListRunFor(execRuns, key),
			Notes:    notes[key],
		})
	}
	return resp
}
