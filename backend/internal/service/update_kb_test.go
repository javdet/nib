package service

import (
	"context"
	"slices"
	"testing"

	"github.com/javdet/nib/internal/executor"
	"github.com/javdet/nib/internal/mode"
	"github.com/google/uuid"
)

func newUpdateKBTestCatalog() toolCatalog {
	return toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
}

func catalogHasTool(catalog toolCatalog, name string) bool {
	for _, def := range catalog.tools {
		if def.Name == name {
			return true
		}
	}
	return false
}

func TestAddLocalTools_includesUpdateKB(t *testing.T) {
	t.Parallel()
	svc := &ChatService{knowledgeSvc: &KnowledgeService{}}
	catalog := newUpdateKBTestCatalog()
	allow := map[string]struct{}{UpdateKBToolName: {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[UpdateKBToolName]; !ok {
		t.Fatal("expected update_kb handler")
	}
	if !catalogHasTool(catalog, UpdateKBToolName) {
		t.Fatal("expected update_kb in tool defs")
	}
}

func TestAddLocalTools_excludesUpdateKBWithoutAllowList(t *testing.T) {
	t.Parallel()
	svc := &ChatService{knowledgeSvc: &KnowledgeService{}}
	catalog := newUpdateKBTestCatalog()
	allow := map[string]struct{}{"other_tool": {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[UpdateKBToolName]; ok {
		t.Fatal("did not expect update_kb handler when not in allow list")
	}
	if catalogHasTool(catalog, UpdateKBToolName) {
		t.Fatal("did not expect update_kb in tool defs when not in allow list")
	}
}

func TestAddLocalTools_excludesUpdateKBWithoutKnowledgeService(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	catalog := newUpdateKBTestCatalog()
	allow := map[string]struct{}{UpdateKBToolName: {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[UpdateKBToolName]; ok {
		t.Fatal("did not expect update_kb handler without a knowledge service")
	}
}

// TestExecuteUpdateKB_rejectsBadArgs covers the cases that return before the
// store or embedder is touched, so a nil-dependency service is enough.
func TestExecuteUpdateKB_rejectsBadArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "missing collection",
			args: map[string]any{"content": "body"},
			want: "collection is empty",
		},
		{
			name: "blank collection",
			args: map[string]any{"collection": "   ", "content": "body"},
			want: "collection is empty",
		},
		{
			name: "non-string collection",
			args: map[string]any{"collection": 42, "content": "body"},
			want: "collection is empty",
		},
		{
			name: "missing content",
			args: map[string]any{"collection": "runbooks"},
			want: "content is empty",
		},
		{
			name: "blank content",
			args: map[string]any{"collection": "runbooks", "content": " \n\t "},
			want: "content is empty",
		},
		{
			name: "non-string content",
			args: map[string]any{"collection": "runbooks", "content": []any{"body"}},
			want: "content is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := &KnowledgeService{}

			got, err := svc.ExecuteUpdateKB(context.Background(), tt.args)
			if err != nil {
				t.Fatalf("ExecuteUpdateKB() err = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("ExecuteUpdateKB() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUpdateKBToolDef_shape(t *testing.T) {
	t.Parallel()

	def := UpdateKBToolDef()
	if def.Name != UpdateKBToolName {
		t.Fatalf("def.Name = %q, want %q", def.Name, UpdateKBToolName)
	}
	if def.Description == "" {
		t.Fatal("def.Description is empty")
	}
	if len(def.Parameters) == 0 {
		t.Fatal("def.Parameters is empty")
	}
}

// TestUpdateKB_isDiscussOnly locks in the mode gating: update_kb writes to shared
// state, so it must appear in the discuss allow list and in no other mode.
func TestUpdateKB_isDiscussOnly(t *testing.T) {
	t.Parallel()

	allowToolsDir := systemToolsTestDataDir(t)
	svc := &ChatService{
		knowledgeSvc:  &KnowledgeService{},
		toolSearchSvc: &ToolSearchService{},
		secretSvc:     &SecretService{},
		variableRepo:  &stubVariableRepo{},
		executorSvc:   &executor.Service{},
		dialogRepo:    &actionListDialogRepo{},
		allowToolsDir: allowToolsDir,
	}

	defs, err := svc.SystemToolDefs()
	if err != nil {
		t.Fatalf("SystemToolDefs() err = %v", err)
	}

	var modes []string
	found := false
	for _, def := range defs {
		if def.Name == UpdateKBToolName {
			found = true
			modes = def.Modes
		}
	}
	if !found {
		t.Fatalf("update_kb missing from SystemToolDefs")
	}
	if !slices.Equal(modes, []string{"discuss"}) {
		t.Fatalf("update_kb modes = %#v, want [discuss]", modes)
	}

	for _, modeName := range mode.Modes {
		perMode, err := svc.SystemToolsForMode(modeName)
		if err != nil {
			t.Fatalf("SystemToolsForMode(%q) err = %v", modeName, err)
		}
		present := false
		for _, def := range perMode {
			if def.Name == UpdateKBToolName {
				present = true
				break
			}
		}
		if want := modeName == "discuss"; present != want {
			t.Fatalf("update_kb present in %q = %v, want %v", modeName, present, want)
		}
	}
}
