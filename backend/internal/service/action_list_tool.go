package service

import (
	"context"
	"encoding/json"
	"fmt"

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
			"Each action and check carries the `number` the operator sees next to it in the web interface.",
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
	Number   string `json:"number"`
	Type     string `json:"type"`
	Action   string `json:"action"`
	Command  string `json:"command,omitempty"`
	PRTitle  string `json:"pr_title,omitempty"`
	PRURL    string `json:"pr_url,omitempty"`
	Executed bool   `json:"executed"`
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
}

type storedActionCheck struct {
	Number      string `json:"number,omitempty"`
	Check       string `json:"check"`
	Expectation string `json:"expectation"`
}

func (s *ChatService) getActionListHandler(dialogID uuid.UUID) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		planID, err := s.resolveActionPlanDialogID(ctx, dialogID)
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

		resp := buildActionListResponse(plan, checkedSet)
		out, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshal action list: %w", err)
		}
		return string(out), nil
	}
}

func (s *ChatService) resolveActionPlanDialogID(ctx context.Context, dialogID uuid.UUID) (uuid.UUID, error) {
	if s.dialogRepo == nil {
		return dialogID, nil
	}
	d, err := s.dialogRepo.GetDialog(ctx, dialogID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("get dialog: %w", err)
	}
	if d.ParentID != nil {
		return *d.ParentID, nil
	}
	return dialogID, nil
}

func buildActionListResponse(plan storedActionPlan, checked map[string]struct{}) actionListResponse {
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
		})
	}
	return resp
}
