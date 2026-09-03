package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
)

type mapDialogRepo struct {
	dialogs map[uuid.UUID]domain.Dialog
}

func (r *mapDialogRepo) CreateDialog(_ context.Context, mode, title string, parentID *uuid.UUID) (domain.Dialog, error) {
	d := domain.Dialog{ID: uuid.New(), Mode: mode, Title: title, ParentID: parentID}
	r.dialogs[d.ID] = d
	return d, nil
}

func (r *mapDialogRepo) ListChildren(_ context.Context, _ uuid.UUID) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *mapDialogRepo) ListRecentDialogs(_ context.Context, _, _ int) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *mapDialogRepo) CountDialogs(_ context.Context) (int, error) {
	return 0, nil
}

func (r *mapDialogRepo) ListDialogsByMode(_ context.Context, _ string, _, _ int) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *mapDialogRepo) CountDialogsByMode(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (r *mapDialogRepo) ListAllRecentDialogs(_ context.Context, _, _ int) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *mapDialogRepo) CountAllDialogs(_ context.Context) (int, error) {
	return 0, nil
}

func (r *mapDialogRepo) SearchDialogs(_ context.Context, _, _ string, _, _ int) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *mapDialogRepo) CountDialogsSearch(_ context.Context, _, _ string) (int, error) {
	return 0, nil
}

func (r *mapDialogRepo) GetDialog(_ context.Context, id uuid.UUID) (domain.Dialog, error) {
	d, ok := r.dialogs[id]
	if !ok {
		return domain.Dialog{}, repository.ErrNotFound
	}
	return d, nil
}

func (r *mapDialogRepo) UpdateTitle(_ context.Context, _ uuid.UUID, title string) error {
	return nil
}

func (r *mapDialogRepo) SetDialogTaskID(_ context.Context, _ uuid.UUID, taskID *string) error {
	return nil
}

func (r *mapDialogRepo) SetDialogCategories(_ context.Context, _ uuid.UUID, categories []string) error {
	return nil
}

func (r *mapDialogRepo) SetDialogPinned(_ context.Context, _ uuid.UUID, pinned bool) error {
	return nil
}

func (r *mapDialogRepo) ListPinnedDialogs(_ context.Context) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *mapDialogRepo) DeleteDialog(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (r *mapDialogRepo) ListMessages(_ context.Context, _ uuid.UUID) ([]domain.DialogMessage, error) {
	return nil, nil
}

func (r *mapDialogRepo) AppendMessage(_ context.Context, dialogID uuid.UUID, msg domain.DialogMessage) (domain.DialogMessage, error) {
	msg.DialogID = dialogID
	return msg, nil
}

func (r *mapDialogRepo) DeleteMessagesAfterSeq(_ context.Context, _ uuid.UUID, afterSeq int) error {
	return nil
}

func writeAttachTestPlan(dir string, planID uuid.UUID) error {
	plan := map[string]any{
		"extra_field": "preserved",
		"stages": []any{
			map[string]any{
				"number":      1,
				"title":       "Stage 1",
				"description": "First",
				"steps": []any{
					map[string]any{"type": "code", "action": "step A", "repository": "org/repo"},
					map[string]any{"type": "code", "action": "step B", "repository": "org/repo"},
				},
				"checks": []any{},
			},
		},
		"rollback": []any{
			map[string]any{"type": "code", "action": "rollback", "repository": "org/repo"},
		},
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, planID.String()+".json"), data, 0o644)
}

func TestAttachActionPullRequest(t *testing.T) {
	t.Parallel()

	planID := uuid.New()
	execID := uuid.New()
	prURL := "https://github.com/org/repo/pull/42"

	dir := t.TempDir()
	svc := &ChatService{
		actionPlansDir: dir,
		activity:       NewActivityBroker(),
	}

	if err := writeAttachTestPlan(dir, planID); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	if err := svc.WriteActionPlanRuns(planID, map[string]string{"s0.step1": execID.String()}); err != nil {
		t.Fatalf("write runs: %v", err)
	}

	events, unsub := svc.SubscribeActivity(planID)
	defer unsub()

	exec := domain.Dialog{
		ID:       execID,
		Mode:     "execute",
		ParentID: &planID,
	}
	if err := svc.attachActionPullRequest(context.Background(), exec, prURL); err != nil {
		t.Fatalf("attachActionPullRequest: %v", err)
	}

	raw, found, err := svc.ReadActionPlan(planID)
	if err != nil || !found {
		t.Fatalf("ReadActionPlan: found=%v err=%v", found, err)
	}
	var plan map[string]any
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if plan["extra_field"] != "preserved" {
		t.Fatalf("extra_field = %v, want preserved", plan["extra_field"])
	}
	stages := plan["stages"].([]any)
	stage0 := stages[0].(map[string]any)
	steps := stage0["steps"].([]any)
	step1 := steps[1].(map[string]any)
	if step1["pr_url"] != prURL {
		t.Fatalf("step pr_url = %v, want %q", step1["pr_url"], prURL)
	}
	step0 := steps[0].(map[string]any)
	if _, ok := step0["pr_url"]; ok {
		t.Fatalf("step0 should not have pr_url")
	}

	select {
	case ev := <-events:
		if ev.Kind != domain.ActivityActionPlanUpdated {
			t.Fatalf("activity kind = %q, want action_plan_updated", ev.Kind)
		}
	default:
		t.Fatal("expected action_plan_updated activity event")
	}
}

func TestAttachActionPullRequestNoOps(t *testing.T) {
	t.Parallel()

	planID := uuid.New()
	execID := uuid.New()
	dir := t.TempDir()
	svc := &ChatService{actionPlansDir: dir}

	if err := writeAttachTestPlan(dir, planID); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	t.Run("empty pr url", func(t *testing.T) {
		exec := domain.Dialog{ID: execID, ParentID: &planID}
		if err := svc.attachActionPullRequest(context.Background(), exec, ""); err != nil {
			t.Fatalf("attachActionPullRequest: %v", err)
		}
	})

	t.Run("no parent", func(t *testing.T) {
		exec := domain.Dialog{ID: execID}
		if err := svc.attachActionPullRequest(context.Background(), exec, "https://github.com/org/repo/pull/1"); err != nil {
			t.Fatalf("attachActionPullRequest: %v", err)
		}
	})

	t.Run("unknown run", func(t *testing.T) {
		exec := domain.Dialog{ID: execID, ParentID: &planID}
		if err := svc.attachActionPullRequest(context.Background(), exec, "https://github.com/org/repo/pull/1"); err != nil {
			t.Fatalf("attachActionPullRequest: %v", err)
		}
		raw, _, err := svc.ReadActionPlan(planID)
		if err != nil {
			t.Fatalf("ReadActionPlan: %v", err)
		}
		if string(raw) == "" {
			t.Fatal("plan missing")
		}
		var plan map[string]any
		if err := json.Unmarshal(raw, &plan); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		stages := plan["stages"].([]any)
		steps := stages[0].(map[string]any)["steps"].([]any)
		for _, s := range steps {
			if _, ok := s.(map[string]any)["pr_url"]; ok {
				t.Fatal("no step should have pr_url")
			}
		}
	})
}

func TestAppendAgentResultAttachesPRAndIsIdempotent(t *testing.T) {
	t.Parallel()

	planID := uuid.New()
	execID := uuid.New()
	prURL := "https://github.com/org/repo/pull/99"
	jobName := "nib-12345678"

	dir := t.TempDir()
	repo := &mapDialogRepo{
		dialogs: map[uuid.UUID]domain.Dialog{
			execID: {ID: execID, Mode: "execute", ParentID: &planID},
		},
	}
	svc := &ChatService{
		dialogRepo:     repo,
		actionPlansDir: dir,
		activity:       NewActivityBroker(),
	}

	if err := writeAttachTestPlan(dir, planID); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	if err := svc.WriteActionPlanRuns(planID, map[string]string{"s0.step0": execID.String()}); err != nil {
		t.Fatalf("write runs: %v", err)
	}

	res := AgentRunResult{
		Status:   "success",
		Result:   "done",
		ChatID:   execID.String(),
		JobName:  jobName,
		PRURL:    prURL,
	}

	_, err := svc.AppendAgentResult(context.Background(), execID, res)
	if err != nil {
		t.Fatalf("AppendAgentResult first: %v", err)
	}

	raw, _, err := svc.ReadActionPlan(planID)
	if err != nil {
		t.Fatalf("ReadActionPlan: %v", err)
	}
	var plan map[string]any
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	steps := plan["stages"].([]any)[0].(map[string]any)["steps"].([]any)
	step0 := steps[0].(map[string]any)
	if step0["pr_url"] != prURL {
		t.Fatalf("pr_url = %v, want %q", step0["pr_url"], prURL)
	}

	_, err = svc.AppendAgentResult(context.Background(), execID, res)
	if err != nil {
		t.Fatalf("AppendAgentResult duplicate: %v", err)
	}

	raw2, _, err := svc.ReadActionPlan(planID)
	if err != nil {
		t.Fatalf("ReadActionPlan after duplicate: %v", err)
	}
	if string(raw2) != string(raw) {
		t.Fatalf("plan changed on duplicate delivery")
	}
}

func TestRecordActionPlanRun(t *testing.T) {
	t.Parallel()

	planID := uuid.New()
	execID := uuid.New()
	dir := t.TempDir()
	svc := &ChatService{actionPlansDir: dir}

	if err := svc.recordActionPlanRun(planID, "s0.step1", execID); err != nil {
		t.Fatalf("recordActionPlanRun: %v", err)
	}

	runs, err := svc.ReadActionPlanRuns(planID)
	if err != nil {
		t.Fatalf("ReadActionPlanRuns: %v", err)
	}
	if runs["s0.step1"] != execID.String() {
		t.Fatalf("runs[s0.step1] = %q, want %q", runs["s0.step1"], execID.String())
	}

	execID2 := uuid.New()
	if err := svc.recordActionPlanRun(planID, "s0.step1", execID2); err != nil {
		t.Fatalf("recordActionPlanRun overwrite: %v", err)
	}
	runs, err = svc.ReadActionPlanRuns(planID)
	if err != nil {
		t.Fatalf("ReadActionPlanRuns: %v", err)
	}
	if runs["s0.step1"] != execID2.String() {
		t.Fatalf("runs[s0.step1] = %q, want %q", runs["s0.step1"], execID2.String())
	}
}

func TestSetActionPlanStepPRURLRollback(t *testing.T) {
	t.Parallel()

	plan := map[string]any{
		"rollback": []any{
			map[string]any{"type": "code", "action": "revert"},
		},
	}
	changed, err := setActionPlanStepPRURL(plan, "rollback.0", "https://github.com/org/repo/pull/7")
	if err != nil {
		t.Fatalf("setActionPlanStepPRURL: %v", err)
	}
	if !changed {
		t.Fatal("expected change")
	}
	step := plan["rollback"].([]any)[0].(map[string]any)
	if step["pr_url"] != "https://github.com/org/repo/pull/7" {
		t.Fatalf("pr_url = %v", step["pr_url"])
	}
}
