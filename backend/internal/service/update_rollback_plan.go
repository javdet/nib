package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
)

const UpdateRollbackPlanToolName = "update_rollback_plan"

const updateRollbackPlanSchemaFile = "schemas/update_rollback_plan.json"

// defaultUpdateRollbackPlanParameters is used when the schema file is missing.
// The entry shape repeats the `rollback` items of create_action_plan: the same
// list, written on its own rather than as part of a whole plan.
var defaultUpdateRollbackPlanParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
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
          "comment": { "type": "string" },
          "categories": { "type": "array", "items": { "type": "string" } }
        },
        "required": ["type", "action", "categories"]
      }
    }
  },
  "required": ["rollback"]
}`)

var updateRollbackPlanParamsCache sync.Map

// UpdateRollbackPlanToolDef returns the LLM tool definition for writing the
// rollback list of the action plan.
func UpdateRollbackPlanToolDef(allowToolsDir string, categoryNames []string) llm.ToolDef {
	return llm.ToolDef{
		Name: UpdateRollbackPlanToolName,
		Description: "Save the rollback list of the action plan for the current conversation, leaving every stage untouched. " +
			"The rollback is one flat list for the whole plan, so the call replaces it as a whole and must carry every entry. " +
			"This is how a plan assembled stage by stage gets its rollback, and how the rollback of an existing plan is redone " +
			"without discarding the operator's checkboxes, comments and action runs.",
		Parameters: withStepCategories(loadUpdateRollbackPlanParameters(allowToolsDir), categoryNames),
	}
}

func loadUpdateRollbackPlanParameters(allowToolsDir string) json.RawMessage {
	if cached, ok := updateRollbackPlanParamsCache.Load(allowToolsDir); ok {
		return cached.(json.RawMessage)
	}

	path := filepath.Join(allowToolsDir, updateRollbackPlanSchemaFile)
	b, err := os.ReadFile(path)
	params := defaultUpdateRollbackPlanParameters
	if err == nil && len(b) > 0 {
		params = json.RawMessage(b)
	}

	updateRollbackPlanParamsCache.Store(allowToolsDir, params)
	return params
}

// updateRollbackPlanHandler returns a local tool handler that writes the
// rollback list of the plan owned by planID, leaving its stages alone.
func (s *ChatService) updateRollbackPlanHandler(planID uuid.UUID) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		raw, ok := args["rollback"]
		if !ok || raw == nil {
			return "rollback is required: pass the whole list, since the call replaces it", nil
		}
		entries, ok := raw.([]any)
		if !ok {
			return "rollback must be a JSON array of entries, each with a type and an action", nil
		}
		for i, entry := range entries {
			if _, ok := entry.(map[string]any); !ok {
				return fmt.Sprintf("rollback entry %d must be a JSON object with a type and an action", i+1), nil
			}
		}

		// The categories decide the MCP tools of the sub-agent that will execute
		// each entry, so a name the catalog does not know is dropped here rather
		// than stored and quietly ignored at execution time.
		valid := s.cachedToolCategoryNames(ctx)
		unknown := normalizeActionPlanCategories(entries, valid)

		count, changed, err := s.writeActionPlanRollback(planID, entries)
		if err != nil {
			return "", err
		}
		if count < 0 {
			return "this plan has no stages yet, so there is nothing to roll back. " +
				"Wait for the stages to be written, or store them first with " + CreateActionPlanToolName, nil
		}

		// The workplace view refreshes the plan on this event, so the rollback is
		// on screen before the turn that produced it ends.
		s.activity.Publish(planID, domain.AgentActivity{Kind: domain.ActivityActionPlanUpdated})

		relPath := filepath.Join("action_plans", planID.String()+".json")
		if !changed {
			return fmt.Sprintf("Rollback unchanged: the %d entries you sent match the stored list, so %s and the operator's checkboxes are left as they are",
				count, relPath) + unknownCategoryNote(unknown, valid), nil
		}
		return fmt.Sprintf("Rollback saved to %s as %d entries, numbered R1 to R%d", relPath, count, count) +
			unknownCategoryNote(unknown, valid), nil
	}
}

// writeActionPlanRollback replaces the plan's rollback list and reports how many
// entries it stored and whether they differ from the ones already there. It
// returns a negative count, and writes nothing, for a plan that has no stages:
// the document is created on demand, so writing a rollback into a plan nobody
// has written would render an undo for work that was never planned.
func (s *ChatService) writeActionPlanRollback(dialogID uuid.UUID, entries []any) (int, bool, error) {
	mu := s.planMutex(dialogID)
	mu.Lock()
	defer mu.Unlock()

	plan, err := s.readActionPlanDocument(dialogID)
	if err != nil {
		return 0, false, err
	}
	if stages, _ := plan["stages"].([]any); len(stages) == 0 {
		return -1, false, nil
	}

	changed, err := rollbackDiffers(plan["rollback"], entries)
	if err != nil {
		return 0, false, err
	}
	plan["rollback"] = entries

	planData, err := json.Marshal(plan)
	if err != nil {
		return 0, false, fmt.Errorf("marshal action plan: %w", err)
	}
	if _, err := s.WriteActionPlan(dialogID, planData); err != nil {
		return 0, false, err
	}

	// The keys are positional, so once the list changes they no longer describe
	// the same work. An identical list keeps them, which is what lets the
	// rollback be re-derived on a later fan-out round for free.
	if changed {
		if err := s.dropActionPlanRollbackKeys(dialogID); err != nil {
			return 0, false, err
		}
	}
	return len(entries), changed, nil
}

// rollbackDiffers compares the stored rollback with the one being written. The
// derived `number` fields are stamped on the way out by WriteActionPlan, so they
// are ignored here rather than making every write look like a change.
func rollbackDiffers(stored any, next []any) (bool, error) {
	strip := func(list []any) ([]byte, error) {
		out := make([]any, 0, len(list))
		for _, raw := range list {
			entry, ok := raw.(map[string]any)
			if !ok {
				out = append(out, raw)
				continue
			}
			copied := make(map[string]any, len(entry))
			for k, v := range entry {
				if k == "number" {
					continue
				}
				copied[k] = v
			}
			out = append(out, copied)
		}
		return json.Marshal(out)
	}

	before, err := strip(asArray(stored))
	if err != nil {
		return false, fmt.Errorf("marshal stored rollback: %w", err)
	}
	after, err := strip(next)
	if err != nil {
		return false, fmt.Errorf("marshal rollback: %w", err)
	}
	return string(before) != string(after), nil
}

// actionPlanRollbackKeyPattern matches the persisted key of a rollback entry.
// Rollback is a plan-level list, so unlike a stage item its key carries no stage
// index -- which is why actionPlanKeyPattern must never be broadened to cover
// it: there is nothing to shift when a stage moves.
var actionPlanRollbackKeyPattern = regexp.MustCompile(`^rollback\.\d+$`)

// dropActionPlanRollbackKeys forgets the checkbox, comment, run, exec-status and
// executor-note keys of the rollback entries, leaving every stage key untouched. Stores that
// hold nothing are left alone so a plan being drafted does not grow empty side
// files.
func (s *ChatService) dropActionPlanRollbackKeys(dialogID uuid.UUID) error {
	keep := func(key string) bool { return !actionPlanRollbackKeyPattern.MatchString(key) }

	checked, err := s.ReadActionPlanChecks(dialogID)
	if err != nil {
		return err
	}
	if kept := filterActionPlanList(checked, keep); len(kept) != len(checked) {
		if err := s.WriteActionPlanChecks(dialogID, kept); err != nil {
			return err
		}
	}

	comments, err := s.ReadActionPlanComments(dialogID)
	if err != nil {
		return err
	}
	if kept := filterActionPlanMap(comments, keep); len(kept) != len(comments) {
		if err := s.WriteActionPlanComments(dialogID, kept); err != nil {
			return err
		}
	}

	runs, err := s.ReadActionPlanRuns(dialogID)
	if err != nil {
		return err
	}
	if kept := filterActionPlanMap(runs, keep); len(kept) != len(runs) {
		if err := s.WriteActionPlanRuns(dialogID, kept); err != nil {
			return err
		}
	}

	// The exec records go too, or a rewritten rollback inherits the run status of
	// whatever used to sit at that position -- an R1 the operator never ran
	// showing as done.
	execRuns, err := s.ReadActionPlanExecRuns(dialogID)
	if err != nil {
		return err
	}
	if kept := filterActionExecRuns(execRuns, keep); len(kept) != len(execRuns) {
		if err := s.WriteActionPlanExecRuns(dialogID, kept); err != nil {
			return err
		}
	}

	notes, err := s.readActionPlanNotes(dialogID)
	if err != nil {
		return err
	}
	if kept := filterActionPlanMap(notes, keep); len(kept) != len(notes) {
		if err := s.writeActionPlanNotes(dialogID, kept); err != nil {
			return err
		}
	}
	return nil
}

func filterActionExecRuns(runs ActionExecRuns, keep func(string) bool) ActionExecRuns {
	out := make(ActionExecRuns, len(runs))
	for key, run := range runs {
		if keep(key) {
			out[key] = run
		}
	}
	return out
}

func filterActionPlanList(keys []string, keep func(string) bool) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if keep(key) {
			out = append(out, key)
		}
	}
	return out
}

func filterActionPlanMap(values map[string]string, keep func(string) bool) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		if keep(key) {
			out[key] = value
		}
	}
	return out
}
