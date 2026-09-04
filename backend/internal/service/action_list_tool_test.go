package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

type actionListDialogRepo struct {
	dialogs map[uuid.UUID]domain.Dialog
}

func (r *actionListDialogRepo) CreateDialog(_ context.Context, _, _ string, _ *uuid.UUID) (domain.Dialog, error) {
	return domain.Dialog{}, nil
}

func (r *actionListDialogRepo) ListChildren(_ context.Context, _ uuid.UUID) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *actionListDialogRepo) ListPlanDialogIDs(_ context.Context) ([]uuid.UUID, error) {
	return nil, nil
}

func (r *actionListDialogRepo) ListRecentDialogs(_ context.Context, _, _ int) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *actionListDialogRepo) CountDialogs(_ context.Context) (int, error) {
	return 0, nil
}

func (r *actionListDialogRepo) ListDialogsByMode(_ context.Context, _ string, _, _ int) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *actionListDialogRepo) CountDialogsByMode(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (r *actionListDialogRepo) ListAllRecentDialogs(_ context.Context, _, _ int) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *actionListDialogRepo) CountAllDialogs(_ context.Context) (int, error) {
	return 0, nil
}

func (r *actionListDialogRepo) SearchDialogs(_ context.Context, _, _ string, _, _ int) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *actionListDialogRepo) CountDialogsSearch(_ context.Context, _, _ string) (int, error) {
	return 0, nil
}

func (r *actionListDialogRepo) GetDialog(_ context.Context, id uuid.UUID) (domain.Dialog, error) {
	d, ok := r.dialogs[id]
	if !ok {
		return domain.Dialog{}, nil
	}
	return d, nil
}

func (r *actionListDialogRepo) UpdateTitle(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}

func (r *actionListDialogRepo) SetDialogTaskID(_ context.Context, _ uuid.UUID, _ *string) error {
	return nil
}

func (r *actionListDialogRepo) SetDialogCategories(_ context.Context, _ uuid.UUID, _ []string) error {
	return nil
}

func (r *actionListDialogRepo) SetDialogPinned(_ context.Context, _ uuid.UUID, _ bool) error {
	return nil
}

func (r *actionListDialogRepo) ListPinnedDialogs(_ context.Context) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *actionListDialogRepo) DeleteDialog(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (r *actionListDialogRepo) ListMessages(_ context.Context, _ uuid.UUID) ([]domain.DialogMessage, error) {
	return nil, nil
}

func (r *actionListDialogRepo) AppendMessage(_ context.Context, _ uuid.UUID, _ domain.DialogMessage) (domain.DialogMessage, error) {
	return domain.DialogMessage{}, nil
}

func (r *actionListDialogRepo) DeleteMessagesAfterSeq(_ context.Context, _ uuid.UUID, _ int) error {
	return nil
}

func sampleStoredActionPlan() []byte {
	plan := map[string]any{
		"stages": []any{
			map[string]any{
				"number":      1,
				"title":       "Deploy",
				"description": "Deploy service",
				"steps": []any{
					map[string]any{"type": "code", "action": "Update Helm values"},
					map[string]any{"type": "shell", "action": "kubectl apply -f deploy.yaml"},
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
	data, _ := json.Marshal(plan)
	return data
}

func TestGetActionListToolDef(t *testing.T) {
	t.Parallel()
	def := GetActionListToolDef()
	if def.Name != GetActionListToolName {
		t.Fatalf("name = %q, want %q", def.Name, GetActionListToolName)
	}
}

func TestGetActionListHandler_viaParentDialog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()

	planID := uuid.New()
	executeID := uuid.New()

	planPath := filepath.Join(dir, planID.String()+".json")
	if err := os.WriteFile(planPath, sampleStoredActionPlan(), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	if err := os.WriteFile(actionPlanChecksPath(dir, planID), []byte(`["s0.step0","rollback.0"]`), 0o644); err != nil {
		t.Fatalf("write checks: %v", err)
	}

	svc := &ChatService{
		actionPlansDir: dir,
		dialogRepo: &actionListDialogRepo{
			dialogs: map[uuid.UUID]domain.Dialog{
				executeID: {ID: executeID, ParentID: &planID, Mode: "execute"},
			},
		},
	}

	out, err := svc.getActionListHandler(executeID)(ctx, nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	var resp actionListResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nout=%s", err, out)
	}
	if len(resp.Stages) != 1 {
		t.Fatalf("stages = %d, want 1", len(resp.Stages))
	}
	if resp.Stages[0].Number != 1 || resp.Stages[0].Title != "Deploy" {
		t.Fatalf("stage = %#v", resp.Stages[0])
	}
	if len(resp.Stages[0].Actions) != 2 {
		t.Fatalf("actions = %d, want 2", len(resp.Stages[0].Actions))
	}
	if resp.Stages[0].Actions[0].Number != "1.1" || !resp.Stages[0].Actions[0].Executed {
		t.Fatalf("first action = %#v", resp.Stages[0].Actions[0])
	}
	if resp.Stages[0].Actions[1].Number != "1.2" || resp.Stages[0].Actions[1].Executed {
		t.Fatalf("second action = %#v", resp.Stages[0].Actions[1])
	}
	if len(resp.Stages[0].Checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(resp.Stages[0].Checks))
	}
	if resp.Stages[0].Checks[0].Number != "1.C1" || resp.Stages[0].Checks[0].Executed {
		t.Fatalf("check = %#v", resp.Stages[0].Checks[0])
	}
	if len(resp.Rollback) != 1 {
		t.Fatalf("rollback = %d, want 1", len(resp.Rollback))
	}
	if resp.Rollback[0].Number != "R1" || !resp.Rollback[0].Executed {
		t.Fatalf("rollback = %#v", resp.Rollback[0])
	}
}

func TestGetActionListHandler_planDialogDirect(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	planID := uuid.New()

	if err := os.WriteFile(filepath.Join(dir, planID.String()+".json"), sampleStoredActionPlan(), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	svc := &ChatService{
		actionPlansDir: dir,
		dialogRepo: &actionListDialogRepo{
			dialogs: map[uuid.UUID]domain.Dialog{
				planID: {ID: planID, Mode: "plan"},
			},
		},
	}

	out, err := svc.getActionListHandler(planID)(ctx, nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out == "no action plan found for this dialog" {
		t.Fatal("expected plan to be found on plan dialog")
	}
}

func TestGetActionListHandler_noPlan(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	executeID := uuid.New()
	planID := uuid.New()

	svc := &ChatService{
		actionPlansDir: t.TempDir(),
		dialogRepo: &actionListDialogRepo{
			dialogs: map[uuid.UUID]domain.Dialog{
				executeID: {ID: executeID, ParentID: &planID, Mode: "execute"},
			},
		},
	}

	out, err := svc.getActionListHandler(executeID)(ctx, nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out != "no action plan found for this dialog" {
		t.Fatalf("out = %q", out)
	}
}

func TestBuildActionListResponse(t *testing.T) {
	t.Parallel()
	plan := storedActionPlan{
		Stages: []storedActionStage{
			{
				Number: 1,
				Title:  "Stage",
				Steps: []storedActionStep{
					{Type: "shell", Action: "run", Command: "kubectl -n prod get pods"},
					{Type: "code", Action: "edit chart", PRTitle: "chore: bump chart version"},
				},
				Checks: []storedActionCheck{
					{Check: "ok", Expectation: "yes"},
				},
			},
		},
		Rollback: []storedActionStep{
			{Type: "code", Action: "revert", PRTitle: "chore: revert chart bump"},
			{Type: "shell", Action: "restart", Command: "kubectl -n prod rollout undo deployment/api"},
		},
	}
	checked := map[string]struct{}{"s0.step0": {}}

	resp := buildActionListResponse(plan, checked, ActionExecRuns{})
	if len(resp.Stages) != 1 || len(resp.Stages[0].Actions) != 2 || !resp.Stages[0].Actions[0].Executed {
		t.Fatalf("actions = %#v", resp.Stages[0].Actions)
	}
	if resp.Stages[0].Actions[1].PRTitle != "chore: bump chart version" {
		t.Fatalf("code step pr_title = %q", resp.Stages[0].Actions[1].PRTitle)
	}
	if resp.Stages[0].Actions[0].Command != "kubectl -n prod get pods" {
		t.Fatalf("shell step command = %q", resp.Stages[0].Actions[0].Command)
	}
	if resp.Stages[0].Actions[1].Command != "" {
		t.Fatalf("code step command = %q, want empty", resp.Stages[0].Actions[1].Command)
	}
	if resp.Stages[0].Checks[0].Executed {
		t.Fatalf("check should not be executed: %#v", resp.Stages[0].Checks[0])
	}
	if resp.Stages[0].Number != 1 {
		t.Fatalf("stage number = %d, want 1", resp.Stages[0].Number)
	}
	if got := resp.Stages[0].Actions[1].Number; got != "1.2" {
		t.Fatalf("second action number = %q, want 1.2", got)
	}
	if got := resp.Stages[0].Checks[0].Number; got != "1.C1" {
		t.Fatalf("check number = %q, want 1.C1", got)
	}
	if got := resp.Rollback[1].Number; got != "R2" {
		t.Fatalf("second rollback number = %q, want R2", got)
	}
	if resp.Rollback[0].Executed {
		t.Fatalf("rollback should not be executed: %#v", resp.Rollback[0])
	}
	if resp.Rollback[0].PRTitle != "chore: revert chart bump" {
		t.Fatalf("rollback pr_title = %q", resp.Rollback[0].PRTitle)
	}
	if resp.Rollback[1].Command != "kubectl -n prod rollout undo deployment/api" {
		t.Fatalf("rollback command = %q", resp.Rollback[1].Command)
	}
}

func TestAddLocalTools_includesGetActionListForDialog(t *testing.T) {
	t.Parallel()
	svc := &ChatService{dialogRepo: &actionListDialogRepo{}}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{GetActionListToolName: {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[GetActionListToolName]; !ok {
		t.Fatal("expected get_action_list handler")
	}
	found := false
	for _, def := range catalog.tools {
		if def.Name == GetActionListToolName {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected get_action_list in tool defs")
	}
}

func TestAddLocalTools_excludesGetActionListWithoutDialogRepo(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{GetActionListToolName: {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[GetActionListToolName]; ok {
		t.Fatal("did not expect get_action_list handler without dialog repo")
	}
}

func TestAddLocalTools_excludesGetActionListWithoutAllowList(t *testing.T) {
	t.Parallel()
	svc := &ChatService{dialogRepo: &actionListDialogRepo{}}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{"other_tool": {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[GetActionListToolName]; ok {
		t.Fatal("did not expect get_action_list handler when not in allow list")
	}
}
