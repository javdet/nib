package service

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

func TestUpdateActionPlanToolDef(t *testing.T) {
	t.Parallel()
	def := UpdateActionPlanToolDef(t.TempDir())
	if def.Name != UpdateActionPlanToolName {
		t.Fatalf("name = %q, want %q", def.Name, UpdateActionPlanToolName)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(def.Parameters, &schema); err != nil {
		t.Fatalf("unmarshal parameters: %v", err)
	}
	if len(schema.Required) != 2 || schema.Required[0] != "stage" || schema.Required[1] != "content" {
		t.Fatalf("required = %#v, want [stage content]", schema.Required)
	}
}

// TestUpdateActionPlanSchema_stepPropertiesMatchCreate guards the three copies of
// the step schema: the two shipped in seed/tools and the fallback used when the
// update file is missing. A stage written through update_action_plan has to
// render the same as one written through create_action_plan.
func TestUpdateActionPlanSchema_stepPropertiesMatchCreate(t *testing.T) {
	t.Parallel()

	seedDir := systemToolsTestDataDir(t)
	fileParams := loadUpdateActionPlanParameters(seedDir)
	if string(fileParams) == string(defaultUpdateActionPlanParameters) {
		t.Fatal("expected the schema file to be loaded, got the fallback")
	}

	want := actionPlanStepProperties(t, loadCreateActionPlanParameters(seedDir), "steps")
	for name, schema := range map[string]json.RawMessage{
		"file":     fileParams,
		"fallback": defaultUpdateActionPlanParameters,
	} {
		got := updateActionPlanStepProperties(t, schema)
		if !maps.Equal(got, want) {
			t.Fatalf("%s step properties = %v, want %v", name, got, want)
		}
	}
}

// updateActionPlanStepProperties returns the property names declared for a step
// of the update_action_plan schema.
func updateActionPlanStepProperties(t *testing.T, schema json.RawMessage) map[string]struct{} {
	t.Helper()

	var doc struct {
		Properties struct {
			Content struct {
				Properties struct {
					Steps struct {
						Items map[string]any `json:"items"`
					} `json:"steps"`
				} `json:"properties"`
			} `json:"content"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &doc); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}

	props, ok := doc.Properties.Content.Properties.Steps.Items["properties"].(map[string]any)
	if !ok {
		t.Fatalf("step schema has no properties object: %#v", doc.Properties.Content.Properties.Steps.Items)
	}

	names := make(map[string]struct{}, len(props))
	for name := range props {
		names[name] = struct{}{}
	}
	return names
}

const updateActionPlanTestDAG = "```mermaid\nflowchart TD\n" +
	"  jmx[\"Configure Cassandra JMX\"] --> reaper[\"Setup and connect Cassandra Reaper\"]\n" +
	"  reaper --> verify[\"Verify repairs\"]\n```"

// updateActionPlanFixture returns the decompose dialog that owns both the
// three-stage DAG above and the action plan written against it.
func updateActionPlanFixture(t *testing.T) (*ChatService, uuid.UUID) {
	t.Helper()

	dagsDir := t.TempDir()
	planID := uuid.New()

	if err := os.WriteFile(filepath.Join(dagsDir, planID.String()+".md"), []byte(updateActionPlanTestDAG), 0o644); err != nil {
		t.Fatalf("write dag: %v", err)
	}

	svc := &ChatService{
		dagsDir:        dagsDir,
		actionPlansDir: t.TempDir(),
		activity:       NewActivityBroker(),
		dialogRepo: &actionListDialogRepo{dialogs: map[uuid.UUID]domain.Dialog{
			planID: {ID: planID, Mode: "decompose"},
		}},
	}
	return svc, planID
}

func stageContent(action string) map[string]any {
	return map[string]any{
		"description": "Stage description",
		"steps": []any{
			map[string]any{"type": "shell", "action": action, "command": "true"},
		},
		"checks": []any{
			map[string]any{"check": "Service is up", "expectation": "systemctl reports active"},
		},
	}
}

// readStoredStages returns the stage list of the plan written for dialogID.
func readStoredStages(t *testing.T, svc *ChatService, dialogID uuid.UUID) []map[string]any {
	t.Helper()

	raw, found, err := svc.ReadActionPlan(dialogID)
	if err != nil {
		t.Fatalf("read action plan: %v", err)
	}
	if !found {
		t.Fatal("expected a stored action plan")
	}

	var plan struct {
		Stages   []map[string]any `json:"stages"`
		Rollback []any            `json:"rollback"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatalf("unmarshal action plan: %v", err)
	}
	if plan.Rollback == nil {
		t.Fatal("expected a rollback array in the stored plan")
	}
	return plan.Stages
}

func stageTitles(stages []map[string]any) []string {
	titles := make([]string, 0, len(stages))
	for _, stage := range stages {
		titles = append(titles, argString(stage["title"]))
	}
	return titles
}

func TestUpdateActionPlanHandler_createsPlanFromFirstStage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, planID := updateActionPlanFixture(t)

	events, unsubscribe := svc.SubscribeActivity(planID)
	defer unsubscribe()

	out, err := svc.updateActionPlanHandler(planID, "")(ctx, map[string]any{
		// Lower case and loose spacing: the DAG spelling is what gets stored.
		"stage":   "configure  cassandra jmx",
		"content": stageContent("Enable the JMX exporter"),
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, `Stage "Configure Cassandra JMX" saved`) || !strings.Contains(out, "stage 1 of 1") {
		t.Fatalf("out = %q", out)
	}

	stages := readStoredStages(t, svc, planID)
	if len(stages) != 1 {
		t.Fatalf("stages = %#v, want one", stages)
	}
	if got := argString(stages[0]["title"]); got != "Configure Cassandra JMX" {
		t.Fatalf("title = %q", got)
	}
	if got, ok := stages[0]["number"].(float64); !ok || got != 1 {
		t.Fatalf("number = %#v, want 1", stages[0]["number"])
	}
	if got := argString(stages[0]["description"]); got != "Stage description" {
		t.Fatalf("description = %q", got)
	}

	select {
	case ev := <-events:
		if ev.Kind != domain.ActivityActionPlanUpdated {
			t.Fatalf("event kind = %q, want %q", ev.Kind, domain.ActivityActionPlanUpdated)
		}
	default:
		t.Fatal("expected an action_plan_updated event")
	}
}

func TestUpdateActionPlanHandler_insertsStagesInDAGOrder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, planID := updateActionPlanFixture(t)
	handler := svc.updateActionPlanHandler(planID, "")

	// Written last-to-first: the DAG, not the call order, fixes the position.
	for _, name := range []string{"Verify repairs", "Configure Cassandra JMX", "Setup and connect Cassandra Reaper"} {
		if _, err := handler(ctx, map[string]any{"stage": name, "content": stageContent("do " + name)}); err != nil {
			t.Fatalf("%s: err = %v", name, err)
		}
	}

	stages := readStoredStages(t, svc, planID)
	want := []string{"Configure Cassandra JMX", "Setup and connect Cassandra Reaper", "Verify repairs"}
	if got := stageTitles(stages); !slicesEqual(got, want) {
		t.Fatalf("titles = %#v, want %#v", got, want)
	}
	for i, stage := range stages {
		if got, ok := stage["number"].(float64); !ok || int(got) != i+1 {
			t.Fatalf("stage %d number = %#v", i, stage["number"])
		}
	}
}

func TestUpdateActionPlanHandler_replacesStageAndKeepsOtherStages(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, planID := updateActionPlanFixture(t)
	handler := svc.updateActionPlanHandler(planID, "")

	for _, name := range []string{"Configure Cassandra JMX", "Setup and connect Cassandra Reaper"} {
		if _, err := handler(ctx, map[string]any{"stage": name, "content": stageContent("first pass")}); err != nil {
			t.Fatalf("%s: err = %v", name, err)
		}
	}

	if _, err := handler(ctx, map[string]any{
		"stage":   "Setup and connect Cassandra Reaper",
		"content": stageContent("second pass"),
	}); err != nil {
		t.Fatalf("err = %v", err)
	}

	stages := readStoredStages(t, svc, planID)
	if len(stages) != 2 {
		t.Fatalf("stages = %#v, want two", stages)
	}
	steps, ok := stages[1]["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("steps = %#v", stages[1]["steps"])
	}
	step, _ := steps[0].(map[string]any)
	if got := argString(step["action"]); got != "second pass" {
		t.Fatalf("replaced action = %q, want %q", got, "second pass")
	}
	if got := argString(stages[0]["steps"].([]any)[0].(map[string]any)["action"]); got != "first pass" {
		t.Fatalf("untouched stage action = %q", got)
	}
}

func TestUpdateActionPlanHandler_remapsCheckedKeys(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, planID := updateActionPlanFixture(t)
	handler := svc.updateActionPlanHandler(planID, "")

	if _, err := handler(ctx, map[string]any{
		"stage":   "Setup and connect Cassandra Reaper",
		"content": stageContent("install reaper"),
	}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if err := svc.WriteActionPlanChecks(planID, []string{"s0.step0", "s0.check0"}); err != nil {
		t.Fatalf("write checks: %v", err)
	}
	if err := svc.WriteActionPlanComments(planID, map[string]string{"s0.step0": "looks right"}); err != nil {
		t.Fatalf("write comments: %v", err)
	}

	t.Run("an earlier stage pushes the checked keys down", func(t *testing.T) {
		if _, err := handler(ctx, map[string]any{
			"stage":   "Configure Cassandra JMX",
			"content": stageContent("enable jmx"),
		}); err != nil {
			t.Fatalf("err = %v", err)
		}

		checked, err := svc.ReadActionPlanChecks(planID)
		if err != nil {
			t.Fatalf("read checks: %v", err)
		}
		if !slicesEqual(checked, []string{"s1.step0", "s1.check0"}) {
			t.Fatalf("checked = %#v, want [s1.step0 s1.check0]", checked)
		}
		comments, err := svc.ReadActionPlanComments(planID)
		if err != nil {
			t.Fatalf("read comments: %v", err)
		}
		if comments["s1.step0"] != "looks right" {
			t.Fatalf("comments = %#v", comments)
		}
	})

	t.Run("rewriting a stage drops its own keys", func(t *testing.T) {
		if _, err := handler(ctx, map[string]any{
			"stage":   "Setup and connect Cassandra Reaper",
			"content": stageContent("reinstall reaper"),
		}); err != nil {
			t.Fatalf("err = %v", err)
		}

		checked, err := svc.ReadActionPlanChecks(planID)
		if err != nil {
			t.Fatalf("read checks: %v", err)
		}
		if len(checked) != 0 {
			t.Fatalf("checked = %#v, want empty", checked)
		}
		comments, err := svc.ReadActionPlanComments(planID)
		if err != nil {
			t.Fatalf("read comments: %v", err)
		}
		if len(comments) != 0 {
			t.Fatalf("comments = %#v, want empty", comments)
		}
	})
}

func TestUpdateActionPlanHandler_rejectsBadInput(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name     string
		args     map[string]any
		wantPart string
	}{
		{
			name:     "missing stage",
			args:     map[string]any{"content": stageContent("x")},
			wantPart: "stage is required",
		},
		{
			name:     "content is not an object",
			args:     map[string]any{"stage": "Verify repairs", "content": "steps"},
			wantPart: "content must be a JSON object",
		},
		{
			name:     "stage is not in the DAG",
			args:     map[string]any{"stage": "Install Prometheus", "content": stageContent("x")},
			wantPart: `stage "Install Prometheus" is not in the DAG`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, planID := updateActionPlanFixture(t)

			out, err := svc.updateActionPlanHandler(planID, "")(ctx, tt.args)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if !strings.Contains(out, tt.wantPart) {
				t.Fatalf("out = %q, want it to contain %q", out, tt.wantPart)
			}
			if _, found, err := svc.ReadActionPlan(planID); err != nil || found {
				t.Fatalf("ReadActionPlan found = %v, err = %v, want no plan written", found, err)
			}
		})
	}
}

// A fan-out subagent is bound to a single stage. The stages run concurrently
// against one plan file, so writing a sibling's stage has to be refused.
func TestUpdateActionPlanHandler_refusesAStageOutsideItsBinding(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, planID := updateActionPlanFixture(t)

	handler := svc.updateActionPlanHandler(planID, "Configure Cassandra JMX")

	out, err := handler(ctx, map[string]any{
		"stage":   "Verify repairs",
		"content": stageContent("x"),
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, `may not write stage "Verify repairs"`) {
		t.Fatalf("out = %q", out)
	}
	if _, found, err := svc.ReadActionPlan(planID); err != nil || found {
		t.Fatalf("ReadActionPlan found = %v, err = %v, want no plan written", found, err)
	}

	// Its own stage still goes through, loose spelling included.
	out, err = handler(ctx, map[string]any{
		"stage":   "configure  cassandra jmx",
		"content": stageContent("Enable the JMX exporter"),
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, `Stage "Configure Cassandra JMX" saved`) {
		t.Fatalf("out = %q", out)
	}
}

// TestUpdateActionPlanHandler_concurrentStagesAllLand is the regression test for
// the plan fan-out: several stage subagents write the same plan file at once.
// The handler read-modify-writes that file, so without serialisation the last
// writer wins and stages silently disappear. Meaningful under -race.
func TestUpdateActionPlanHandler_concurrentStagesAllLand(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, planID := updateActionPlanFixture(t)

	// Deliberately not in DAG order: the stored order must come from the DAG,
	// not from whichever subagent happened to finish first.
	stages := []string{"Verify repairs", "Configure Cassandra JMX", "Setup and connect Cassandra Reaper"}

	var wg sync.WaitGroup
	for _, stage := range stages {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.updateActionPlanHandler(planID, stage)(ctx, map[string]any{
				"stage":   stage,
				"content": stageContent("work for " + stage),
			}); err != nil {
				t.Errorf("%s: %v", stage, err)
			}
		}()
	}
	wg.Wait()

	stored := readStoredStages(t, svc, planID)
	if len(stored) != len(stages) {
		t.Fatalf("stored %d stages, want %d", len(stored), len(stages))
	}
	want := []string{"Configure Cassandra JMX", "Setup and connect Cassandra Reaper", "Verify repairs"}
	for i, w := range want {
		if got := argString(stored[i]["title"]); got != w {
			t.Fatalf("stage %d title = %q, want %q", i, got, w)
		}
		if got, ok := stored[i]["number"].(float64); !ok || int(got) != i+1 {
			t.Fatalf("stage %d number = %#v, want %d", i, stored[i]["number"], i+1)
		}
	}
}

func TestUpdateActionPlanHandler_refusesWithoutDAG(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	planID := uuid.New()
	svc := &ChatService{
		dagsDir:        t.TempDir(),
		actionPlansDir: t.TempDir(),
		activity:       NewActivityBroker(),
		dialogRepo: &actionListDialogRepo{dialogs: map[uuid.UUID]domain.Dialog{
			planID: {ID: planID, Mode: "decompose"},
		}},
	}

	out, err := svc.updateActionPlanHandler(planID, "")(ctx, map[string]any{
		"stage":   "Configure Cassandra JMX",
		"content": stageContent("x"),
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, "has no DAG") || !strings.Contains(out, CreateActionPlanToolName) {
		t.Fatalf("out = %q", out)
	}
}

func TestAddLocalTools_includesUpdateActionPlanForDialog(t *testing.T) {
	t.Parallel()
	svc := &ChatService{dialogRepo: &actionListDialogRepo{}}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{UpdateActionPlanToolName: {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[UpdateActionPlanToolName]; !ok {
		t.Fatal("expected update_action_plan handler")
	}
	found := false
	for _, def := range catalog.tools {
		if def.Name == UpdateActionPlanToolName {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected update_action_plan in tool defs")
	}
}

func TestAddLocalTools_excludesUpdateActionPlanWithoutPlanOwner(t *testing.T) {
	t.Parallel()
	svc := &ChatService{dialogRepo: &actionListDialogRepo{}}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{UpdateActionPlanToolName: {}}

	// A binding with no plan owner has no action plan to write into.
	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.Nil))

	if _, ok := catalog.localHandlers[UpdateActionPlanToolName]; ok {
		t.Fatal("did not expect update_action_plan handler without a plan owner")
	}
}

func TestAddLocalTools_excludesUpdateActionPlanWithoutAllowList(t *testing.T) {
	t.Parallel()
	svc := &ChatService{dialogRepo: &actionListDialogRepo{}}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{"other_tool": {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[UpdateActionPlanToolName]; ok {
		t.Fatal("did not expect update_action_plan handler when not in allow list")
	}
}

func slicesEqual(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
