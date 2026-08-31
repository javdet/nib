package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

func TestListVariablesToolDef(t *testing.T) {
	t.Parallel()

	def := ListVariablesToolDef()
	if def.Name != ListVariablesToolName {
		t.Fatalf("name = %q, want %q", def.Name, ListVariablesToolName)
	}
	if def.Description == "" {
		t.Fatal("description is empty")
	}
	var schema struct {
		Type       string         `json:"type"`
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(def.Parameters, &schema); err != nil {
		t.Fatalf("unmarshal parameters: %v", err)
	}
	if schema.Type != "object" {
		t.Fatalf("type = %q, want object", schema.Type)
	}
}

func TestExecuteListVariables(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("returns sorted JSON array", func(t *testing.T) {
		svc := &ChatService{
			variableRepo: &stubVariableRepo{
				list: []domain.PromptVariable{
					{Scope: "global", Name: "CompanyName", Description: "Company name", Value: "Acme", Kind: "string"},
					{Scope: "global", Name: "toolCategories", Description: "Tool categories", Value: `["k8s","cloud"]`, Kind: "list"},
					{Scope: "env", Name: "Region", Description: "Region", Value: "us-east-1", Kind: "string"},
				},
			},
		}

		out, err := svc.ExecuteListVariables(ctx, nil)
		if err != nil {
			t.Fatalf("err = %v", err)
		}

		var rows []listVariablesRow
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatalf("unmarshal output: %v", err)
		}
		if len(rows) != 3 {
			t.Fatalf("len(rows) = %d, want 3", len(rows))
		}
		if rows[0].Scope != "env" || rows[0].Name != "Region" {
			t.Fatalf("rows[0] = %#v, want env/Region first", rows[0])
		}
		if rows[1].Scope != "global" || rows[1].Name != "CompanyName" {
			t.Fatalf("rows[1] = %#v, want global/CompanyName", rows[1])
		}
		if rows[2].Kind != "list" || rows[2].Value != `["k8s","cloud"]` {
			t.Fatalf("rows[2] = %#v", rows[2])
		}
	})

	t.Run("empty list returns empty array", func(t *testing.T) {
		svc := &ChatService{
			variableRepo: &stubVariableRepo{list: []domain.PromptVariable{}},
		}

		out, err := svc.ExecuteListVariables(ctx, nil)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if out != "[]" {
			t.Fatalf("out = %q, want []", out)
		}
	})

	t.Run("nil variableRepo returns error", func(t *testing.T) {
		svc := &ChatService{}

		_, err := svc.ExecuteListVariables(ctx, nil)
		if err == nil {
			t.Fatal("expected error for nil variableRepo")
		}
	})
}

func TestAddLocalTools_includesListVariablesWhenAllowed(t *testing.T) {
	t.Parallel()

	svc := &ChatService{variableRepo: &stubVariableRepo{}}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{ListVariablesToolName: {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[ListVariablesToolName]; !ok {
		t.Fatal("expected list_variables handler")
	}
	found := false
	for _, def := range catalog.tools {
		if def.Name == ListVariablesToolName {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected list_variables in tool defs")
	}
}

func TestAddLocalTools_excludesListVariablesWithoutAllowList(t *testing.T) {
	t.Parallel()

	svc := &ChatService{variableRepo: &stubVariableRepo{}}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{"other_tool": {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[ListVariablesToolName]; ok {
		t.Fatal("did not expect list_variables handler when not in allow list")
	}
}

func TestAddLocalTools_excludesListVariablesWithoutVariableRepo(t *testing.T) {
	t.Parallel()

	svc := &ChatService{}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{ListVariablesToolName: {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[ListVariablesToolName]; ok {
		t.Fatal("did not expect list_variables handler without variableRepo")
	}
}
