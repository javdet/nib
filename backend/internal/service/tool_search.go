package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/toolcatalog"
)

const (
	toolSearchDefaultLimit = 10
	toolSearchMaxLimit     = 50
)

var toolSearchParameters = json.RawMessage(`{
  "type": "object",
  "required": ["query"],
  "properties": {
    "query": {
      "type": "string",
      "description": "Free-text search over tool names and descriptions."
    },
    "categories": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Optional filter: only tools assigned to one of these categories via name/prefix patterns."
    },
    "limit": {
      "type": "integer",
      "default": 10,
      "maximum": 50
    },
    "scope": {
      "type": "string",
      "enum": ["tool", "server", "both"],
      "default": "tool"
    }
  }
}`)

// catalogSearcher is the subset of toolcatalog.Store used by tool_search and
// dynamic tool routing (mockable in tests).
type catalogSearcher interface {
	Search(ctx context.Context, query string, queryEmbedding []float32, categories []string, limit int, scope toolcatalog.SearchScope) (toolcatalog.SearchResult, error)
	GetToolByName(ctx context.Context, name string) (toolcatalog.Tool, bool, error)
}

// ToolSearchService runs hybrid full-text and embedding search over the MCP tool catalog.
type ToolSearchService struct {
	store          catalogSearcher
	embedder       llm.Embedder
	embeddingModel string
}

// NewToolSearchService wires the catalog store and optional embedder used by tool_search.
func NewToolSearchService(store catalogSearcher, embedder llm.Embedder, embeddingModel string) *ToolSearchService {
	return &ToolSearchService{
		store:          store,
		embedder:       embedder,
		embeddingModel: strings.TrimSpace(embeddingModel),
	}
}

// ToolSearchToolDef returns the LLM tool definition for the local tool_search handler.
func ToolSearchToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name:        toolcatalog.ToolSearchToolName,
		Description: "Search the catalog of available MCP tools by free-text query and optional category. Returns matching tools with server, name, description, and per-tool categories (null when uncategorized). Server-level matches when scope is server or both.",
		Parameters:  toolSearchParameters,
	}
}

type toolSearchResponseRow struct {
	Server      string   `json:"server"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`
	Score       float64  `json:"score"`
}

type toolSearchServerResponseRow struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	URL         string   `json:"url"`
	Categories  []string `json:"categories"`
	Score       float64  `json:"score"`
}

type toolSearchBothResponse struct {
	Tools   []toolSearchResponseRow       `json:"tools"`
	Servers []toolSearchServerResponseRow `json:"servers"`
}

// ExecuteToolSearch runs hybrid search against the tool catalog and returns the
// tool output text only.
func (s *ToolSearchService) ExecuteToolSearch(ctx context.Context, args map[string]any) (string, error) {
	out, _, err := s.SearchTools(ctx, args)
	return out, err
}

// SearchTools runs hybrid search against the tool catalog and returns both the
// tool output text and the matched tool hits, so callers can make the matches
// callable for the rest of the agent loop.
// Validation failures are returned as tool output text (not Go errors) so the agent loop can continue.
func (s *ToolSearchService) SearchTools(ctx context.Context, args map[string]any) (string, []toolcatalog.SearchHit, error) {
	if s == nil || s.store == nil {
		return "tool catalog is not configured", nil, nil
	}

	query, _ := args["query"].(string)
	query = strings.TrimSpace(query)
	if query == "" {
		return "query is empty", nil, nil
	}

	limit := toolSearchDefaultLimit
	if raw, ok := args["limit"]; ok && raw != nil {
		switch v := raw.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		case int64:
			limit = int(v)
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > toolSearchMaxLimit {
		limit = toolSearchMaxLimit
	}

	scope := toolcatalog.SearchScope(strings.TrimSpace(stringArg(args, "scope")))
	if scope == "" {
		scope = toolcatalog.SearchScopeTool
	}
	switch scope {
	case toolcatalog.SearchScopeTool, toolcatalog.SearchScopeServer, toolcatalog.SearchScopeBoth:
	default:
		return fmt.Sprintf("invalid scope %q (want tool, server, or both)", scope), nil, nil
	}

	cats := parseStringSliceArg(args["categories"])
	if len(cats) == 0 {
		cats = nil
	}

	queryEmbedding := s.embedQuery(ctx, query)

	res, err := s.store.Search(ctx, query, queryEmbedding, cats, limit, scope)
	if err != nil {
		return "", nil, fmt.Errorf("tool search: %w", err)
	}

	switch scope {
	case toolcatalog.SearchScopeServer:
		rows := make([]toolSearchServerResponseRow, 0, len(res.Servers))
		for _, h := range res.Servers {
			rows = append(rows, toolSearchServerResponseRow{
				Name:        h.Name,
				Description: h.Description,
				URL:         h.URL,
				Categories:  h.Categories,
				Score:       h.Score,
			})
		}
		b, err := json.Marshal(rows)
		if err != nil {
			return "", nil, fmt.Errorf("marshal search results: %w", err)
		}
		return string(b), nil, nil

	case toolcatalog.SearchScopeBoth:
		toolRows := make([]toolSearchResponseRow, 0, len(res.Tools))
		for _, h := range res.Tools {
			toolRows = append(toolRows, toolSearchResponseRow{
				Server:      h.Server,
				Name:        h.Name,
				Description: h.Description,
				Categories:  h.Categories,
				Score:       h.Score,
			})
		}
		srvRows := make([]toolSearchServerResponseRow, 0, len(res.Servers))
		for _, h := range res.Servers {
			srvRows = append(srvRows, toolSearchServerResponseRow{
				Name:        h.Name,
				Description: h.Description,
				URL:         h.URL,
				Categories:  h.Categories,
				Score:       h.Score,
			})
		}
		both := toolSearchBothResponse{Tools: toolRows, Servers: srvRows}
		b, err := json.Marshal(both)
		if err != nil {
			return "", nil, fmt.Errorf("marshal search results: %w", err)
		}
		return string(b), res.Tools, nil

	default:
		rows := make([]toolSearchResponseRow, 0, len(res.Tools))
		for _, h := range res.Tools {
			rows = append(rows, toolSearchResponseRow{
				Server:      h.Server,
				Name:        h.Name,
				Description: h.Description,
				Categories:  h.Categories,
				Score:       h.Score,
			})
		}
		b, err := json.Marshal(rows)
		if err != nil {
			return "", nil, fmt.Errorf("marshal search results: %w", err)
		}
		return string(b), res.Tools, nil
	}
}

// LookupTool returns the indexed catalog entry for an exact tool name.
func (s *ToolSearchService) LookupTool(ctx context.Context, name string) (toolcatalog.Tool, bool, error) {
	if s == nil || s.store == nil {
		return toolcatalog.Tool{}, false, nil
	}
	return s.store.GetToolByName(ctx, name)
}

func (s *ToolSearchService) embedQuery(ctx context.Context, query string) []float32 {
	if s == nil || s.embedder == nil {
		return nil
	}
	vecs, _, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		slog.Warn("tool search: embed query failed, falling back to full-text search", "error", err)
		return nil
	}
	if len(vecs) != 1 {
		slog.Warn("tool search: embedder returned unexpected vector count", "got", len(vecs))
		return nil
	}
	return vecs[0]
}

func stringArg(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

func parseStringSliceArg(raw any) []string {
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if t := strings.TrimSpace(s); t != "" {
				out = append(out, t)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, _ := item.(string)
			if t := strings.TrimSpace(s); t != "" {
				out = append(out, t)
			}
		}
		return out
	default:
		return nil
	}
}
