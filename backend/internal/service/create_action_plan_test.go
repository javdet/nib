package service

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestCreateActionPlanToolDef(t *testing.T) {
	t.Parallel()
	def := CreateActionPlanToolDef(t.TempDir(), nil)
	if def.Name != CreateActionPlanToolName {
		t.Fatalf("name = %q, want %q", def.Name, CreateActionPlanToolName)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(def.Parameters, &schema); err != nil {
		t.Fatalf("unmarshal parameters: %v", err)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "plan" {
		t.Fatalf("required = %#v, want [plan]", schema.Required)
	}
}

// TestCreateActionPlanSchema_stepPropertiesMatchFallback guards the two copies
// of the step schema: the file shipped in seed/tools (copied into DATA_DIR/tools
// on first start) and the fallback used when that file is missing. A field added
// to one and not the other silently changes the plan contract depending on how
// the service is deployed.
func TestCreateActionPlanSchema_stepPropertiesMatchFallback(t *testing.T) {
	t.Parallel()

	fileParams := loadCreateActionPlanParameters(systemToolsTestDataDir(t))
	if string(fileParams) == string(defaultCreateActionPlanParameters) {
		t.Fatal("expected the schema file to be loaded, got the fallback")
	}

	for _, scope := range []string{"steps", "rollback"} {
		fromFile := actionPlanStepProperties(t, fileParams, scope)
		fromFallback := actionPlanStepProperties(t, defaultCreateActionPlanParameters, scope)
		if !maps.Equal(fromFile, fromFallback) {
			t.Fatalf("%s properties: file = %v, fallback = %v", scope, fromFile, fromFallback)
		}
		if _, ok := fromFile["command"]; !ok {
			t.Fatalf("%s properties = %v, want a command field", scope, fromFile)
		}
	}
}

// actionPlanStepProperties returns the property names declared for a stage step
// ("steps") or a rollback entry ("rollback") of the create_action_plan schema.
func actionPlanStepProperties(t *testing.T, schema json.RawMessage, scope string) map[string]struct{} {
	t.Helper()

	var doc struct {
		Properties struct {
			Plan struct {
				Properties struct {
					Stages struct {
						Items struct {
							Properties struct {
								Steps struct {
									Items map[string]any `json:"items"`
								} `json:"steps"`
							} `json:"properties"`
						} `json:"items"`
					} `json:"stages"`
					Rollback struct {
						Items map[string]any `json:"items"`
					} `json:"rollback"`
				} `json:"properties"`
			} `json:"plan"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &doc); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}

	items := doc.Properties.Plan.Properties.Rollback.Items
	if scope == "steps" {
		items = doc.Properties.Plan.Properties.Stages.Items.Properties.Steps.Items
	}

	props, ok := items["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s schema has no properties object: %#v", scope, items)
	}

	names := make(map[string]struct{}, len(props))
	for name := range props {
		names[name] = struct{}{}
	}
	return names
}

func TestCreateActionPlanHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dialogID := uuid.New()
	dir := t.TempDir()

	svc := &ChatService{actionPlansDir: dir}
	handler := svc.createActionPlanHandler(dialogID)

	samplePlan := map[string]any{
		"stages": []any{
			map[string]any{
				"number":      1,
				"title":       "Deploy",
				"description": "Deploy service",
				"steps": []any{
					map[string]any{"type": "code", "action": "Update Helm values"},
				},
				"checks": []any{
					map[string]any{"check": "Pod running", "expectation": "kubectl get pods shows Running"},
				},
			},
		},
		"rollback": []any{
			map[string]any{"type": "code", "action": "Revert Helm values"},
		},
	}

	t.Run("writes action plan json", func(t *testing.T) {
		out, err := handler(ctx, map[string]any{"plan": samplePlan})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		wantRel := filepath.Join("action_plans", dialogID.String()+".json")
		if out != "Action plan saved to "+wantRel {
			t.Fatalf("out = %q", out)
		}

		data, err := os.ReadFile(filepath.Join(dir, dialogID.String()+".json"))
		if err != nil {
			t.Fatalf("read file: %v", err)
		}

		var stored map[string]any
		if err := json.Unmarshal(data, &stored); err != nil {
			t.Fatalf("unmarshal stored plan: %v", err)
		}
		stages, ok := stored["stages"].([]any)
		if !ok || len(stages) != 1 {
			t.Fatalf("stored stages = %#v", stored["stages"])
		}
	})

	t.Run("overwrites existing file and clears checks", func(t *testing.T) {
		checksPath := actionPlanChecksPath(dir, dialogID)
		if err := os.WriteFile(checksPath, []byte(`["s0.step0"]`), 0o644); err != nil {
			t.Fatalf("write checks: %v", err)
		}
		commentsPath := actionPlanCommentsPath(dir, dialogID)
		if err := os.WriteFile(commentsPath, []byte(`{"s0.step0":"note"}`), 0o644); err != nil {
			t.Fatalf("write comments: %v", err)
		}
		notesPath := actionPlanNotesPath(dir, dialogID)
		if err := os.WriteFile(notesPath, []byte(`{"s0.step0":"created vpc-0a91f3"}`), 0o644); err != nil {
			t.Fatalf("write notes: %v", err)
		}

		_, err := handler(ctx, map[string]any{"plan": samplePlan})
		if err != nil {
			t.Fatalf("first call: %v", err)
		}

		updated := map[string]any{
			"stages":   []any{},
			"rollback": []any{},
		}
		out, err := handler(ctx, map[string]any{"plan": updated})
		if err != nil {
			t.Fatalf("second call: %v", err)
		}
		if out != "Action plan saved to "+filepath.Join("action_plans", dialogID.String()+".json") {
			t.Fatalf("out = %q", out)
		}

		if _, err := os.Stat(checksPath); !os.IsNotExist(err) {
			t.Fatal("expected checks file to be removed on plan overwrite")
		}
		if _, err := os.Stat(commentsPath); !os.IsNotExist(err) {
			t.Fatal("expected comments file to be removed on plan overwrite")
		}
		// The actions are different work now; a result reported by the ones they
		// replaced would be handed to whoever executes these.
		if _, err := os.Stat(notesPath); !os.IsNotExist(err) {
			t.Fatal("expected notes file to be removed on plan overwrite")
		}
	})

	t.Run("missing plan", func(t *testing.T) {
		out, err := handler(ctx, map[string]any{})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if out != "plan is required" {
			t.Fatalf("out = %q", out)
		}
	})
}

func TestReadWriteActionPlanChecks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dialogID := uuid.New()
	svc := &ChatService{actionPlansDir: dir}

	checked, err := svc.ReadActionPlanChecks(dialogID)
	if err != nil {
		t.Fatalf("read empty checks: %v", err)
	}
	if len(checked) != 0 {
		t.Fatalf("checked = %#v, want empty", checked)
	}

	want := []string{"s0.step0", "s0.check0", "rollback.0"}
	if err := svc.WriteActionPlanChecks(dialogID, want); err != nil {
		t.Fatalf("write checks: %v", err)
	}

	got, err := svc.ReadActionPlanChecks(dialogID)
	if err != nil {
		t.Fatalf("read checks: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReadWriteActionPlanComments(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dialogID := uuid.New()
	svc := &ChatService{actionPlansDir: dir}

	comments, err := svc.ReadActionPlanComments(dialogID)
	if err != nil {
		t.Fatalf("read empty comments: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments = %#v, want empty", comments)
	}

	want := map[string]string{
		"s0.step0":    "Use staging first",
		"rollback.0":  "Revert carefully",
	}
	if err := svc.WriteActionPlanComments(dialogID, want); err != nil {
		t.Fatalf("write comments: %v", err)
	}

	got, err := svc.ReadActionPlanComments(dialogID)
	if err != nil {
		t.Fatalf("read comments: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got = %#v, want %#v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("got[%q] = %q, want %q", key, got[key], value)
		}
	}
}

func TestAddLocalTools_includesCreateActionPlanForDialog(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{CreateActionPlanToolName: {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[CreateActionPlanToolName]; !ok {
		t.Fatal("expected create_action_plan handler")
	}
	found := false
	for _, def := range catalog.tools {
		if def.Name == CreateActionPlanToolName {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected create_action_plan in tool defs")
	}
}

func TestAddLocalTools_excludesCreateActionPlanWithoutAllowList(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{"other_tool": {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[CreateActionPlanToolName]; ok {
		t.Fatal("did not expect create_action_plan handler when not in allow list")
	}
}
