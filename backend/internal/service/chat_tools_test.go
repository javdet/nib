package service

import (
	"context"
	"testing"

	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/mcpclient"
	"github.com/google/uuid"
)

func TestAppendMCPTools_respectsAllowList(t *testing.T) {
	t.Parallel()

	allow := map[string]struct{}{
		"jira_search": {},
		"other_tool":  {},
	}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	tools := []mcpclient.ToolInfo{
		{Name: "jira_search", Description: "search jira"},
		{Name: "jira_create", Description: "create issue"},
	}
	appendMCPTools(&catalog, tools, allow, toolRoute{serverURL: "https://example/mcp"}, "mcpServer", "atlassian")

	if len(catalog.tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(catalog.tools))
	}
	if catalog.tools[0].Name != "jira_search" {
		t.Fatalf("tool name = %q, want jira_search", catalog.tools[0].Name)
	}
	if route := catalog.mcpRoutes["jira_search"]; route.serverURL != "https://example/mcp" {
		t.Fatalf("route serverURL = %q", route.serverURL)
	}
}

func TestAppendMCPTools_skipsDuplicateNames(t *testing.T) {
	t.Parallel()

	catalog := toolCatalog{
		mcpRoutes: map[string]toolRoute{
			"jira_search": {connID: uuid.New()},
		},
		localHandlers: make(map[string]localToolHandler),
	}
	tools := []mcpclient.ToolInfo{{Name: "jira_search", Description: "dup"}}
	appendMCPTools(&catalog, tools, nil, toolRoute{serverURL: "https://example/mcp"}, "mcpServer", "atlassian")

	if len(catalog.tools) != 0 {
		t.Fatalf("len(tools) = %d, want 0", len(catalog.tools))
	}
}

func TestAppendMCPTools_skipsLocalToolNames(t *testing.T) {
	t.Parallel()

	catalog := toolCatalog{
		mcpRoutes: make(map[string]toolRoute),
		localHandlers: map[string]localToolHandler{
			"knowledge_search": func(context.Context, map[string]any) (string, error) { return "", nil },
		},
	}
	tools := []mcpclient.ToolInfo{{Name: "knowledge_search", Description: "local wins"}}
	appendMCPTools(&catalog, tools, nil, toolRoute{serverURL: "https://example/mcp"}, "mcpServer", "kb")

	if len(catalog.tools) != 0 {
		t.Fatalf("len(tools) = %d, want 0", len(catalog.tools))
	}
}

func TestLogMissingAllowTools(t *testing.T) {
	t.Parallel()

	allow := map[string]struct{}{
		"jira_search":    {},
		"knowledge_search": {},
	}
	catalog := toolCatalog{
		tools: []llm.ToolDef{{Name: "knowledge_search"}},
	}
	// Should not panic; missing jira_search is logged at warn level.
	logMissingAllowTools(allow, catalog)
}
