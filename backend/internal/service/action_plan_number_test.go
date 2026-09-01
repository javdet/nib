package service

import (
	"encoding/json"
	"testing"
)

func TestActionPlanItemNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		stage int
		scope ActionPlanScope
		index int
		want  string
	}{
		{"first step of first stage", 0, ActionPlanScopeSteps, 0, "1.1"},
		{"third step of second stage", 1, ActionPlanScopeSteps, 2, "2.3"},
		{"first check of first stage", 0, ActionPlanScopeChecks, 0, "1.C1"},
		{"second check of third stage", 2, ActionPlanScopeChecks, 1, "3.C2"},
		{"two digit positions", 9, ActionPlanScopeSteps, 11, "10.12"},
		{"unknown scope", 0, ActionPlanScope("rollback"), 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := actionPlanItemNumber(tt.stage, tt.scope, tt.index); got != tt.want {
				t.Fatalf("actionPlanItemNumber = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestActionPlanRollbackNumber(t *testing.T) {
	t.Parallel()

	for index, want := range map[int]string{0: "R1", 1: "R2", 9: "R10"} {
		if got := actionPlanRollbackNumber(index); got != want {
			t.Fatalf("actionPlanRollbackNumber(%d) = %q, want %q", index, got, want)
		}
	}
}

func TestActionPlanStageNumber(t *testing.T) {
	t.Parallel()

	if got := actionPlanStageNumber(0); got != 1 {
		t.Fatalf("actionPlanStageNumber(0) = %d, want 1", got)
	}
	if got := actionPlanStageNumber(4); got != 5 {
		t.Fatalf("actionPlanStageNumber(4) = %d, want 5", got)
	}
}

// TestRenumberActionPlan_overrulesSuppliedNumbers proves the invariant the write
// path relies on: position decides every number, whatever the model sent.
func TestRenumberActionPlan_overrulesSuppliedNumbers(t *testing.T) {
	t.Parallel()

	var plan map[string]any
	raw := `{
		"stages": [
			{
				"number": 7,
				"title": "Prepare",
				"steps": [
					{"type": "code", "action": "edit", "number": "wrong"},
					{"type": "shell", "action": "run"}
				],
				"checks": [{"check": "ok", "expectation": "yes", "number": 42}]
			},
			{
				"title": "Roll out",
				"steps": [{"type": "web", "action": "click"}],
				"checks": []
			}
		],
		"rollback": [
			{"type": "curl", "action": "revert", "number": "R9"},
			{"type": "shell", "action": "undo"}
		]
	}`
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	renumberActionPlan(plan)

	stages := plan["stages"].([]any)
	first := stages[0].(map[string]any)
	if got := first["number"]; got != 1 {
		t.Fatalf("first stage number = %#v, want 1", got)
	}
	if got := stages[1].(map[string]any)["number"]; got != 2 {
		t.Fatalf("second stage number = %#v, want 2", got)
	}

	steps := first["steps"].([]any)
	if got := steps[0].(map[string]any)["number"]; got != "1.1" {
		t.Fatalf("first step number = %#v, want 1.1", got)
	}
	if got := steps[1].(map[string]any)["number"]; got != "1.2" {
		t.Fatalf("second step number = %#v, want 1.2", got)
	}

	checks := first["checks"].([]any)
	if got := checks[0].(map[string]any)["number"]; got != "1.C1" {
		t.Fatalf("check number = %#v, want 1.C1", got)
	}

	secondSteps := stages[1].(map[string]any)["steps"].([]any)
	if got := secondSteps[0].(map[string]any)["number"]; got != "2.1" {
		t.Fatalf("second stage step number = %#v, want 2.1", got)
	}

	rollback := plan["rollback"].([]any)
	if got := rollback[0].(map[string]any)["number"]; got != "R1" {
		t.Fatalf("first rollback number = %#v, want R1", got)
	}
	if got := rollback[1].(map[string]any)["number"]; got != "R2" {
		t.Fatalf("second rollback number = %#v, want R2", got)
	}
}

// TestRenumberActionPlan_leavesUnknownShapesAlone covers the documents a write
// must survive: extra keys the plan format does not know about, and array
// elements that did not decode to an object.
func TestRenumberActionPlan_leavesUnknownShapesAlone(t *testing.T) {
	t.Parallel()

	var plan map[string]any
	raw := `{
		"extra_field": "preserved",
		"stages": [
			"not an object",
			{
				"title": "Deploy",
				"steps": ["not an object", {"type": "shell", "action": "run", "keep": "me"}],
				"checks": "not an array"
			}
		],
		"rollback": [null, {"type": "shell", "action": "undo"}]
	}`
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	renumberActionPlan(plan)

	if got := plan["extra_field"]; got != "preserved" {
		t.Fatalf("extra_field = %#v, want preserved", got)
	}

	stages := plan["stages"].([]any)
	if got := stages[0]; got != "not an object" {
		t.Fatalf("non-object stage = %#v, want untouched", got)
	}

	stage := stages[1].(map[string]any)
	if got := stage["number"]; got != 2 {
		t.Fatalf("stage number = %#v, want 2 (position, not object count)", got)
	}
	if got := stage["checks"]; got != "not an array" {
		t.Fatalf("non-array checks = %#v, want untouched", got)
	}

	steps := stage["steps"].([]any)
	step := steps[1].(map[string]any)
	if got := step["number"]; got != "2.2" {
		t.Fatalf("step number = %#v, want 2.2", got)
	}
	if got := step["keep"]; got != "me" {
		t.Fatalf("unknown step key = %#v, want me", got)
	}

	rollback := plan["rollback"].([]any)
	if rollback[0] != nil {
		t.Fatalf("null rollback entry = %#v, want untouched", rollback[0])
	}
	if got := rollback[1].(map[string]any)["number"]; got != "R2" {
		t.Fatalf("rollback number = %#v, want R2", got)
	}
}

// TestRenumberActionPlan_emptyPlan guards the drafting path, where a plan file
// exists before any stage has been stored.
func TestRenumberActionPlan_emptyPlan(t *testing.T) {
	t.Parallel()

	plan := map[string]any{}
	renumberActionPlan(plan)
	if len(plan) != 0 {
		t.Fatalf("plan = %#v, want empty", plan)
	}
}
