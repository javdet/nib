package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/toolcatalog"
	"github.com/google/uuid"
)

type allowSetDialogRepo struct {
	dialogs map[uuid.UUID]domain.Dialog
}

func (r *allowSetDialogRepo) CreateDialog(context.Context, string, string, *uuid.UUID) (domain.Dialog, error) {
	return domain.Dialog{}, nil
}
func (r *allowSetDialogRepo) ListRecentDialogs(context.Context, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *allowSetDialogRepo) CountDialogs(context.Context) (int, error) { return 0, nil }
func (r *allowSetDialogRepo) ListDialogsByMode(context.Context, string, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *allowSetDialogRepo) CountDialogsByMode(context.Context, string) (int, error) { return 0, nil }
func (r *allowSetDialogRepo) ListAllRecentDialogs(context.Context, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *allowSetDialogRepo) CountAllDialogs(context.Context) (int, error) { return 0, nil }
func (r *allowSetDialogRepo) SearchDialogs(context.Context, string, string, int, int) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *allowSetDialogRepo) CountDialogsSearch(context.Context, string, string) (int, error) {
	return 0, nil
}
func (r *allowSetDialogRepo) ListChildren(context.Context, uuid.UUID) ([]domain.Dialog, error) {
	return nil, nil
}

func (r *allowSetDialogRepo) ListPlanDialogIDs(_ context.Context) ([]uuid.UUID, error) {
	return nil, nil
}
func (r *allowSetDialogRepo) GetDialog(_ context.Context, id uuid.UUID) (domain.Dialog, error) {
	d, ok := r.dialogs[id]
	if !ok {
		return domain.Dialog{}, nil
	}
	return d, nil
}
func (r *allowSetDialogRepo) UpdateTitle(context.Context, uuid.UUID, string) error { return nil }
func (r *allowSetDialogRepo) SetDialogTaskID(context.Context, uuid.UUID, *string) error {
	return nil
}

func (r *allowSetDialogRepo) SetDialogCategories(context.Context, uuid.UUID, []string) error {
	return nil
}
func (r *allowSetDialogRepo) SetDialogPinned(context.Context, uuid.UUID, bool) error { return nil }
func (r *allowSetDialogRepo) ListPinnedDialogs(context.Context) ([]domain.Dialog, error) {
	return nil, nil
}
func (r *allowSetDialogRepo) DeleteDialog(context.Context, uuid.UUID) error { return nil }
func (r *allowSetDialogRepo) ListMessages(context.Context, uuid.UUID) ([]domain.DialogMessage, error) {
	return nil, nil
}
func (r *allowSetDialogRepo) AppendMessage(context.Context, uuid.UUID, domain.DialogMessage) (domain.DialogMessage, error) {
	return domain.DialogMessage{}, nil
}
func (r *allowSetDialogRepo) DeleteMessagesAfterSeq(context.Context, uuid.UUID, int) error {
	return nil
}

func TestResolveDialogAllowSet(t *testing.T) {
	allowDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(allowDir, "plan.json"), []byte(`{"allow_tools":["ask_question"]}`), 0o644); err != nil {
		t.Fatalf("write plan allow list: %v", err)
	}
	if err := os.WriteFile(filepath.Join(allowDir, "decompose.json"), []byte(`{"allow_tools":["set_category"]}`), 0o644); err != nil {
		t.Fatalf("write decompose allow list: %v", err)
	}

	parentID := uuid.New()
	planID := uuid.New()
	repo := &allowSetDialogRepo{
		dialogs: map[uuid.UUID]domain.Dialog{
			parentID: {
				ID:         parentID,
				Mode:       "decompose",
				Categories: []string{"logging"},
			},
			planID: {
				ID:       planID,
				Mode:     "plan",
				ParentID: &parentID,
			},
		},
	}
	lister := &stubToolCategoryLister{
		byCategory: map[string][]toolcatalog.CatalogTool{
			"logging": {
				{Name: "ask_question"},
				{Name: "grafana_query_loki_logs"},
			},
		},
	}

	svc := &ChatService{
		dialogRepo:    repo,
		toolCategorySvc: lister,
		allowToolsDir: allowDir,
	}

	tests := []struct {
		name     string
		dialog   domain.Dialog
		want     []string
		notWant  []string
	}{
		{
			name:   "plan inherits parent categories",
			dialog: repo.dialogs[planID],
			want:   []string{"ask_question", "grafana_query_loki_logs"},
		},
		{
			name:   "decompose unchanged",
			dialog: repo.dialogs[parentID],
			want:   []string{"set_category"},
			notWant: []string{"grafana_query_loki_logs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allow, err := svc.resolveDialogAllowSet(context.Background(), tt.dialog)
			if err != nil {
				t.Fatalf("resolveDialogAllowSet: %v", err)
			}
			for _, name := range tt.want {
				if _, ok := allow[name]; !ok {
					t.Fatalf("expected %q in allow set, got %v", name, allow)
				}
			}
			for _, name := range tt.notWant {
				if _, ok := allow[name]; ok {
					t.Fatalf("did not expect %q in allow set, got %v", name, allow)
				}
			}
		})
	}
}

func TestResolveDialogAllowSet_MergesOverlappingTools(t *testing.T) {
	allowDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(allowDir, "plan.json"), []byte(`{"allow_tools":["ask_question"]}`), 0o644); err != nil {
		t.Fatalf("write plan allow list: %v", err)
	}

	parentID := uuid.New()
	planID := uuid.New()
	repo := &allowSetDialogRepo{
		dialogs: map[uuid.UUID]domain.Dialog{
			parentID: {ID: parentID, Mode: "decompose", Categories: []string{"monitoring"}},
			planID:   {ID: planID, Mode: "plan", ParentID: &parentID},
		},
	}
	lister := &stubToolCategoryLister{
		byCategory: map[string][]toolcatalog.CatalogTool{
			"monitoring": {
				{Name: "ask_question"},
				{Name: "grafana_list_dashboards"},
			},
		},
	}
	svc := &ChatService{
		dialogRepo:      repo,
		toolCategorySvc: lister,
		allowToolsDir:   allowDir,
	}

	allow, err := svc.resolveDialogAllowSet(context.Background(), repo.dialogs[planID])
	if err != nil {
		t.Fatalf("resolveDialogAllowSet: %v", err)
	}
	if len(allow) != 2 {
		t.Fatalf("expected 2 unique tools, got %d: %v", len(allow), allow)
	}
}

func TestResolveDialogAllowSet_ListToolsErrorIsNonFatal(t *testing.T) {
	allowDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(allowDir, "plan.json"), []byte(`{"allow_tools":["ask_question"]}`), 0o644); err != nil {
		t.Fatalf("write plan allow list: %v", err)
	}

	parentID := uuid.New()
	planID := uuid.New()
	repo := &allowSetDialogRepo{
		dialogs: map[uuid.UUID]domain.Dialog{
			parentID: {ID: parentID, Mode: "decompose", Categories: []string{"logging"}},
			planID:   {ID: planID, Mode: "plan", ParentID: &parentID},
		},
	}
	lister := &stubToolCategoryLister{
		toolsErr: context.Canceled,
	}
	svc := &ChatService{
		dialogRepo:      repo,
		toolCategorySvc: lister,
		allowToolsDir:   allowDir,
	}

	allow, err := svc.resolveDialogAllowSet(context.Background(), repo.dialogs[planID])
	if err != nil {
		t.Fatalf("resolveDialogAllowSet: %v", err)
	}
	if _, ok := allow["ask_question"]; !ok {
		t.Fatalf("expected base allow tool to remain, got %v", allow)
	}
}

// An action subagent gets the categories of the one action it owns, not the ones
// decomposition chose for the plan, and an action carrying none gets no MCP
// tools at all.
func TestActionAgentAllowSet_UsesStepCategories(t *testing.T) {
	allowDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(allowDir, "execute.json"),
		[]byte(`{"allow_tools":["tool_search","execute_command"]}`), 0o644); err != nil {
		t.Fatalf("write execute allow list: %v", err)
	}

	lister := &stubToolCategoryLister{
		byCategory: map[string][]toolcatalog.CatalogTool{
			"kubernetes":    {{Name: "kubernetes_resources_get"}},
			"issue-tracker": {{Name: "jira_search"}},
		},
	}
	svc := &ChatService{toolCategorySvc: lister, allowToolsDir: allowDir}

	tests := []struct {
		name       string
		categories []string
		want       []string
		notWant    []string
	}{
		{
			name:       "only the action's own categories",
			categories: []string{"kubernetes"},
			want:       []string{"tool_search", "execute_command", "kubernetes_resources_get"},
			notWant:    []string{"jira_search"},
		},
		{
			name:       "no categories means no category tools",
			categories: nil,
			want:       []string{"tool_search", "execute_command"},
			notWant:    []string{"kubernetes_resources_get", "jira_search"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allow, err := svc.actionAgentAllowSet(context.Background(), tt.categories)
			if err != nil {
				t.Fatalf("actionAgentAllowSet: %v", err)
			}
			for _, name := range tt.want {
				if _, ok := allow[name]; !ok {
					t.Fatalf("expected %q in allow set, got %v", name, allow)
				}
			}
			for _, name := range tt.notWant {
				if _, ok := allow[name]; ok {
					t.Fatalf("did not expect %q in allow set, got %v", name, allow)
				}
			}
		})
	}
}

// A follow-up the operator types into a finished action subagent's transcript
// has to rebuild the same catalog the subagent had, which means resolving the
// dialog back to its action through runs.json.
func TestResolveDialogAllowSet_ExecuteDialogResolvesItsAction(t *testing.T) {
	allowDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(allowDir, "execute.json"),
		[]byte(`{"allow_tools":["tool_search"]}`), 0o644); err != nil {
		t.Fatalf("write execute allow list: %v", err)
	}

	planID := uuid.New()
	execID := uuid.New()

	plansDir := t.TempDir()
	plan := `{"stages":[{"title":"Deploy","steps":[
		{"type":"shell","action":"restart","categories":["kubernetes"]}
	],"checks":[]}],"rollback":[]}`
	if err := os.WriteFile(filepath.Join(plansDir, planID.String()+".json"), []byte(plan), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	runs := `{"s0.step0":"` + execID.String() + `"}`
	if err := os.WriteFile(filepath.Join(plansDir, planID.String()+".runs.json"), []byte(runs), 0o644); err != nil {
		t.Fatalf("write runs: %v", err)
	}

	lister := &stubToolCategoryLister{
		byCategory: map[string][]toolcatalog.CatalogTool{
			"kubernetes": {{Name: "kubernetes_resources_get"}},
		},
	}
	svc := &ChatService{
		toolCategorySvc: lister,
		allowToolsDir:   allowDir,
		actionPlansDir:  plansDir,
	}

	allow, err := svc.resolveDialogAllowSet(context.Background(), domain.Dialog{
		ID:       execID,
		Mode:     "execute",
		ParentID: &planID,
	})
	if err != nil {
		t.Fatalf("resolveDialogAllowSet: %v", err)
	}
	for _, name := range []string{"tool_search", "kubernetes_resources_get"} {
		if _, ok := allow[name]; !ok {
			t.Fatalf("expected %q in allow set, got %v", name, allow)
		}
	}
}

// An execute dialog nothing registered in runs.json -- one an operator opened
// themselves -- must not fail the turn, it just gets no category tools.
func TestResolveDialogAllowSet_ExecuteDialogWithoutRun(t *testing.T) {
	allowDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(allowDir, "execute.json"),
		[]byte(`{"allow_tools":["tool_search"]}`), 0o644); err != nil {
		t.Fatalf("write execute allow list: %v", err)
	}

	planID := uuid.New()
	svc := &ChatService{
		toolCategorySvc: &stubToolCategoryLister{},
		allowToolsDir:   allowDir,
		actionPlansDir:  t.TempDir(),
	}

	allow, err := svc.resolveDialogAllowSet(context.Background(), domain.Dialog{
		ID:       uuid.New(),
		Mode:     "execute",
		ParentID: &planID,
	})
	if err != nil {
		t.Fatalf("resolveDialogAllowSet: %v", err)
	}
	if len(allow) != 1 {
		t.Fatalf("expected only the mode allow list, got %v", allow)
	}
}
