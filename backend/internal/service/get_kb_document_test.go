package service

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/javdet/nib/internal/executor"
	"github.com/javdet/nib/internal/kbdoc"
	"github.com/javdet/nib/internal/mode"
	"github.com/google/uuid"
)

func TestAddLocalTools_includesGetKBDocument(t *testing.T) {
	t.Parallel()
	svc := &ChatService{knowledgeSvc: &KnowledgeService{}}
	catalog := newUpdateKBTestCatalog()
	allow := map[string]struct{}{GetKBDocumentToolName: {}}

	svc.addLocalTools(&catalog, allow, uuid.New())

	if _, ok := catalog.localHandlers[GetKBDocumentToolName]; !ok {
		t.Fatal("expected get_kb_document handler")
	}
	if !catalogHasTool(catalog, GetKBDocumentToolName) {
		t.Fatal("expected get_kb_document in tool defs")
	}
}

func TestAddLocalTools_excludesGetKBDocumentWithoutAllowList(t *testing.T) {
	t.Parallel()
	svc := &ChatService{knowledgeSvc: &KnowledgeService{}}
	catalog := newUpdateKBTestCatalog()
	allow := map[string]struct{}{"other_tool": {}}

	svc.addLocalTools(&catalog, allow, uuid.New())

	if _, ok := catalog.localHandlers[GetKBDocumentToolName]; ok {
		t.Fatal("did not expect get_kb_document handler when not in allow list")
	}
	if catalogHasTool(catalog, GetKBDocumentToolName) {
		t.Fatal("did not expect get_kb_document in tool defs when not in allow list")
	}
}

func TestAddLocalTools_excludesGetKBDocumentWithoutKnowledgeService(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	catalog := newUpdateKBTestCatalog()
	allow := map[string]struct{}{GetKBDocumentToolName: {}}

	svc.addLocalTools(&catalog, allow, uuid.New())

	if _, ok := catalog.localHandlers[GetKBDocumentToolName]; ok {
		t.Fatal("did not expect get_kb_document handler without a knowledge service")
	}
}

func TestExecuteGetKBDocument_withoutDocumentStore(t *testing.T) {
	t.Parallel()
	svc := &KnowledgeService{}

	if _, err := svc.ExecuteGetKBDocument(context.Background(), map[string]any{"collection": "runbooks"}); err == nil {
		t.Fatal("ExecuteGetKBDocument() err = nil, want error without a document store")
	}
}

// TestExecuteGetKBDocument_rejectsBadCollection covers the name validation that
// returns before the document store is touched, so an empty dir is enough.
func TestExecuteGetKBDocument_rejectsBadCollection(t *testing.T) {
	t.Parallel()
	svc := &KnowledgeService{
		defaultCollection: "default",
		docs:              kbdoc.NewService(t.TempDir(), "knowledgebase"),
	}

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "path traversal",
			args: map[string]any{"collection": "../etc/passwd"},
			want: `invalid collection name "../etc/passwd": must match [a-zA-Z0-9][a-zA-Z0-9._-]{0,63}`,
		},
		{
			name: "leading dot",
			args: map[string]any{"collection": ".hidden"},
			want: `invalid collection name ".hidden": must match [a-zA-Z0-9][a-zA-Z0-9._-]{0,63}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := svc.ExecuteGetKBDocument(context.Background(), tt.args)
			if err != nil {
				t.Fatalf("ExecuteGetKBDocument() err = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("ExecuteGetKBDocument() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestExecuteGetKBDocument_fallsBackToSkeleton locks in the contract the
// build-knowledge-base skill relies on: a collection nobody has uploaded to
// answers with the template, not an error.
func TestExecuteGetKBDocument_fallsBackToSkeleton(t *testing.T) {
	t.Parallel()
	svc := &KnowledgeService{
		defaultCollection: "default",
		docs:              kbdoc.NewService(t.TempDir(), "knowledgebase"),
	}

	out, err := svc.ExecuteGetKBDocument(context.Background(), map[string]any{"collection": "fresh"})
	if err != nil {
		t.Fatalf("ExecuteGetKBDocument() err = %v", err)
	}

	var got struct {
		Collection string `json:"collection"`
		Source     string `json:"source"`
		UpdatedAt  string `json:"updated_at"`
		Content    string `json:"content"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal tool output: %v", err)
	}
	if got.Collection != "fresh" {
		t.Fatalf("collection = %q, want %q", got.Collection, "fresh")
	}
	if got.Source != string(kbdoc.SourceTemplate) {
		t.Fatalf("source = %q, want %q", got.Source, kbdoc.SourceTemplate)
	}
	if got.UpdatedAt != "" {
		t.Fatalf("updated_at = %q, want empty for the template", got.UpdatedAt)
	}
	if got.Content != kbdoc.Skeleton() {
		t.Fatal("content is not the embedded skeleton")
	}
}

func TestExecuteGetKBDocument_returnsUploadedDocument(t *testing.T) {
	t.Parallel()
	docs := kbdoc.NewService(t.TempDir(), "knowledgebase")
	if err := docs.Save("kiss", "# Knowledge Base\n\n## Project Overview\nGame platform.\n"); err != nil {
		t.Fatalf("Save() err = %v", err)
	}
	svc := &KnowledgeService{defaultCollection: "default", docs: docs}

	out, err := svc.ExecuteGetKBDocument(context.Background(), map[string]any{"collection": "kiss"})
	if err != nil {
		t.Fatalf("ExecuteGetKBDocument() err = %v", err)
	}

	var got struct {
		Source    string `json:"source"`
		UpdatedAt string `json:"updated_at"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal tool output: %v", err)
	}
	if got.Source != string(kbdoc.SourceUploaded) {
		t.Fatalf("source = %q, want %q", got.Source, kbdoc.SourceUploaded)
	}
	if got.UpdatedAt == "" {
		t.Fatal("updated_at is empty for an uploaded document")
	}
	if got.Content != "# Knowledge Base\n\n## Project Overview\nGame platform.\n" {
		t.Fatalf("content = %q", got.Content)
	}
}

func TestGetKBDocumentToolDef_shape(t *testing.T) {
	t.Parallel()

	def := GetKBDocumentToolDef()
	if def.Name != GetKBDocumentToolName {
		t.Fatalf("def.Name = %q, want %q", def.Name, GetKBDocumentToolName)
	}
	if def.Description == "" {
		t.Fatal("def.Description is empty")
	}
	if len(def.Parameters) == 0 {
		t.Fatal("def.Parameters is empty")
	}
}

// TestGetKBDocument_isDiscussOnly mirrors the update_kb gating: the read half
// belongs in the same mode as the write half it feeds.
func TestGetKBDocument_isDiscussOnly(t *testing.T) {
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
		if def.Name == GetKBDocumentToolName {
			found = true
			modes = def.Modes
		}
	}
	if !found {
		t.Fatalf("get_kb_document missing from SystemToolDefs")
	}
	if !slices.Equal(modes, []string{"discuss"}) {
		t.Fatalf("get_kb_document modes = %#v, want [discuss]", modes)
	}

	for _, modeName := range mode.Modes {
		perMode, err := svc.SystemToolsForMode(modeName)
		if err != nil {
			t.Fatalf("SystemToolsForMode(%q) err = %v", modeName, err)
		}
		present := false
		for _, def := range perMode {
			if def.Name == GetKBDocumentToolName {
				present = true
				break
			}
		}
		if want := modeName == "discuss"; present != want {
			t.Fatalf("get_kb_document present in %q = %v, want %v", modeName, present, want)
		}
	}
}
