package service

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

func TestEnforceModeToolLimits_incidentTools(t *testing.T) {
	t.Parallel()

	everything := func() map[string]struct{} {
		allow := make(map[string]struct{})
		for _, name := range append(append([]string{}, incidentOnlyTools...), incidentWithheldTools...) {
			allow[name] = struct{}{}
		}
		allow[KnowledgeSearchToolName] = struct{}{}
		return allow
	}

	for _, modeName := range []string{"main", "plan", "execute", "discuss", "decompose"} {
		allow := everything()
		enforceModeToolLimits(modeName, allow)
		for _, name := range incidentOnlyTools {
			if _, ok := allow[name]; ok {
				t.Errorf("%s keeps incident-only %s", modeName, name)
			}
		}
	}

	allow := everything()
	enforceModeToolLimits(incidentDialogMode, allow)
	for _, name := range incidentWithheldTools {
		if _, ok := allow[name]; ok {
			t.Errorf("incident keeps withheld %s", name)
		}
	}
	for _, name := range append([]string{KnowledgeSearchToolName}, incidentOnlyTools...) {
		if _, ok := allow[name]; !ok {
			t.Errorf("incident lost %s", name)
		}
	}
}

func catalogToolNames(c *toolCatalog) []string {
	names := make([]string, 0, len(c.tools))
	for _, tool := range c.tools {
		names = append(names, tool.Name)
	}
	return names
}

func TestPostmortemTools_registeredOnlyForIncidentWriters(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	root := uuid.New()
	allow := map[string]struct{}{GetPostmortemToolName: {}, UpdatePostmortemToolName: {}}

	tests := []struct {
		name       string
		binding    toolBinding
		wantGet    bool
		wantUpdate bool
	}{
		{"incident orchestrator", toolBinding{dialogID: root, planID: root, mode: incidentDialogMode, incidentRole: incidentRoleOrchestrator}, true, true},
		{"synthesis agent", toolBinding{dialogID: uuid.New(), planID: root, mode: incidentDialogMode, incidentRole: incidentRoleSynthesis}, true, true},
		{"digger", toolBinding{dialogID: uuid.New(), planID: root, mode: incidentDialogMode, incidentRole: incidentRoleDigger, system: "api"}, true, false},
		{"follow-up in a sub-agent transcript", toolBinding{dialogID: uuid.New(), planID: root, mode: incidentDialogMode}, true, false},
		{"discuss turn", toolBinding{dialogID: root, planID: root, mode: discussDialogMode, incidentRole: incidentRoleOrchestrator}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			catalog := newToolCatalog()
			svc.addLocalTools(catalog, allow, tt.binding)
			names := catalogToolNames(catalog)
			if got := slices.Contains(names, GetPostmortemToolName); got != tt.wantGet {
				t.Errorf("get_postmortem registered = %v, want %v", got, tt.wantGet)
			}
			if got := slices.Contains(names, UpdatePostmortemToolName); got != tt.wantUpdate {
				t.Errorf("update_postmortem registered = %v, want %v", got, tt.wantUpdate)
			}
		})
	}
}

func TestDialogToolBinding_incidentRootIsTheOrchestrator(t *testing.T) {
	t.Parallel()
	repo := newMultiDialogRepo()
	svc := &ChatService{dialogRepo: repo}
	ctx := context.Background()

	root := repo.add(domain.Dialog{Mode: incidentDialogMode})
	child, _ := repo.CreateDialog(ctx, incidentDialogMode, "digger", &root)
	plan := repo.add(domain.Dialog{Mode: "main"})

	rootDialog, _ := repo.GetDialog(ctx, root)
	if b := svc.dialogToolBinding(ctx, rootDialog); b.incidentRole != incidentRoleOrchestrator {
		t.Fatalf("root role = %q, want orchestrator", b.incidentRole)
	}
	if b := svc.dialogToolBinding(ctx, child); b.incidentRole != "" || b.planID != root {
		t.Fatalf("child binding = role %q plan %s, want no role and the incident as plan", b.incidentRole, b.planID)
	}
	planDialog, _ := repo.GetDialog(ctx, plan)
	if b := svc.dialogToolBinding(ctx, planDialog); b.incidentRole != "" {
		t.Fatalf("plan role = %q, want none", b.incidentRole)
	}
}

func TestIncidentStepAllowSet(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	ctx := context.Background()

	tests := []struct {
		kind    IncidentStepKind
		want    []string
		without []string
	}{
		{
			IncidentStepDigger,
			[]string{ExecuteCommandToolName, APICallToolName, "tool_search", ReportFindingToolName},
			[]string{UpdatePostmortemToolName, AskQuestionToolName, RunInvestigationToolName, RunExecutorToolName},
		},
		{
			IncidentStepImpact,
			[]string{APICallToolName, GetPostmortemToolName, UpdatePostmortemToolName},
			[]string{ReportFindingToolName, AskQuestionToolName},
		},
		{
			IncidentStepSynthesis,
			[]string{GetPostmortemToolName, UpdatePostmortemToolName, KnowledgeSearchToolName},
			[]string{ExecuteCommandToolName, APICallToolName, ReportFindingToolName},
		},
		{
			IncidentStepClosure,
			[]string{GetPostmortemToolName, UpdatePostmortemToolName},
			[]string{ExecuteCommandToolName, KnowledgeSearchToolName},
		},
	}
	for _, tt := range tests {
		allow := svc.incidentStepAllowSet(ctx, tt.kind, nil)
		for _, name := range tt.want {
			if _, ok := allow[name]; !ok {
				t.Errorf("%s lacks %s", tt.kind, name)
			}
		}
		for _, name := range tt.without {
			if _, ok := allow[name]; ok {
				t.Errorf("%s has %s", tt.kind, name)
			}
		}
	}

	follow := svc.incidentFollowUpAllowSet(ctx, domain.Dialog{Mode: incidentDialogMode})
	for _, name := range []string{ReportFindingToolName, UpdatePostmortemToolName} {
		if _, ok := follow[name]; ok {
			t.Errorf("follow-up allow set has writer %s", name)
		}
	}
}

func TestInvestigationTools_registration(t *testing.T) {
	t.Parallel()
	svc := &ChatService{dialogRepo: newMultiDialogRepo()}
	root := uuid.New()
	allow := map[string]struct{}{RunInvestigationToolName: {}, ReportFindingToolName: {}, RunSubagentToolName: {}}

	tests := []struct {
		name       string
		binding    toolBinding
		wantRun    bool
		wantReport bool
	}{
		{"incident root", toolBinding{dialogID: root, planID: root, mode: incidentDialogMode, incidentRole: incidentRoleOrchestrator}, true, false},
		{"digger", toolBinding{dialogID: uuid.New(), planID: root, mode: incidentDialogMode, incidentRole: incidentRoleDigger, system: "api"}, false, true},
		{"synthesis", toolBinding{dialogID: uuid.New(), planID: root, mode: incidentDialogMode, incidentRole: incidentRoleSynthesis}, false, false},
		{"main root", toolBinding{dialogID: root, planID: root, mode: mainDialogMode}, false, false},
	}
	for _, tt := range tests {
		catalog := newToolCatalog()
		svc.addLocalTools(catalog, allow, tt.binding)
		names := catalogToolNames(catalog)
		if got := slices.Contains(names, RunInvestigationToolName); got != tt.wantRun {
			t.Errorf("%s: run_investigation = %v, want %v", tt.name, got, tt.wantRun)
		}
		if got := slices.Contains(names, ReportFindingToolName); got != tt.wantReport {
			t.Errorf("%s: report_finding = %v, want %v", tt.name, got, tt.wantReport)
		}
	}

	stripped := stripSubagentTools(map[string]struct{}{RunInvestigationToolName: {}})
	if _, ok := stripped[RunInvestigationToolName]; ok {
		t.Error("stripSubagentTools keeps run_investigation")
	}
}

func TestIncidentOverlays_digger(t *testing.T) {
	t.Parallel()
	prompt, err := promptTestService(t).incidentStepPrompt(context.Background(), "incident_digger")
	if err != nil {
		t.Fatalf("incidentStepPrompt() err = %v", err)
	}
	for _, want := range []string{"## Environment", "Read-only", "report_finding"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("digger prompt lacks %q", want)
		}
	}
}
