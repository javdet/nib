package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/filestore"
	"github.com/javdet/nib/internal/mode"
)

func TestWriteRuleToolDef(t *testing.T) {
	t.Parallel()

	def := WriteRuleToolDef()
	if def.Name != WriteRuleToolName {
		t.Fatalf("Name = %q, want %q", def.Name, WriteRuleToolName)
	}
	if def.Description == "" {
		t.Fatal("Description is empty")
	}
	for _, field := range []string{"name", "description", "body"} {
		if !strings.Contains(string(def.Parameters), `"`+field+`"`) {
			t.Fatalf("Parameters missing %q: %s", field, def.Parameters)
		}
	}
}

func TestWriteRuleCreates(t *testing.T) {
	t.Parallel()

	dataDir, rulesSvc := writeTestRules(t, nil)
	chatSvc := &ChatService{rulesSvc: rulesSvc}

	out, err := chatSvc.writeRuleHandler()(context.Background(), map[string]any{
		"name":        "postgres",
		"description": "How we run Postgres",
		"body":        "never expose 5432 publicly",
	})
	if err != nil {
		t.Fatalf("writeRuleHandler() error = %v", err)
	}
	if !strings.Contains(out, "created") {
		t.Fatalf("out = %q, want created", out)
	}

	data, err := os.ReadFile(filepath.Join(dataDir, "rules", "postgres.md"))
	if err != nil {
		t.Fatalf("read rule file: %v", err)
	}
	content := string(data)
	meta := filestore.ParseFrontmatter(content)
	if meta.Name != "postgres" || meta.Description != "How we run Postgres" {
		t.Fatalf("frontmatter = %+v, want name/description filled: %s", meta, content)
	}
	if !strings.Contains(content, "never expose 5432 publicly") {
		t.Fatalf("body missing: %s", content)
	}

	// The rule has to be visible to the catalog the planner reads, not just on
	// disk under some name of its own.
	section, err := chatSvc.buildRulesSection()
	if err != nil {
		t.Fatalf("buildRulesSection() error = %v", err)
	}
	if !strings.Contains(section, "- postgres: How we run Postgres") {
		t.Fatalf("section missing the new rule: %s", section)
	}
}

func TestWriteRuleReplacesExisting(t *testing.T) {
	t.Parallel()

	dataDir, rulesSvc := writeTestRules(t, map[string]string{
		"postgres": "---\nname: postgres\ndescription: old\n---\n\nold body",
	})
	chatSvc := &ChatService{rulesSvc: rulesSvc}

	out, err := chatSvc.writeRuleHandler()(context.Background(), map[string]any{
		"name":        "postgres",
		"description": "new description",
		"body":        "new body",
	})
	if err != nil {
		t.Fatalf("writeRuleHandler() error = %v", err)
	}
	if !strings.Contains(out, "replaced") {
		t.Fatalf("out = %q, want replaced", out)
	}

	data, err := os.ReadFile(filepath.Join(dataDir, "rules", "postgres.md"))
	if err != nil {
		t.Fatalf("read rule file: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "old body") || strings.Contains(content, "description: old") {
		t.Fatalf("old content survived the replace: %s", content)
	}
	if !strings.Contains(content, "new body") {
		t.Fatalf("new body missing: %s", content)
	}
}

// Bad arguments come back as tool output, not as a Go error, so the agent can
// fix them in the next round instead of losing the turn.
func TestWriteRuleRejectsBadArguments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "no name",
			args: map[string]any{"description": "d", "body": "b"},
			want: "name is required",
		},
		{
			name: "path traversal",
			args: map[string]any{"name": "../escape", "description": "d", "body": "b"},
			want: "invalid rule name",
		},
		{
			name: "dotted name",
			args: map[string]any{"name": "postgres.md", "description": "d", "body": "b"},
			want: "invalid rule name",
		},
		{
			name: "no description",
			args: map[string]any{"name": "postgres", "description": "  ", "body": "b"},
			want: "description is required",
		},
		{
			name: "no body",
			args: map[string]any{"name": "postgres", "description": "d", "body": "\n"},
			want: "body is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dataDir, rulesSvc := writeTestRules(t, nil)
			chatSvc := &ChatService{rulesSvc: rulesSvc}

			out, err := chatSvc.writeRuleHandler()(context.Background(), tt.args)
			if err != nil {
				t.Fatalf("writeRuleHandler() error = %v, want tool output", err)
			}
			if !strings.Contains(out, tt.want) {
				t.Fatalf("out = %q, want %q", out, tt.want)
			}

			entries, err := os.ReadDir(filepath.Join(dataDir, "rules"))
			if err != nil {
				t.Fatalf("read rules dir: %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("rules dir = %v, want nothing written", entries)
			}
		})
	}
}

// A pasted multi-line description would push everything after the first newline
// out of the frontmatter block, so it is folded into one line.
func TestWriteRuleFoldsMultilineDescription(t *testing.T) {
	t.Parallel()

	_, rulesSvc := writeTestRules(t, nil)
	chatSvc := &ChatService{rulesSvc: rulesSvc}

	if _, err := chatSvc.writeRuleHandler()(context.Background(), map[string]any{
		"name":        "postgres",
		"description": "How we run Postgres\nand its replicas",
		"body":        "body",
	}); err != nil {
		t.Fatalf("writeRuleHandler() error = %v", err)
	}

	rule, err := rulesSvc.Get("postgres")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if want := "How we run Postgres and its replicas"; filestore.ParseFrontmatter(rule.Content).Description != want {
		t.Fatalf("description = %q, want %q", filestore.ParseFrontmatter(rule.Content).Description, want)
	}
}

func TestWriteRuleWithoutRulesService(t *testing.T) {
	t.Parallel()

	chatSvc := &ChatService{}
	if _, err := chatSvc.writeRuleHandler()(context.Background(), map[string]any{
		"name": "postgres", "description": "d", "body": "b",
	}); err == nil {
		t.Fatal("expected an error without a rules service")
	}
}

// write_rule is discuss mode's alone: the registrar checks the binding and
// enforceModeToolLimits withdraws the name from every other mode, so an
// operator who adds it to plan.json still does not get a planner that can
// rewrite the guardrails it is planning against.
func TestWriteRuleIsDiscussOnly(t *testing.T) {
	t.Parallel()

	_, rulesSvc := writeTestRules(t, nil)
	chatSvc := &ChatService{rulesSvc: rulesSvc}
	allow := map[string]struct{}{WriteRuleToolName: {}}

	for _, m := range mode.Modes {
		catalog := toolCatalog{
			mcpRoutes:     make(map[string]toolRoute),
			localHandlers: make(map[string]localToolHandler),
		}
		registerWriteRuleTool(chatSvc, &catalog, systemToolsBinding(m), allow)
		_, registered := catalog.localHandlers[WriteRuleToolName]
		if want := m == discussDialogMode; registered != want {
			t.Fatalf("mode %q: registered = %v, want %v", m, registered, want)
		}

		withdrawn := map[string]struct{}{WriteRuleToolName: {}}
		enforceModeToolLimits(m, withdrawn)
		_, kept := withdrawn[WriteRuleToolName]
		if want := m == discussDialogMode; kept != want {
			t.Fatalf("mode %q: allowed = %v, want %v", m, kept, want)
		}
	}
}

// Registration is only half the mechanism: without the two names in the discuss
// allow list the tools never reach the catalog. get_rule is there because
// write_rule replaces a rule in full, so editing one starts by reading it.
func TestDiscussAllowListCarriesRuleTools(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if _, err := mode.SeedAllowLists(dir); err != nil {
		t.Fatalf("SeedAllowLists: %v", err)
	}

	allow, err := mode.LoadAllowList(dir, discussDialogMode)
	if err != nil {
		t.Fatalf("LoadAllowList(discuss): %v", err)
	}
	for _, name := range []string{WriteRuleToolName, GetRuleToolName} {
		if _, ok := allow[name]; !ok {
			t.Fatalf("discuss allow list is missing %q", name)
		}
	}
}

// The discuss prompt has to carry the catalog too, or the agent cannot tell
// which rules exist before replacing one.
func TestDiscussListsRules(t *testing.T) {
	t.Parallel()

	if !modeListsRules(discussDialogMode) {
		t.Fatal("modeListsRules(discuss) = false, want true")
	}
}
