package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/llm"
	"github.com/google/uuid"
)

const CreateActionPlanToolName = "create_action_plan"

const createActionPlanSchemaFile = "schemas/create_action_plan.json"

// maxActionPlanReminders caps how many extra rounds a plan turn may spend being
// told to finish its contract, so a model that keeps answering in prose cannot
// spin through the whole iteration budget.
const maxActionPlanReminders = 2

// actionPlanReminder is replayed to a plan dialog whose turn is about to end
// without a stored plan. Reasoning models read the prompt's "think before you
// call tools" rule literally and deliver the intended tool calls as prose,
// which otherwise ends the turn with nothing to render.
const actionPlanReminder = "Your last message contained no tool calls, which ends the turn, and this dialog still has no action plan. " +
	"Describing calls you intend to make has no effect: only an actual tool call runs. " +
	"Issue the calls you need now, and store the plan itself: update_action_plan for each stage you have finished, or create_action_plan with the full plan object."

// stageActionPlanReminder is the fan-out variant. A stage subagent shares one
// plan file with its siblings, so "a plan exists" says nothing about whether
// this stage was written.
const stageActionPlanReminder = "Your last message contained no tool calls, which ends the turn, and you have not stored your stage yet. " +
	"Describing calls you intend to make has no effect: only an actual tool call runs. " +
	"Call update_action_plan now with stage %q and the steps and checks you worked out. " +
	"If something is still unclear, call report_blocker, then store the stage anyway under a stated assumption."

func actionPlanReminderFor(stage string) string {
	if stage == "" {
		return actionPlanReminder
	}
	return fmt.Sprintf(stageActionPlanReminder, stage)
}

// needsActionPlanReminder reports whether a tool-free reply is ending a plan turn
// before the work it owed was persisted. Later turns of a dialog that already has
// a plan are free to answer in prose.
func (s *ChatService) needsActionPlanReminder(cfg loopConfig, modeName string, catalog *toolCatalog, planUpdated bool) bool {
	if planUpdated || modeName != "plan" {
		return false
	}
	_, canUpdate := catalog.localHandlers[UpdateActionPlanToolName]
	_, canCreate := catalog.localHandlers[CreateActionPlanToolName]
	if !canUpdate && !canCreate {
		return false
	}

	// A stage subagent owes exactly one stage, and its siblings write the same
	// file, so the file existing proves nothing about this stage.
	if cfg.stage != "" {
		return true
	}

	_, exists, err := s.ReadActionPlan(cfg.planID)
	if err != nil {
		slog.Warn("action plan reminder: read stored plan", "dialog_id", cfg.planID, "error", err)
		return false
	}
	return !exists
}

// ActionPlanStatus is the lifecycle state of an action plan.
type ActionPlanStatus string

const (
	ActionPlanStatusDraft      ActionPlanStatus = "draft"
	ActionPlanStatusScheduled  ActionPlanStatus = "scheduled"
	ActionPlanStatusInProgress ActionPlanStatus = "in_progress"
	// ActionPlanStatusDone marks a finished plan: every stage step and check is executed.
	ActionPlanStatusDone ActionPlanStatus = "done"
	// ActionPlanStatusReopened marks a finished plan whose item was later unchecked.
	ActionPlanStatusReopened   ActionPlanStatus = "reopened"
	ActionPlanStatusRolledBack ActionPlanStatus = "rolled_back"
)

func isValidActionPlanStatus(status ActionPlanStatus) bool {
	switch status {
	case ActionPlanStatusDraft,
		ActionPlanStatusScheduled,
		ActionPlanStatusInProgress,
		ActionPlanStatusDone,
		ActionPlanStatusReopened,
		ActionPlanStatusRolledBack:
		return true
	default:
		return false
	}
}

// defaultCreateActionPlanParameters is used when the schema file is missing.
var defaultCreateActionPlanParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "plan": {
      "type": "object",
      "properties": {
        "stages": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "number": { "type": "integer" },
              "title": { "type": "string" },
              "description": { "type": "string" },
              "steps": {
                "type": "array",
                "items": {
                  "type": "object",
                  "properties": {
                    "type": { "type": "string" },
                    "action": { "type": "string" },
                    "command": { "type": "string" },
                    "repository": { "type": "string" },
                    "pr_title": { "type": "string" },
                    "comment": { "type": "string" }
                  },
                  "required": ["type", "action"]
                }
              },
              "checks": {
                "type": "array",
                "items": {
                  "type": "object",
                  "properties": {
                    "check": { "type": "string" },
                    "expectation": { "type": "string" }
                  },
                  "required": ["check", "expectation"]
                }
              }
            },
            "required": ["number", "title", "description", "steps", "checks"]
          }
        },
        "rollback": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "type": { "type": "string" },
              "action": { "type": "string" },
              "command": { "type": "string" },
              "repository": { "type": "string" },
              "pr_title": { "type": "string" },
              "comment": { "type": "string" }
            },
            "required": ["type", "action"]
          }
        }
      },
      "required": ["stages", "rollback"]
    }
  },
  "required": ["plan"]
}`)

// CreateActionPlanToolDef returns the LLM tool definition for persisting an action plan.
func CreateActionPlanToolDef(allowToolsDir string) llm.ToolDef {
	return llm.ToolDef{
		Name:        CreateActionPlanToolName,
		Description: "Save a detailed action plan for the current conversation. The plan is stored for rendering in the web interface.",
		Parameters:  loadCreateActionPlanParameters(allowToolsDir),
	}
}

func loadCreateActionPlanParameters(allowToolsDir string) json.RawMessage {
	if cached, ok := createActionPlanParamsCache.Load(allowToolsDir); ok {
		return cached.(json.RawMessage)
	}

	path := filepath.Join(allowToolsDir, createActionPlanSchemaFile)
	b, err := os.ReadFile(path)
	params := defaultCreateActionPlanParameters
	if err == nil && len(b) > 0 {
		params = json.RawMessage(b)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		params = defaultCreateActionPlanParameters
	}

	createActionPlanParamsCache.Store(allowToolsDir, params)
	return params
}

var createActionPlanParamsCache sync.Map

// createActionPlanHandler returns a local tool handler bound to a specific dialog.
func (s *ChatService) createActionPlanHandler(dialogID uuid.UUID) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		raw, ok := args["plan"]
		if !ok || raw == nil {
			return "plan is required", nil
		}

		data, err := json.Marshal(raw)
		if err != nil {
			return "plan must be a valid JSON object", nil
		}

		relPath := filepath.Join("action_plans", dialogID.String()+".json")

		if _, err := s.WriteActionPlan(dialogID, data); err != nil {
			return "", err
		}

		checksPath := actionPlanChecksPath(s.actionPlansDir, dialogID)
		_ = os.Remove(checksPath)

		commentsPath := actionPlanCommentsPath(s.actionPlansDir, dialogID)
		_ = os.Remove(commentsPath)

		runsPath := actionPlanRunsPath(s.actionPlansDir, dialogID)
		_ = os.Remove(runsPath)

		return "Action plan saved to " + relPath, nil
	}
}

func writeActionPlanFile(path string, content []byte) error {
	return atomicfile.Write(path, content)
}

func actionPlanChecksPath(dir string, dialogID uuid.UUID) string {
	return filepath.Join(dir, dialogID.String()+".checks.json")
}

func actionPlanCommentsPath(dir string, dialogID uuid.UUID) string {
	return filepath.Join(dir, dialogID.String()+".comments.json")
}

func actionPlanRunsPath(dir string, dialogID uuid.UUID) string {
	return filepath.Join(dir, dialogID.String()+".runs.json")
}

func actionPlanExecutorPath(dir string, dialogID uuid.UUID) string {
	return filepath.Join(dir, dialogID.String()+".executor.json")
}

// ReadActionPlan returns the stored action plan JSON for a dialog, or false if none exists.
func (s *ChatService) ReadActionPlan(dialogID uuid.UUID) (json.RawMessage, bool, error) {
	path := filepath.Join(s.actionPlansDir, dialogID.String()+".json")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return json.RawMessage(b), true, nil
}

// WriteActionPlan overwrites the stored action plan JSON for a dialog and
// returns the bytes it wrote. Every write goes through here, so stamping the
// derived item numbers on the way past makes "numbers follow position" an
// invariant of the file rather than a rule each caller has to remember. The
// normalized bytes are returned because callers that echo the plan back to the
// client must send the numbered document, not their input.
func (s *ChatService) WriteActionPlan(dialogID uuid.UUID, plan json.RawMessage) (json.RawMessage, error) {
	if err := os.MkdirAll(s.actionPlansDir, 0o755); err != nil {
		return nil, fmt.Errorf("create action_plans directory: %w", err)
	}

	// A document that is not a JSON object cannot be numbered, but refusing it
	// here would turn a malformed operator PUT into a write failure, so it is
	// stored as it came in.
	if numbered, err := renumberActionPlanJSON(plan); err == nil {
		plan = numbered
	}

	path := filepath.Join(s.actionPlansDir, dialogID.String()+".json")
	if err := writeActionPlanFile(path, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func renumberActionPlanJSON(plan json.RawMessage) (json.RawMessage, error) {
	var doc map[string]any
	if err := json.Unmarshal(plan, &doc); err != nil {
		return nil, err
	}
	renumberActionPlan(doc)
	return json.Marshal(doc)
}

// ReadActionPlanChecks returns persisted checkbox keys for a dialog.
func (s *ChatService) ReadActionPlanChecks(dialogID uuid.UUID) ([]string, error) {
	path := actionPlanChecksPath(s.actionPlansDir, dialogID)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}

	var checked []string
	if err := json.Unmarshal(b, &checked); err != nil {
		return nil, fmt.Errorf("unmarshal action plan checks: %w", err)
	}
	if checked == nil {
		return []string{}, nil
	}
	return checked, nil
}

// WriteActionPlanChecks persists checkbox keys for a dialog.
func (s *ChatService) WriteActionPlanChecks(dialogID uuid.UUID, checked []string) error {
	if checked == nil {
		checked = []string{}
	}

	if err := os.MkdirAll(s.actionPlansDir, 0o755); err != nil {
		return fmt.Errorf("create action_plans directory: %w", err)
	}

	data, err := json.Marshal(checked)
	if err != nil {
		return fmt.Errorf("marshal action plan checks: %w", err)
	}

	path := actionPlanChecksPath(s.actionPlansDir, dialogID)
	return writeActionPlanFile(path, data)
}

// ReadActionPlanComments returns persisted user comments keyed by action row key.
func (s *ChatService) ReadActionPlanComments(dialogID uuid.UUID) (map[string]string, error) {
	path := actionPlanCommentsPath(s.actionPlansDir, dialogID)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}

	var comments map[string]string
	if err := json.Unmarshal(b, &comments); err != nil {
		return nil, fmt.Errorf("unmarshal action plan comments: %w", err)
	}
	if comments == nil {
		return map[string]string{}, nil
	}
	return comments, nil
}

// WriteActionPlanComments persists user comments keyed by action row key.
func (s *ChatService) WriteActionPlanComments(dialogID uuid.UUID, comments map[string]string) error {
	if comments == nil {
		comments = map[string]string{}
	}

	if err := os.MkdirAll(s.actionPlansDir, 0o755); err != nil {
		return fmt.Errorf("create action_plans directory: %w", err)
	}

	data, err := json.Marshal(comments)
	if err != nil {
		return fmt.Errorf("marshal action plan comments: %w", err)
	}

	path := actionPlanCommentsPath(s.actionPlansDir, dialogID)
	return writeActionPlanFile(path, data)
}

// ReadActionPlanRuns returns persisted execute-dialog ids keyed by action row key.
func (s *ChatService) ReadActionPlanRuns(dialogID uuid.UUID) (map[string]string, error) {
	path := actionPlanRunsPath(s.actionPlansDir, dialogID)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}

	var runs map[string]string
	if err := json.Unmarshal(b, &runs); err != nil {
		return nil, fmt.Errorf("unmarshal action plan runs: %w", err)
	}
	if runs == nil {
		return map[string]string{}, nil
	}
	return runs, nil
}

// WriteActionPlanRuns persists execute-dialog ids keyed by action row key.
func (s *ChatService) WriteActionPlanRuns(dialogID uuid.UUID, runs map[string]string) error {
	if runs == nil {
		runs = map[string]string{}
	}

	if err := os.MkdirAll(s.actionPlansDir, 0o755); err != nil {
		return fmt.Errorf("create action_plans directory: %w", err)
	}

	data, err := json.Marshal(runs)
	if err != nil {
		return fmt.Errorf("marshal action plan runs: %w", err)
	}

	path := actionPlanRunsPath(s.actionPlansDir, dialogID)
	return writeActionPlanFile(path, data)
}

// ReadActionPlanExecutorDialog returns the persisted shared execute chat id for
// a plan dialog, or false when none has been created yet.
func (s *ChatService) ReadActionPlanExecutorDialog(dialogID uuid.UUID) (uuid.UUID, bool, error) {
	path := actionPlanExecutorPath(s.actionPlansDir, dialogID)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}

	var stored struct {
		DialogID string `json:"dialogId"`
	}
	if err := json.Unmarshal(b, &stored); err != nil {
		return uuid.Nil, false, fmt.Errorf("unmarshal action plan executor: %w", err)
	}

	execID, err := uuid.Parse(strings.TrimSpace(stored.DialogID))
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("parse action plan executor id: %w", err)
	}
	return execID, true, nil
}

// WriteActionPlanExecutorDialog persists the shared execute chat id for a plan.
func (s *ChatService) WriteActionPlanExecutorDialog(dialogID, execDialogID uuid.UUID) error {
	if err := os.MkdirAll(s.actionPlansDir, 0o755); err != nil {
		return fmt.Errorf("create action_plans directory: %w", err)
	}

	data, err := json.Marshal(map[string]string{
		"dialogId": execDialogID.String(),
	})
	if err != nil {
		return fmt.Errorf("marshal action plan executor: %w", err)
	}

	path := actionPlanExecutorPath(s.actionPlansDir, dialogID)
	return writeActionPlanFile(path, data)
}
