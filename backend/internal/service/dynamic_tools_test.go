package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/mcpconfig"
	"github.com/javdet/nib/internal/toolcatalog"
)

const testMCPConfig = `{
  "mcpServers": {
    "tool-gw": {
      "url": "https://tool-gw.example.com/mcp",
      "transport": "http",
      "headers": { "Authorization": "Bearer test" }
    },
    "stdio-only": {
      "command": "npx"
    }
  }
}`

func newTestMCPConfigService(t *testing.T) *mcpconfig.Service {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte(testMCPConfig), 0o600); err != nil {
		t.Fatalf("write mcp.json: %v", err)
	}
	return mcpconfig.NewServiceAtPath(path)
}

func newTestChatService(t *testing.T, store *fakeCatalogSearcher) *ChatService {
	t.Helper()
	return &ChatService{
		mcpConfigSvc:  newTestMCPConfigService(t),
		toolSearchSvc: NewToolSearchService(store, nil, ""),
	}
}

func TestActivateCatalogTools_makesSearchHitsCallable(t *testing.T) {
	t.Parallel()

	svc := newTestChatService(t, &fakeCatalogSearcher{})
	catalog := newToolCatalog()
	hits := []toolcatalog.SearchHit{
		{Tool: toolcatalog.Tool{
			Server:      "tool-gw",
			Name:        "atlassian_jira_get_agile_boards",
			Description: "Get jira agile boards",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		{Tool: toolcatalog.Tool{Server: "stdio-only", Name: "local_thing"}},
		{Tool: toolcatalog.Tool{Server: "missing-server", Name: "ghost_tool"}},
	}

	svc.activateCatalogTools(context.Background(), catalog, hits)

	route, ok := catalog.mcpRoutes["atlassian_jira_get_agile_boards"]
	if !ok {
		t.Fatal("expected searched tool to be routable")
	}
	if route.serverURL != "https://tool-gw.example.com/mcp" {
		t.Fatalf("serverURL = %q", route.serverURL)
	}
	if route.headers["Authorization"] != "Bearer test" {
		t.Fatalf("headers = %#v", route.headers)
	}
	if len(catalog.tools) != 1 || catalog.tools[0].Name != "atlassian_jira_get_agile_boards" {
		t.Fatalf("tools = %#v, want only the http-served hit", catalog.tools)
	}
}

func TestActivateCatalogTools_keepsExistingRoutes(t *testing.T) {
	t.Parallel()

	svc := newTestChatService(t, &fakeCatalogSearcher{})
	catalog := newToolCatalog()
	catalog.localHandlers["knowledge_search"] = func(context.Context, map[string]any) (string, error) { return "", nil }
	catalog.addMCPTool(llm.ToolDef{Name: "jira_search"}, toolRoute{serverURL: "https://existing/mcp"})

	svc.activateCatalogTools(context.Background(), catalog, []toolcatalog.SearchHit{
		{Tool: toolcatalog.Tool{Server: "tool-gw", Name: "knowledge_search"}},
		{Tool: toolcatalog.Tool{Server: "tool-gw", Name: "jira_search"}},
	})

	if _, ok := catalog.mcpRoutes["knowledge_search"]; ok {
		t.Fatal("local handler must win over a catalog hit")
	}
	if route := catalog.mcpRoutes["jira_search"]; route.serverURL != "https://existing/mcp" {
		t.Fatalf("existing route overwritten: %q", route.serverURL)
	}
	if len(catalog.tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(catalog.tools))
	}
}

func TestResolveDynamicTool_fromCatalogByName(t *testing.T) {
	t.Parallel()

	store := &fakeCatalogSearcher{
		lookupFound: true,
		lookupTool: toolcatalog.Tool{
			Server:      "tool-gw",
			Name:        "atlassian_jira_search",
			Description: "Search jira issues",
		},
	}
	svc := newTestChatService(t, store)

	def, route, ok := svc.resolveDynamicTool(context.Background(), "atlassian_jira_search")
	if !ok {
		t.Fatal("expected tool to resolve")
	}
	if store.gotLookupName != "atlassian_jira_search" {
		t.Fatalf("lookup name = %q", store.gotLookupName)
	}
	if def.Name != "atlassian_jira_search" || string(def.Parameters) != `{"type":"object"}` {
		t.Fatalf("def = %#v", def)
	}
	if route.serverURL != "https://tool-gw.example.com/mcp" {
		t.Fatalf("serverURL = %q", route.serverURL)
	}
}

func TestResolveDynamicTool_unknownName(t *testing.T) {
	t.Parallel()

	svc := newTestChatService(t, &fakeCatalogSearcher{})
	if _, _, ok := svc.resolveDynamicTool(context.Background(), "nope"); ok {
		t.Fatal("expected unknown tool not to resolve")
	}
}

func TestExecuteToolCall_unknownToolStaysUnknown(t *testing.T) {
	t.Parallel()

	svc := newTestChatService(t, &fakeCatalogSearcher{})
	_, err := svc.executeToolCall(context.Background(), newToolCatalog(), llm.ToolCall{
		Name:      "nope",
		Arguments: "{}",
	})
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestToolSearchHandler_activatesResults(t *testing.T) {
	t.Parallel()

	store := &fakeCatalogSearcher{
		result: toolcatalog.SearchResult{
			Tools: []toolcatalog.SearchHit{
				{Tool: toolcatalog.Tool{Server: "tool-gw", Name: "atlassian_jira_get_agile_boards"}},
			},
		},
	}
	svc := newTestChatService(t, store)
	catalog := newToolCatalog()

	out, err := svc.toolSearchHandler(catalog)(context.Background(), map[string]any{"query": "jira boards"})
	if err != nil {
		t.Fatalf("tool search handler: %v", err)
	}
	if out == "" {
		t.Fatal("expected search output")
	}
	if _, ok := catalog.mcpRoutes["atlassian_jira_get_agile_boards"]; !ok {
		t.Fatal("expected search hit to be activated in the catalog")
	}
}
