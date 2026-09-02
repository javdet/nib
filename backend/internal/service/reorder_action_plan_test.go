package service

import (
	"errors"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func sampleReorderPlan() map[string]any {
	return map[string]any{
		"extra_field": "preserved",
		"stages": []any{
			map[string]any{
				"number":      1,
				"title":       "Stage 1",
				"description": "First stage",
				"steps": []any{
					map[string]any{"type": "code", "action": "step A"},
					map[string]any{"type": "code", "action": "step B"},
					map[string]any{"type": "code", "action": "step C"},
				},
				"checks": []any{
					map[string]any{"check": "check A", "expectation": "expect A"},
					map[string]any{"check": "check B", "expectation": "expect B"},
				},
			},
			map[string]any{
				"number":      2,
				"title":       "Stage 2",
				"description": "Second stage",
				"steps": []any{
					map[string]any{"type": "code", "action": "step X"},
				},
				"checks": []any{},
			},
		},
		"rollback": []any{
			map[string]any{"type": "code", "action": "rollback action"},
		},
	}
}

func writeSamplePlan(dir string, dialogID uuid.UUID) error {
	data, err := json.Marshal(sampleReorderPlan())
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, dialogID.String()+".json"), data, 0o644)
}

func TestMoveIndex(t *testing.T) {
	t.Parallel()

	t.Run("move down", func(t *testing.T) {
		perm, err := moveIndex(3, 0, 2)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		want := []int{2, 0, 1}
		for i, v := range want {
			if perm[i] != v {
				t.Fatalf("perm[%d] = %d, want %d", i, perm[i], v)
			}
		}
	})

	t.Run("move up", func(t *testing.T) {
		perm, err := moveIndex(3, 2, 0)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		want := []int{1, 2, 0}
		for i, v := range want {
			if perm[i] != v {
				t.Fatalf("perm[%d] = %d, want %d", i, perm[i], v)
			}
		}
	})

	t.Run("no op", func(t *testing.T) {
		perm, err := moveIndex(3, 1, 1)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		want := []int{0, 1, 2}
		for i, v := range want {
			if perm[i] != v {
				t.Fatalf("perm[%d] = %d, want %d", i, perm[i], v)
			}
		}
	})

	t.Run("out of range", func(t *testing.T) {
		_, err := moveIndex(3, -1, 0)
		if !errors.Is(err, ErrActionPlanIndexOutOfRange) {
			t.Fatalf("err = %v, want ErrActionPlanIndexOutOfRange", err)
		}
		_, err = moveIndex(3, 0, 3)
		if !errors.Is(err, ErrActionPlanIndexOutOfRange) {
			t.Fatalf("err = %v, want ErrActionPlanIndexOutOfRange", err)
		}
	})
}

func TestRemapCheckedKeys(t *testing.T) {
	t.Parallel()

	perm, err := moveIndex(3, 0, 2)
	if err != nil {
		t.Fatalf("moveIndex: %v", err)
	}

	checked := []string{
		"s0.step0",
		"s0.step1",
		"s0.step2",
		"s0.check0",
		"s1.step0",
		"rollback.0",
	}
	got := remapCheckedKeys(checked, 0, ActionPlanScopeSteps, perm)
	want := []string{
		"s0.step2",
		"s0.step0",
		"s0.step1",
		"s0.check0",
		"s1.step0",
		"rollback.0",
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

func TestRemapCommentKeys(t *testing.T) {
	t.Parallel()

	perm, err := moveIndex(3, 0, 2)
	if err != nil {
		t.Fatalf("moveIndex: %v", err)
	}

	comments := map[string]string{
		"s0.step0":   "note A",
		"s0.step1":   "note B",
		"s0.check0":  "check note",
		"s1.step0":   "other stage",
		"rollback.0": "rollback note",
	}
	got := remapActionPlanKeys(comments, 0, ActionPlanScopeSteps, perm)
	want := map[string]string{
		"s0.step2":   "note A",
		"s0.step0":   "note B",
		"s0.check0":  "check note",
		"s1.step0":   "other stage",
		"rollback.0": "rollback note",
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("got[%q] = %q, want %q", key, got[key], value)
		}
	}
}

func TestReorderActionPlanItems(t *testing.T) {
	t.Parallel()

	dialogID := uuid.New()
	dir := t.TempDir()
	svc := &ChatService{actionPlansDir: dir}

	if err := writeSamplePlan(dir, dialogID); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	checked := []string{"s0.step0", "s0.step1", "s0.step2", "s0.check0", "s1.step0", "rollback.0"}
	if err := svc.WriteActionPlanChecks(dialogID, checked); err != nil {
		t.Fatalf("write checks: %v", err)
	}
	comments := map[string]string{
		"s0.step0":   "note A",
		"s0.step1":   "note B",
		"s0.check0":  "check note",
		"s1.step0":   "other stage",
		"rollback.0": "rollback note",
	}
	if err := svc.WriteActionPlanComments(dialogID, comments); err != nil {
		t.Fatalf("write comments: %v", err)
	}

	execID0 := uuid.New()
	execID1 := uuid.New()
	runs := map[string]string{
		"s0.step0":   execID0.String(),
		"s0.step1":   execID1.String(),
		"s0.check0":  "should-not-remap",
		"s1.step0":   "other-stage",
		"rollback.0": "rollback-exec",
	}
	if err := svc.WriteActionPlanRuns(dialogID, runs); err != nil {
		t.Fatalf("write runs: %v", err)
	}

	t.Run("move step down", func(t *testing.T) {
		if err := writeSamplePlan(dir, dialogID); err != nil {
			t.Fatalf("reset plan: %v", err)
		}
		if err := svc.WriteActionPlanChecks(dialogID, checked); err != nil {
			t.Fatalf("reset checks: %v", err)
		}
		if err := svc.WriteActionPlanComments(dialogID, comments); err != nil {
			t.Fatalf("reset comments: %v", err)
		}
		if err := svc.WriteActionPlanRuns(dialogID, runs); err != nil {
			t.Fatalf("reset runs: %v", err)
		}

		planRaw, gotChecked, gotComments, err := svc.ReorderActionPlanItems(
			dialogID, ActionPlanScopeSteps, 0, 0, 2,
		)
		if err != nil {
			t.Fatalf("ReorderActionPlanItems: %v", err)
		}

		var plan map[string]any
		if err := json.Unmarshal(planRaw, &plan); err != nil {
			t.Fatalf("unmarshal plan: %v", err)
		}
		if plan["extra_field"] != "preserved" {
			t.Fatalf("extra_field = %v, want preserved", plan["extra_field"])
		}

		stages := plan["stages"].([]any)
		stage0 := stages[0].(map[string]any)
		steps := stage0["steps"].([]any)
		actions := make([]string, len(steps))
		for i, s := range steps {
			actions[i] = s.(map[string]any)["action"].(string)
		}
		wantActions := []string{"step B", "step C", "step A"}
		for i, want := range wantActions {
			if actions[i] != want {
				t.Fatalf("actions[%d] = %q, want %q", i, actions[i], want)
			}
		}

		wantChecked := []string{"s0.step2", "s0.step0", "s0.step1", "s0.check0", "s1.step0", "rollback.0"}
		if len(gotChecked) != len(wantChecked) {
			t.Fatalf("checked = %#v, want %#v", gotChecked, wantChecked)
		}
		for i := range wantChecked {
			if gotChecked[i] != wantChecked[i] {
				t.Fatalf("checked[%d] = %q, want %q", i, gotChecked[i], wantChecked[i])
			}
		}

		if gotComments["s0.step2"] != "note A" {
			t.Fatalf("comments[s0.step2] = %q, want note A", gotComments["s0.step2"])
		}
		if gotComments["s0.step0"] != "note B" {
			t.Fatalf("comments[s0.step0] = %q, want note B", gotComments["s0.step0"])
		}
		if gotComments["s0.check0"] != "check note" {
			t.Fatalf("comments[s0.check0] = %q, want check note", gotComments["s0.check0"])
		}
		if gotComments["rollback.0"] != "rollback note" {
			t.Fatalf("comments[rollback.0] = %q", gotComments["rollback.0"])
		}

		gotRuns, err := svc.ReadActionPlanRuns(dialogID)
		if err != nil {
			t.Fatalf("ReadActionPlanRuns: %v", err)
		}
		if gotRuns["s0.step2"] != execID0.String() {
			t.Fatalf("runs[s0.step2] = %q, want %q", gotRuns["s0.step2"], execID0.String())
		}
		if gotRuns["s0.step0"] != execID1.String() {
			t.Fatalf("runs[s0.step0] = %q, want %q", gotRuns["s0.step0"], execID1.String())
		}
		if gotRuns["s0.check0"] != "should-not-remap" {
			t.Fatalf("runs[s0.check0] = %q, want should-not-remap", gotRuns["s0.check0"])
		}
	})

	t.Run("move check up", func(t *testing.T) {
		if err := writeSamplePlan(dir, dialogID); err != nil {
			t.Fatalf("reset plan: %v", err)
		}
		if err := svc.WriteActionPlanChecks(dialogID, checked); err != nil {
			t.Fatalf("reset checks: %v", err)
		}
		if err := svc.WriteActionPlanComments(dialogID, comments); err != nil {
			t.Fatalf("reset comments: %v", err)
		}

		planRaw, gotChecked, gotComments, err := svc.ReorderActionPlanItems(
			dialogID, ActionPlanScopeChecks, 0, 1, 0,
		)
		if err != nil {
			t.Fatalf("ReorderActionPlanItems: %v", err)
		}

		var plan map[string]any
		if err := json.Unmarshal(planRaw, &plan); err != nil {
			t.Fatalf("unmarshal plan: %v", err)
		}
		stages := plan["stages"].([]any)
		stage0 := stages[0].(map[string]any)
		checks := stage0["checks"].([]any)
		names := make([]string, len(checks))
		for i, c := range checks {
			names[i] = c.(map[string]any)["check"].(string)
		}
		wantNames := []string{"check B", "check A"}
		for i, want := range wantNames {
			if names[i] != want {
				t.Fatalf("checks[%d] = %q, want %q", i, names[i], want)
			}
		}

		if len(gotChecked) != len(checked) {
			t.Fatalf("checked length changed unexpectedly")
		}
		if len(gotComments) != len(comments) {
			t.Fatalf("comments length changed unexpectedly")
		}
	})

	t.Run("invalid scope", func(t *testing.T) {
		_, _, _, err := svc.ReorderActionPlanItems(dialogID, ActionPlanScope("invalid"), 0, 0, 1)
		if !errors.Is(err, ErrInvalidActionPlanScope) {
			t.Fatalf("err = %v, want ErrInvalidActionPlanScope", err)
		}
	})

	t.Run("out of range stage", func(t *testing.T) {
		_, _, _, err := svc.ReorderActionPlanItems(dialogID, ActionPlanScopeSteps, 99, 0, 1)
		if !errors.Is(err, ErrActionPlanIndexOutOfRange) {
			t.Fatalf("err = %v, want ErrActionPlanIndexOutOfRange", err)
		}
	})

	t.Run("out of range from", func(t *testing.T) {
		_, _, _, err := svc.ReorderActionPlanItems(dialogID, ActionPlanScopeSteps, 0, 99, 0)
		if !errors.Is(err, ErrActionPlanIndexOutOfRange) {
			t.Fatalf("err = %v, want ErrActionPlanIndexOutOfRange", err)
		}
	})

	t.Run("no op move", func(t *testing.T) {
		if err := writeSamplePlan(dir, dialogID); err != nil {
			t.Fatalf("reset plan: %v", err)
		}
		planRaw, gotChecked, gotComments, err := svc.ReorderActionPlanItems(
			dialogID, ActionPlanScopeSteps, 0, 1, 1,
		)
		if err != nil {
			t.Fatalf("ReorderActionPlanItems: %v", err)
		}
		if planRaw == nil {
			t.Fatal("expected plan")
		}
		if len(gotChecked) == 0 {
			t.Fatal("expected checked")
		}
		if len(gotComments) == 0 {
			t.Fatal("expected comments")
		}
	})
}
