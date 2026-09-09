package service

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/mode"
	"github.com/javdet/nib/internal/rules"
	"github.com/javdet/nib/internal/systemprompts"
)

func writeTestRules(t *testing.T, files map[string]string) (dataDir string, svc *rules.Service) {
	t.Helper()

	dataDir = t.TempDir()
	ruleDir := filepath.Join(dataDir, "rules")
	if err := os.MkdirAll(ruleDir, 0o755); err != nil {
		t.Fatalf("mkdir rules: %v", err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(ruleDir, name+".md"), []byte(content), 0o644); err != nil {
			t.Fatalf("write rule %s: %v", name, err)
		}
	}
	return dataDir, rules.NewService(dataDir, "rules")
}

func TestGetRuleToolDef(t *testing.T) {
	t.Parallel()

	def := GetRuleToolDef()
	if def.Name != GetRuleToolName {
		t.Fatalf("Name = %q, want %q", def.Name, GetRuleToolName)
	}
	if def.Description == "" {
		t.Fatal("Description is empty")
	}
	if len(def.Parameters) == 0 {
		t.Fatal("Parameters is empty")
	}
}

func TestExecuteGetRule(t *testing.T) {
	t.Parallel()

	_, rulesSvc := writeTestRules(t, map[string]string{
		"postgres": "---\nname: postgres\ndescription: How we run Postgres\n---\n\nnever expose 5432 publicly",
	})
	chatSvc := &ChatService{rulesSvc: rulesSvc}

	out, err := chatSvc.ExecuteGetRule(context.Background(), map[string]any{"name": "postgres"})
	if err != nil {
		t.Fatalf("ExecuteGetRule() error = %v", err)
	}
	if !strings.Contains(out, "never expose 5432 publicly") {
		t.Fatalf("output missing body: %s", out)
	}
}

func TestExecuteGetRuleNotFound(t *testing.T) {
	t.Parallel()

	_, rulesSvc := writeTestRules(t, nil)
	chatSvc := &ChatService{rulesSvc: rulesSvc}

	_, err := chatSvc.ExecuteGetRule(context.Background(), map[string]any{"name": "missing"})
	if err == nil {
		t.Fatal("expected error for missing rule")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v, want not found", err)
	}
}

// A rule body is a template like every other prompt fragment: the tool has to
// render it before the agent sees it, or the agent reads the braces.
func TestExecuteGetRuleRendersTemplateVariables(t *testing.T) {
	t.Parallel()

	_, rulesSvc := writeTestRules(t, map[string]string{
		"postgres": "Company: {{ .global.CompanyName }}, project: {{ .builtin.Project }}",
	})
	selection := NewSelectionStore()
	selection.Set(domain.Selection{Project: "demo", Environment: "any", Cloud: "any", Location: "any"})

	chatSvc := &ChatService{
		rulesSvc: rulesSvc,
		variableRepo: &stubVariableRepo{vars: map[string]map[string]any{
			"global": {"CompanyName": "AutomagicOps"},
		}},
		selection: selection,
	}

	out, err := chatSvc.ExecuteGetRule(context.Background(), map[string]any{"name": "postgres"})
	if err != nil {
		t.Fatalf("ExecuteGetRule() error = %v", err)
	}
	if want := "Company: AutomagicOps, project: demo"; out != want {
		t.Fatalf("rendered = %q, want %q", out, want)
	}
}

func TestExecuteGetRulePlainContentUnchanged(t *testing.T) {
	t.Parallel()

	const plain = "- Postgres must listen on all interfaces"
	_, rulesSvc := writeTestRules(t, map[string]string{"postgres": plain})
	chatSvc := &ChatService{rulesSvc: rulesSvc}

	out, err := chatSvc.ExecuteGetRule(context.Background(), map[string]any{"name": "postgres"})
	if err != nil {
		t.Fatalf("ExecuteGetRule() error = %v", err)
	}
	if out != plain {
		t.Fatalf("out = %q, want %q", out, plain)
	}
}

func TestBuildRulesSection(t *testing.T) {
	t.Parallel()

	_, rulesSvc := writeTestRules(t, map[string]string{
		"postgres": "---\nname: postgres\ndescription: How we run Postgres\n---\nbody",
		"nginx":    "---\nname: nginx\ndescription: How we run nginx\n---\nbody",
		"bare":     "no frontmatter at all",
	})

	chatSvc := &ChatService{rulesSvc: rulesSvc}
	section, err := chatSvc.buildRulesSection()
	if err != nil {
		t.Fatalf("buildRulesSection() error = %v", err)
	}
	if !strings.Contains(section, "## Rule library") {
		t.Fatalf("section missing heading: %s", section)
	}
	if !strings.Contains(section, "- postgres: How we run Postgres") {
		t.Fatalf("section missing postgres: %s", section)
	}
	if !strings.Contains(section, "- nginx: How we run nginx") {
		t.Fatalf("section missing nginx: %s", section)
	}
	// A rule with no description still has to be offered; only the colon goes.
	if !strings.Contains(section, "- bare\n") {
		t.Fatalf("section missing bare rule: %s", section)
	}
}

func TestBuildRulesSectionEmptyWithoutRules(t *testing.T) {
	t.Parallel()

	_, rulesSvc := writeTestRules(t, nil)
	chatSvc := &ChatService{rulesSvc: rulesSvc}

	section, err := chatSvc.buildRulesSection()
	if err != nil {
		t.Fatalf("buildRulesSection() error = %v", err)
	}
	if section != "" {
		t.Fatalf("section = %q, want empty", section)
	}
}

func TestModeListsRules(t *testing.T) {
	t.Parallel()

	// plan applies the rules, discuss maintains them; every other mode has no
	// business with the catalog.
	for _, m := range []string{"plan", "discuss"} {
		if !modeListsRules(m) {
			t.Fatalf("modeListsRules(%q) = false, want true", m)
		}
	}
	for _, m := range []string{"main", "decompose", "execute", "incident", ""} {
		if modeListsRules(m) {
			t.Fatalf("modeListsRules(%q) = true, want false", m)
		}
	}
}

// The catalog is only half the mechanism: without get_rule in the plan allow
// list the planner reads a menu it cannot order from. Nothing else pins this,
// because get_rule needs a rules service to register and so never reaches
// localSystemToolNames.
func TestPlanAllowListCarriesGetRule(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if _, err := mode.SeedAllowLists(dir); err != nil {
		t.Fatalf("SeedAllowLists: %v", err)
	}

	plan, err := mode.LoadAllowList(dir, "plan")
	if err != nil {
		t.Fatalf("LoadAllowList(plan): %v", err)
	}
	if _, ok := plan[GetRuleToolName]; !ok {
		t.Fatalf("plan allow list = %v, want %q", slices.Sorted(maps.Keys(plan)), GetRuleToolName)
	}

	decompose, err := mode.LoadAllowList(dir, "decompose")
	if err != nil {
		t.Fatalf("LoadAllowList(decompose): %v", err)
	}
	if _, ok := decompose["create_subjects"]; ok {
		t.Fatal("decompose allow list still carries create_subjects")
	}
}

// The catalog only matters if it actually reaches the prompt the planner reads.
// This walks the real path: the embedded plan.md, rendered, with the rule
// library appended -- and asserts the decompose prompt gets nothing.
func TestResolveSystemPromptAppendsTheRuleLibrary(t *testing.T) {
	t.Parallel()

	dataDir, rulesSvc := writeTestRules(t, map[string]string{
		"postgres": "---\nname: postgres\ndescription: How we run Postgres\n---\nbody",
	})
	selection := NewSelectionStore()
	selection.Set(domain.Selection{Project: "demo", Environment: "any", Cloud: "any", Location: "any"})

	svc := &ChatService{
		systemPromptsSvc: systemprompts.NewService(dataDir, "prompts"),
		rulesSvc:         rulesSvc,
		selection:        selection,
		variableRepo: &stubVariableRepo{vars: map[string]map[string]any{
			"global": {
				"CompanyName":    "AutomagicOps",
				"TaskTracker":    "Jira",
				"IssueProject":   "OPS",
				"toolCategories": []any{"cloud", "k8s"},
			},
		}},
	}

	plan, err := svc.resolveSystemPrompt(context.Background(), "plan")
	if err != nil {
		t.Fatalf("resolveSystemPrompt(plan): %v", err)
	}
	if !strings.Contains(plan, "## Rule library") {
		t.Fatal("plan prompt has no rule library")
	}
	if !strings.Contains(plan, "- postgres: How we run Postgres") {
		t.Fatal("plan prompt does not list the postgres rule")
	}

	decompose, err := svc.resolveSystemPrompt(context.Background(), "decompose")
	if err != nil {
		t.Fatalf("resolveSystemPrompt(decompose): %v", err)
	}
	if strings.Contains(decompose, "## Rule library") {
		t.Fatal("decompose prompt carries the rule library, want none")
	}
}
