package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/executor"
	"github.com/javdet/nib/internal/mode"
	"github.com/javdet/nib/internal/toolcatalog"
)

var localSystemToolNames = []string{
	ChatNameToolName,
	KnowledgeSearchToolName,
	toolcatalog.ToolSearchToolName,
	APICallToolName,
	ExecuteCommandToolName,
	GetSecretsToolName,
	ListVariablesToolName,
	RunExecutorToolName,
	AskQuestionToolName,
	ReportBlockerToolName,
	CreateDAGToolName,
	CreatePlanContractToolName,
	CreateSummaryToolName,
	CreateTableToolName,
	CreateSubjectsToolName,
	CreateActionPlanToolName,
	UpdateActionPlanToolName,
	UpdateRollbackPlanToolName,
	GetActionListToolName,
	ExecuteActionToolName,
	GetKBDocumentToolName,
	UpdateKBToolName,
	RunSubagentToolName,
	StopExecutionToolName,
}

func isLocalSystemTool(name string) bool {
	return slices.Contains(localSystemToolNames, name)
}

func TestSystemToolDefs_returnsAllLocalTools(t *testing.T) {
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
	if len(defs) != len(localSystemToolNames) {
		t.Fatalf("len(defs) = %d, want %d", len(defs), len(localSystemToolNames))
	}

	byName := make(map[string]SystemToolDef, len(defs))
	for _, def := range defs {
		byName[def.Name] = def
		if def.Name == "" {
			t.Fatal("tool name is empty")
		}
		if strings.TrimSpace(def.Description) == "" {
			t.Fatalf("tool %q has empty description", def.Name)
		}
		if len(def.Parameters) == 0 {
			t.Fatalf("tool %q has empty parameters", def.Name)
		}
	}

	for _, name := range localSystemToolNames {
		if _, ok := byName[name]; !ok {
			t.Fatalf("missing tool %q", name)
		}
	}

	chatName := byName[ChatNameToolName]
	if !chatName.AlwaysOn {
		t.Fatalf("chat_name always_on = false, want true")
	}
	if chatName.Modes == nil {
		t.Fatal("chat_name modes is nil, want empty slice")
	}
	if len(chatName.Modes) != 0 {
		t.Fatalf("chat_name modes = %#v, want empty", chatName.Modes)
	}

	raw, err := json.Marshal(defs)
	if err != nil {
		t.Fatalf("json.Marshal(defs) err = %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(defs) err = %v", err)
	}
	for _, item := range decoded {
		modes, ok := item["modes"]
		if !ok {
			t.Fatalf("tool %q missing modes", item["name"])
		}
		if modes == nil {
			t.Fatalf("tool %q modes is JSON null", item["name"])
		}
	}
}

func TestSystemToolDefs_modeAllowListsMatchDataFiles(t *testing.T) {
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

	byName := make(map[string]SystemToolDef, len(defs))
	for _, def := range defs {
		byName[def.Name] = def
	}

	for _, modeName := range mode.Modes {
		allow, err := mode.LoadAllowList(allowToolsDir, modeName)
		if err != nil {
			t.Fatalf("LoadAllowList(%q) err = %v", modeName, err)
		}
		for name := range allow {
			if !isLocalSystemTool(name) || name == ChatNameToolName {
				continue
			}
			def, ok := byName[name]
			if !ok {
				t.Fatalf("local tool %q in %s allow-list but missing from SystemToolDefs", name, modeName)
			}
			if !slices.Contains(def.Modes, modeName) {
				t.Fatalf("tool %q missing mode %q, got modes %#v", name, modeName, def.Modes)
			}
		}
	}
}

func TestSystemToolsForMode_returnsModeAllowList(t *testing.T) {
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

	defs, err := svc.SystemToolsForMode("discuss")
	if err != nil {
		t.Fatalf("SystemToolsForMode(discuss) err = %v", err)
	}
	if len(defs) == 0 {
		t.Fatal("expected system tools for discuss mode")
	}

	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	if !slices.Contains(names, KnowledgeSearchToolName) {
		t.Fatalf("discuss tools = %#v, missing knowledge_search", names)
	}
}

// systemToolsTestDataDir builds the tools directory a running backend would have:
// the per-mode allow lists reconciled out of the binary, plus the tool schemas
// seeded from the image. The runtime DATA_DIR is a Docker volume and is not in
// the working tree, so these tests assemble the same thing from both sources.
func systemToolsTestDataDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	if _, err := mode.SeedAllowLists(dir); err != nil {
		t.Fatalf("seed allow lists: %v", err)
	}

	schemaSrc := filepath.Join("..", "..", "seed", "tools", "schemas")
	entries, err := os.ReadDir(schemaSrc)
	if err != nil {
		t.Fatalf("read seed schemas: %v", err)
	}
	schemaDst := filepath.Join(dir, "schemas")
	if err := os.MkdirAll(schemaDst, 0o755); err != nil {
		t.Fatalf("create schemas dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(schemaSrc, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(schemaDst, e.Name()), b, 0o644); err != nil {
			t.Fatalf("write %s: %v", e.Name(), err)
		}
	}
	return dir
}
