package service

import (
	"context"
	"log/slog"
	"strings"

	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/toolcatalog"
)

// toolSearchHandler wraps tool_search so every matched tool becomes callable for
// the remaining rounds of the agent loop. Without this the model can only see
// the statically allow-listed tools, and calling a discovered one fails with
// "unknown tool".
func (s *ChatService) toolSearchHandler(catalog *toolCatalog) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		out, hits, err := s.toolSearchSvc.SearchTools(ctx, args)
		if err != nil {
			return "", err
		}
		s.activateCatalogTools(ctx, catalog, hits)
		return out, nil
	}
}

// activateCatalogTools adds tools discovered by tool_search to the live catalog,
// skipping names already served locally or by another route.
func (s *ChatService) activateCatalogTools(ctx context.Context, catalog *toolCatalog, hits []toolcatalog.SearchHit) {
	if catalog == nil {
		return
	}
	for _, hit := range hits {
		def, route, ok := s.catalogToolRoute(ctx, hit.Tool)
		if !ok {
			continue
		}
		if catalog.addMCPTool(def, route) {
			slog.Info("tool search: activated tool", "server", hit.Server, "name", hit.Name)
		}
	}
}

// resolveDynamicTool looks up a tool by exact name in the indexed catalog and
// builds its MCP route. It backs tool calls that reference tools discovered in
// an earlier turn, whose activation did not survive the catalog rebuild.
func (s *ChatService) resolveDynamicTool(ctx context.Context, name string) (llm.ToolDef, toolRoute, bool) {
	if s.toolSearchSvc == nil {
		return llm.ToolDef{}, toolRoute{}, false
	}
	tool, found, err := s.toolSearchSvc.LookupTool(ctx, name)
	if err != nil {
		slog.Warn("resolve tool from catalog failed", "name", name, "error", err)
		return llm.ToolDef{}, toolRoute{}, false
	}
	if !found {
		return llm.ToolDef{}, toolRoute{}, false
	}
	return s.catalogToolRoute(ctx, tool)
}

// catalogToolRoute turns a catalog entry into an LLM tool definition plus the
// MCP route that serves it, resolved from mcp.json by server name. Any ${NAME}
// references in the entry are expanded before the route is built.
func (s *ChatService) catalogToolRoute(ctx context.Context, tool toolcatalog.Tool) (llm.ToolDef, toolRoute, bool) {
	name := strings.TrimSpace(tool.Name)
	if name == "" {
		return llm.ToolDef{}, toolRoute{}, false
	}
	if s.mcpConfigSvc == nil {
		return llm.ToolDef{}, toolRoute{}, false
	}

	server, err := s.mcpConfigSvc.GetServerResolved(ctx, tool.Server)
	if err != nil {
		slog.Warn("catalog tool mcp.json server unavailable",
			"server", tool.Server, "name", name, "error", err)
		return llm.ToolDef{}, toolRoute{}, false
	}
	url := strings.TrimSpace(server.URL)
	if url == "" {
		slog.Warn("catalog tool server has no URL", "server", tool.Server, "name", name)
		return llm.ToolDef{}, toolRoute{}, false
	}

	def := llm.ToolDef{
		Name:        name,
		Description: tool.Description,
		Parameters:  toolParametersJSON(tool.InputSchema),
	}
	route := toolRoute{
		serverURL: url,
		headers:   cloneHeaderMap(server.Headers),
		secrets:   server.Values(),
	}
	return def, route, true
}
