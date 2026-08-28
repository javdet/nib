package mcpconfig

// ServerEntry is a single MCP server definition in mcp.json.
type ServerEntry struct {
	URL         string            `json:"url,omitempty"`
	Transport   string            `json:"transport,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Description string            `json:"description,omitempty"`
}

// Document is the top-level mcp.json shape.
type Document struct {
	MCPServers map[string]ServerEntry `json:"mcpServers"`
}

// Server is a named MCP server entry returned by the API.
type Server struct {
	Name string `json:"name"`
	ServerEntry
}
