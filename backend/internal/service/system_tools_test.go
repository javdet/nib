package service

import (
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
	GetActionListToolName,
	GetKBDocumentToolName,
	UpdateKBToolName,
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
	if len(chatName.Modes) != 0 {
		t.Fatalf("chat_name modes = %#v, want empty", chatName.Modes)
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

// systemToolsTestDataDir returns backend/seed/tools, the defaults baked into the
// image and copied into DATA_DIR/tools on first start (see docker-entrypoint.sh).
// The runtime DATA_DIR itself is a Docker volume and is not in the working tree,
// so the seed directory is what these tests can assert against.
func systemToolsTestDataDir(t *testing.T) string {
	t.Helper()

	dir := filepath.Join("..", "..", "seed", "tools")
	if _, err := os.Stat(filepath.Join(dir, "decompose.json")); err != nil {
		t.Fatalf("stat seed tools dir: %v", err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("filepath.Abs(%q) err = %v", dir, err)
	}
	return abs
}
