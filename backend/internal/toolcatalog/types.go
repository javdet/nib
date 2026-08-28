package toolcatalog

import (
	"encoding/json"

	"github.com/google/uuid"
)

// Category is one row in tool_categories.
type Category struct {
	ID          uuid.UUID
	Name        string
	Description string
}

// Server is one MCP server row with resolved category names.
type Server struct {
	ID          uuid.UUID
	Name        string
	Description string
	URL         string
	Categories  []string
}

// Tool is one catalog tool with its server name and inherited categories.
type Tool struct {
	ID          uuid.UUID
	ServerID    uuid.UUID
	Server      string
	Name        string
	Description string
	Categories  []string
	InputSchema json.RawMessage
}

// SearchHit is a ranked tool match from full-text search.
type SearchHit struct {
	Tool
	Score float64
}

// ServerHit is a ranked server-level match from full-text search.
type ServerHit struct {
	Server
	Score float64
}

// SearchScope controls whether Search returns tools, servers, or both.
type SearchScope string

const (
	SearchScopeTool   SearchScope = "tool"
	SearchScopeServer SearchScope = "server"
	SearchScopeBoth   SearchScope = "both"
)

// SearchResult holds ranked hits for the requested scope(s).
type SearchResult struct {
	Tools   []SearchHit
	Servers []ServerHit
}
