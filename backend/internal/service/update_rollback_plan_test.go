package service

import (
	"context"
	"encoding/json"
	"maps"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

func TestUpdateRollbackPlanToolDef(t *testing.T) {
	t.Parallel()
	def := UpdateRollbackPlanToolDef(t.TempDir(), nil)
	if def.Name != UpdateRollbackPlanToolName {
		t.Fatalf("name = %q, want %q", def.Name, UpdateRollbackPlanToolName)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(def.Parameters, &schema); err != nil {
		t.Fatalf("unmarshal parameters: %v", err)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "rollback" {
		t.Fatalf("required = %#v, want [rollback]", schema.Required)
	}
}

// TestUpdateRollbackPlanSchema_entryPropertiesMatchCreate guards the three copies
// of the rollback entry schema: the two shipped in seed/tools and the fallback
// used when the update file is missing. A rollback written through
// update_rollback_plan has to render the same as one written through
// create_action_plan.
func TestUpdateRollbackPlanSchema_entryPropertiesMatchCreate(t *testing.T) {
	t.Parallel()

	seedDir := systemToolsTestDataDir(t)
	fileParams := loadUpdateRollbackPlanParameters(seedDir)
	if string(fileParams) == string(defaultUpdateRollbackPlanParameters) {
		t.Fatal("expected the schema file to be loaded, got the fallback")
	}

	want := actionPlanStepProperties(t, loadCreateActionPlanParameters(seedDir), "rollback")
	for name, schema := range map[string]json.RawMessage{
		"file":     fileParams,
		"fallback": defaultUpdateRollbackPlanParameters,
	} {
		got := updateRollbackEntryProperties(t, schema)
		if !maps.Equal(got, want) {
			t.Fatalf("%s rollback properties = %v, want %v", name, got, want)
		}
	}
}

func updateRollbackEntryProperties(t *testing.T, schema json.RawMessage) map[string]struct{} {
	t.Helper()

	var doc struct {
		Properties struct {
			Rollback struct {
				Items map[string]any `json:"items"`
			} `json:"rollback"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &doc); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}

	props, ok := doc.Properties.Rollback.Items["properties"].(map[string]any)
	if !ok {
		t.Fatalf("rollback schema has no properties object: %#v", doc.Properties.Rollback.Items)
	}

	names := make(map[string]struct{}, len(props))
	for name := range props {
		names[name] = struct{}{}
	}
	return names
}

// rollbackEntries is the list a rollback agent would send: shell first, then the
// one code entry for the repository.
func rollbackEntries() []any {
	return []any{
		map[string]any{
			"type":    "shell",
			"action":  "Roll the api deployment back to the previous revision.",
			"command": "kubectl -n prod rollout undo deployment/api",
		},
		map[string]any{
			"type":       "code",
			"repository": "helm-charts",
			"pr_title":   "chore: revert chart version bump",
			"action":     "Full revert for this repository in one pull request.",
		},
	}
}

// readStoredRollback returns the rollback list of the plan written for dialogID.
func readStoredRollback(t *testing.T, svc *ChatService, dialogID uuid.UUID) []map[string]any {
	t.Helper()

	raw, found, err := svc.ReadActionPlan(dialogID)
	if err != nil {
		t.Fatalf("read action plan: %v", err)
	}
	if !found {
		t.Fatal("expected a stored action plan")
	}

	var plan struct {
		Rollback []map[string]any `json:"rollback"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatalf("unmarshal action plan: %v", err)
	}
	return plan.Rollback
}

// rollbackFixture returns a service whose plan already holds one stage, which is
// the state the rollback agent runs in.
func rollbackFixture(t *testing.T) (*ChatService, uuid.UUID) {
	t.Helper()

	svc, planID := updateActionPlanFixture(t)
	if _, err := svc.updateActionPlanHandler(planID, "")(context.Background(), map[string]any{
		"stage":   "Configure Cassandra JMX",
		"content": stageContent("Enable the JMX exporter"),
	}); err != nil {
		t.Fatalf("seed stage: %v", err)
	}
	return svc, planID
}

func TestUpdateRollbackPlanHandler_writesRollbackAndLeavesStagesAlone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, planID := rollbackFixture(t)

	before, err := json.Marshal(readStoredStages(t, svc, planID))
	if err != nil {
		t.Fatalf("marshal stages: %v", err)
	}

	events, unsubscribe := svc.SubscribeActivity(planID)
	defer unsubscribe()

	out, err := svc.updateRollbackPlanHandler(planID)(ctx, map[string]any{"rollback": rollbackEntries()})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, "2 entries") || !strings.Contains(out, "R1 to R2") {
		t.Fatalf("out = %q", out)
	}

	rollback := readStoredRollback(t, svc, planID)
	if len(rollback) != 2 {
		t.Fatalf("rollback = %#v, want two entries", rollback)
	}
	if got := argString(rollback[0]["number"]); got != "R1" {
		t.Fatalf("first number = %q, want R1", got)
	}
	if got := argString(rollback[1]["number"]); got != "R2" {
		t.Fatalf("second number = %q, want R2", got)
	}
	if got := argString(rollback[1]["repository"]); got != "helm-charts" {
		t.Fatalf("repository = %q", got)
	}

	after, err := json.Marshal(readStoredStages(t, svc, planID))
	if err != nil {
		t.Fatalf("marshal stages: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("stages changed:\nbefore %s\nafter  %s", before, after)
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

// The numbers are derived from position on every write, so one the model sends is
// discarded rather than trusted.
func TestUpdateRollbackPlanHandler_overrulesSuppliedNumbers(t *testing.T) {
	t.Parallel()
	svc, planID := rollbackFixture(t)

	if _, err := svc.updateRollbackPlanHandler(planID)(context.Background(), map[string]any{
		"rollback": []any{
			map[string]any{"type": "web", "action": "first", "number": "R7"},
			map[string]any{"type": "web", "action": "second"},
		},
	}); err != nil {
		t.Fatalf("err = %v", err)
	}

	rollback := readStoredRollback(t, svc, planID)
	if got := argString(rollback[0]["number"]); got != "R1" {
		t.Fatalf("number = %q, want R1", got)
	}
}

func TestUpdateRollbackPlanHandler_rejectsMalformedInput(t *testing.T) {
	t.Parallel()
	svc, planID := rollbackFixture(t)
	handler := svc.updateRollbackPlanHandler(planID)

	for name, args := range map[string]map[string]any{
		"missing":    {},
		"not a list": {"rollback": "revert everything"},
		"not object": {"rollback": []any{"revert everything"}},
	} {
		out, err := handler(context.Background(), args)
		if err != nil {
			t.Fatalf("%s: err = %v", name, err)
		}
		if !strings.Contains(out, "rollback") {
			t.Fatalf("%s: out = %q, want an explanation naming rollback", name, out)
		}
		if len(readStoredRollback(t, svc, planID)) != 0 {
			t.Fatalf("%s: stored a rollback anyway", name)
		}
	}
}

// A plan whose stages all failed has nothing to undo, and the plan document is
// created on demand, so writing here would render an undo for work nobody planned.
func TestUpdateRollbackPlanHandler_refusesAPlanWithNoStages(t *testing.T) {
	t.Parallel()
	svc, planID := updateActionPlanFixture(t)

	out, err := svc.updateRollbackPlanHandler(planID)(context.Background(), map[string]any{
		"rollback": rollbackEntries(),
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, "no stages") {
		t.Fatalf("out = %q", out)
	}
	if _, found, err := svc.ReadActionPlan(planID); err != nil || found {
		t.Fatalf("ReadActionPlan found = %v, err = %v, want no plan written", found, err)
	}
}

// The keys are positional, so a changed list invalidates them -- and an identical
// one does not, which is what lets a fan-out round re-derive the rollback without
// costing the operator their checkboxes.
func TestUpdateRollbackPlanHandler_dropsRollbackKeysOnlyWhenTheListChanges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, planID := rollbackFixture(t)
	handler := svc.updateRollbackPlanHandler(planID)

	if _, err := handler(ctx, map[string]any{"rollback": rollbackEntries()}); err != nil {
		t.Fatalf("first write: err = %v", err)
	}

	if err := svc.WriteActionPlanChecks(planID, []string{"rollback.0", "s0.step0"}); err != nil {
		t.Fatalf("write checks: %v", err)
	}
	if err := svc.WriteActionPlanComments(planID, map[string]string{
		"rollback.1": "ask the DBA first",
		"s0.check0":  "flaky",
	}); err != nil {
		t.Fatalf("write comments: %v", err)
	}
	if err := svc.WriteActionPlanRuns(planID, map[string]string{"rollback.1": uuid.NewString()}); err != nil {
		t.Fatalf("write runs: %v", err)
	}

	// Re-derived identically: everything survives.
	out, err := handler(ctx, map[string]any{"rollback": rollbackEntries()})
	if err != nil {
		t.Fatalf("second write: err = %v", err)
	}
	if !strings.Contains(out, "unchanged") {
		t.Fatalf("out = %q, want it to report the list unchanged", out)
	}
	checks, err := svc.ReadActionPlanChecks(planID)
	if err != nil {
		t.Fatalf("read checks: %v", err)
	}
	if len(checks) != 2 {
		t.Fatalf("checks = %v, want both kept", checks)
	}

	// Changed: the rollback keys go, the stage keys stay.
	if _, err := handler(ctx, map[string]any{"rollback": []any{
		map[string]any{"type": "web", "action": "Turn the feature flag off in the console."},
	}}); err != nil {
		t.Fatalf("third write: err = %v", err)
	}

	checks, err = svc.ReadActionPlanChecks(planID)
	if err != nil {
		t.Fatalf("read checks: %v", err)
	}
	if len(checks) != 1 || checks[0] != "s0.step0" {
		t.Fatalf("checks = %v, want only the stage key", checks)
	}
	comments, err := svc.ReadActionPlanComments(planID)
	if err != nil {
		t.Fatalf("read comments: %v", err)
	}
	if len(comments) != 1 || comments["s0.check0"] != "flaky" {
		t.Fatalf("comments = %v, want only the stage key", comments)
	}
	runs, err := svc.ReadActionPlanRuns(planID)
	if err != nil {
		t.Fatalf("read runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("runs = %v, want the rollback run key dropped", runs)
	}
}

// A stage write moves stage keys around and must leave the rollback's alone:
// rollback keys carry no stage index, so there is nothing in them to shift.
func TestUpdateActionPlanHandler_leavesRollbackKeysAlone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, planID := rollbackFixture(t)

	if _, err := svc.updateRollbackPlanHandler(planID)(ctx, map[string]any{"rollback": rollbackEntries()}); err != nil {
		t.Fatalf("write rollback: %v", err)
	}
	if err := svc.WriteActionPlanChecks(planID, []string{"rollback.0", "rollback.1"}); err != nil {
		t.Fatalf("write checks: %v", err)
	}

	// A stage inserted before the existing one shifts every stage key by hand.
	if _, err := svc.updateActionPlanHandler(planID, "")(ctx, map[string]any{
		"stage":   "Verify repairs",
		"content": stageContent("check the repair log"),
	}); err != nil {
		t.Fatalf("write stage: %v", err)
	}

	checks, err := svc.ReadActionPlanChecks(planID)
	if err != nil {
		t.Fatalf("read checks: %v", err)
	}
	if len(checks) != 2 {
		t.Fatalf("checks = %v, want both rollback keys kept", checks)
	}
	if len(readStoredRollback(t, svc, planID)) != 2 {
		t.Fatal("expected the rollback to survive a stage write")
	}
}

func TestAddLocalTools_includesUpdateRollbackPlanForPlanOwner(t *testing.T) {
	t.Parallel()
	svc := &ChatService{dialogRepo: &actionListDialogRepo{}}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{UpdateRollbackPlanToolName: {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[UpdateRollbackPlanToolName]; !ok {
		t.Fatal("expected update_rollback_plan handler")
	}
	found := false
	for _, def := range catalog.tools {
		if def.Name == UpdateRollbackPlanToolName {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected update_rollback_plan in tool defs")
	}
}

func TestAddLocalTools_excludesUpdateRollbackPlanWhenNotAllowedOrUnowned(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		allow   map[string]struct{}
		binding toolBinding
	}{
		"not allowed":   {map[string]struct{}{"other_tool": {}}, newToolBinding(uuid.New())},
		"no plan owner": {map[string]struct{}{UpdateRollbackPlanToolName: {}}, newToolBinding(uuid.Nil)},
	} {
		svc := &ChatService{dialogRepo: &actionListDialogRepo{}}
		catalog := toolCatalog{
			mcpRoutes:     make(map[string]toolRoute),
			localHandlers: make(map[string]localToolHandler),
		}

		svc.addLocalTools(&catalog, tc.allow, tc.binding)

		if _, ok := catalog.localHandlers[UpdateRollbackPlanToolName]; ok {
			t.Fatalf("%s: did not expect an update_rollback_plan handler", name)
		}
	}
}
