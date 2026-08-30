package service

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
	"github.com/google/uuid"
)

const UpdateActionPlanToolName = "update_action_plan"

const updateActionPlanSchemaFile = "schemas/update_action_plan.json"

// defaultUpdateActionPlanParameters is used when the schema file is missing.
// The stage body repeats the stage object of create_action_plan without `number`
// and `title`: the position comes from the DAG and the title from `stage`.
var defaultUpdateActionPlanParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "stage": {
      "type": "string",
      "description": "Name of the stage to write. Must match a stage name in the DAG built during decomposition."
    },
    "content": {
      "type": "object",
      "properties": {
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
      "required": ["description", "steps", "checks"]
    }
  },
  "required": ["stage", "content"]
}`)

var updateActionPlanParamsCache sync.Map

// UpdateActionPlanToolDef returns the LLM tool definition for writing one stage
// of the action plan.
func UpdateActionPlanToolDef(allowToolsDir string) llm.ToolDef {
	return llm.ToolDef{
		Name: UpdateActionPlanToolName,
		Description: "Save a single stage of the action plan for the current conversation, leaving the other stages and the rollback untouched. " +
			"The stage name must be one of the stages of the DAG built during decomposition; an existing stage of that name is replaced. " +
			"The stage appears in the web interface as soon as the call returns, so call it once per stage while planning instead of waiting for the whole plan.",
		Parameters: loadUpdateActionPlanParameters(allowToolsDir),
	}
}

func loadUpdateActionPlanParameters(allowToolsDir string) json.RawMessage {
	if cached, ok := updateActionPlanParamsCache.Load(allowToolsDir); ok {
		return cached.(json.RawMessage)
	}

	path := filepath.Join(allowToolsDir, updateActionPlanSchemaFile)
	b, err := os.ReadFile(path)
	params := defaultUpdateActionPlanParameters
	if err == nil && len(b) > 0 {
		params = json.RawMessage(b)
	}

	updateActionPlanParamsCache.Store(allowToolsDir, params)
	return params
}

// updateActionPlanHandler returns a local tool handler bound to a specific dialog.
func (s *ChatService) updateActionPlanHandler(dialogID uuid.UUID) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		name := strings.TrimSpace(argString(args["stage"]))
		if name == "" {
			return "stage is required", nil
		}
		content, ok := args["content"].(map[string]any)
		if !ok {
			return "content must be a JSON object holding the stage description, steps and checks", nil
		}

		d, err := s.dialogRepo.GetDialog(ctx, dialogID)
		if err != nil {
			return "", fmt.Errorf("get dialog: %w", err)
		}
		// The DAG belongs to the decompose parent and names the only stages a
		// plan may hold, so there is nothing to write against anywhere else.
		if d.Mode != "plan" || d.ParentID == nil {
			return UpdateActionPlanToolName + " is only available in plan mode", nil
		}

		dag, found, err := s.ReadDAG(*d.ParentID)
		if err != nil {
			return "", fmt.Errorf("read dag: %w", err)
		}
		titles := dagStageTitles(dag)
		if !found || len(titles) == 0 {
			return "this plan has no DAG stages to write into, so call " + CreateActionPlanToolName + " with the whole plan instead", nil
		}

		title, ok := matchDAGStage(titles, name)
		if !ok {
			return fmt.Sprintf("stage %q is not in the DAG. Use one of: %s", name, strings.Join(titles, ", ")), nil
		}

		stageIdx, total, err := s.writeActionPlanStage(dialogID, titles, title, content)
		if err != nil {
			return "", err
		}

		// The workplace view refreshes the plan on this event, so the stage is
		// on screen before the turn that produced it ends.
		s.activity.Publish(dialogID, domain.AgentActivity{Kind: domain.ActivityActionPlanUpdated})

		relPath := filepath.Join("action_plans", dialogID.String()+".json")
		return fmt.Sprintf("Stage %q saved to %s as stage %d of %d", title, relPath, stageIdx+1, total), nil
	}
}

// writeActionPlanStage stores one stage in the plan file and returns the index
// it landed on together with the new stage count. Checkbox, comment and run keys
// are positional, so they are remapped in the same pass.
func (s *ChatService) writeActionPlanStage(
	dialogID uuid.UUID,
	titles []string,
	title string,
	content map[string]any,
) (int, int, error) {
	plan, err := s.readActionPlanDocument(dialogID)
	if err != nil {
		return 0, 0, err
	}

	stages, _ := plan["stages"].([]any)
	stages, index, replaced := upsertActionPlanStage(stages, titles, title, content)
	plan["stages"] = stages
	if _, ok := plan["rollback"]; !ok {
		plan["rollback"] = []any{}
	}

	planData, err := json.Marshal(plan)
	if err != nil {
		return 0, 0, fmt.Errorf("marshal action plan: %w", err)
	}
	if err := s.WriteActionPlan(dialogID, planData); err != nil {
		return 0, 0, err
	}

	remap := insertedStageRemap(index)
	if replaced {
		remap = replacedStageRemap(index)
	}
	if err := s.remapActionPlanItemKeys(dialogID, remap); err != nil {
		return 0, 0, err
	}

	return index, len(stages), nil
}

// readActionPlanDocument returns the stored plan as a generic document, or an
// empty one when the dialog has no plan yet. Keys this package does not model
// are preserved, the way the HTTP edit path does.
func (s *ChatService) readActionPlanDocument(dialogID uuid.UUID) (map[string]any, error) {
	raw, found, err := s.ReadActionPlan(dialogID)
	if err != nil {
		return nil, fmt.Errorf("read action plan: %w", err)
	}
	if !found {
		return map[string]any{"stages": []any{}, "rollback": []any{}}, nil
	}

	var plan map[string]any
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil, fmt.Errorf("unmarshal action plan: %w", err)
	}
	if plan == nil {
		plan = map[string]any{}
	}
	return plan, nil
}

// upsertActionPlanStage replaces the stage named title, or inserts it at the
// position the DAG gives it, and renumbers every stage. It reports the index the
// stage landed on and whether an existing stage was replaced.
func upsertActionPlanStage(
	stages []any,
	titles []string,
	title string,
	content map[string]any,
) ([]any, int, bool) {
	stage := make(map[string]any, len(content)+2)
	maps.Copy(stage, content)
	// `number` is derived from the stage order and `title` from the DAG label,
	// so a model that sends either is overruled rather than trusted.
	stage["title"] = title

	index, replaced := actionPlanStagePosition(stages, titles, title)
	if replaced {
		stages[index] = stage
	} else {
		stages = append(stages, nil)
		copy(stages[index+1:], stages[index:])
		stages[index] = stage
	}

	for i, raw := range stages {
		obj, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		obj["number"] = i + 1
	}
	return stages, index, replaced
}

// actionPlanStagePosition locates title among stages: the index of the stage it
// replaces, or the index it should be inserted at to keep the plan in DAG order.
func actionPlanStagePosition(stages []any, titles []string, title string) (int, bool) {
	order := make(map[string]int, len(titles))
	for i, t := range titles {
		order[normalizeStageTitle(t)] = i
	}

	want := order[normalizeStageTitle(title)]
	insert := len(stages)
	for i, raw := range stages {
		obj, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		key := normalizeStageTitle(argString(obj["title"]))
		if key == normalizeStageTitle(title) {
			return i, true
		}
		// Stages the current DAG no longer names have no position to compare
		// against, so they are stepped over rather than pushed around.
		pos, known := order[key]
		if known && pos > want && insert == len(stages) {
			insert = i
		}
	}
	return insert, false
}

// replacedStageRemap drops the keys of a stage whose steps and checks were just
// replaced: their positions no longer describe the same work.
func replacedStageRemap(index int) func(int) int {
	return func(stage int) int {
		if stage == index {
			return -1
		}
		return stage
	}
}

// insertedStageRemap shifts the keys of every stage that a newly inserted stage
// pushed down.
func insertedStageRemap(index int) func(int) int {
	return func(stage int) int {
		if stage >= index {
			return stage + 1
		}
		return stage
	}
}

// remapActionPlanItemKeys rewrites the persisted checkbox, comment and run keys
// of a dialog after stages moved. Stores that hold nothing are left untouched so
// a plan being drafted does not grow empty side files.
func (s *ChatService) remapActionPlanItemKeys(dialogID uuid.UUID, remap func(int) int) error {
	checked, err := s.ReadActionPlanChecks(dialogID)
	if err != nil {
		return err
	}
	if len(checked) > 0 {
		if err := s.WriteActionPlanChecks(dialogID, rekeyActionPlanList(checked, remap)); err != nil {
			return err
		}
	}

	comments, err := s.ReadActionPlanComments(dialogID)
	if err != nil {
		return err
	}
	if len(comments) > 0 {
		if err := s.WriteActionPlanComments(dialogID, rekeyActionPlanMap(comments, remap)); err != nil {
			return err
		}
	}

	runs, err := s.ReadActionPlanRuns(dialogID)
	if err != nil {
		return err
	}
	if len(runs) > 0 {
		if err := s.WriteActionPlanRuns(dialogID, rekeyActionPlanMap(runs, remap)); err != nil {
			return err
		}
	}
	return nil
}

// rekeyActionPlanItem rewrites one positional item key. remap returns the new
// stage index for an old one, or a negative value to drop the key. Keys that do
// not address a stage item are passed through untouched.
func rekeyActionPlanItem(key string, remap func(int) int) (string, bool) {
	m := actionPlanKeyPattern.FindStringSubmatch(key)
	if m == nil {
		return key, true
	}
	next := remap(atoi(m[1]))
	if next < 0 {
		return "", false
	}
	return fmt.Sprintf("s%d.%s%s", next, m[2], m[3]), true
}

func rekeyActionPlanList(keys []string, remap func(int) int) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if next, keep := rekeyActionPlanItem(key, remap); keep {
			out = append(out, next)
		}
	}
	return out
}

func rekeyActionPlanMap(values map[string]string, remap func(int) int) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		if next, keep := rekeyActionPlanItem(key, remap); keep {
			out[next] = value
		}
	}
	return out
}
