package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Session wraps an active MCP client session with its metadata.
type Session struct {
	session *mcp.ClientSession
	connID  uuid.UUID
}

// Manager manages the lifecycle of MCP client sessions keyed by connection ID.
type Manager struct {
	mu       sync.RWMutex
	sessions map[uuid.UUID]*Session
	client   *mcp.Client
}

func NewManager() *Manager {
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "nib",
		Version: "v0.1.0",
	}, nil)

	return &Manager{
		sessions: make(map[uuid.UUID]*Session),
		client:   client,
	}
}

// Connect establishes an MCP session to a remote server using the given auth scheme.
func (m *Manager) Connect(ctx context.Context, connID uuid.UUID, serverURL, token string, scheme AuthScheme) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.sessions[connID]; ok {
		existing.session.Close()
		delete(m.sessions, connID)
	}

	transport := &mcp.StreamableClientTransport{
		Endpoint: serverURL,
		HTTPClient: &http.Client{
			Transport: newAuthRoundTripper(scheme, token),
		},
	}

	session, err := m.client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("mcp connect to %s: %w", serverURL, err)
	}

	m.sessions[connID] = &Session{
		session: session,
		connID:  connID,
	}

	slog.Info("mcp session established", "connectionID", connID, "serverURL", serverURL)
	return nil
}

// Disconnect closes and removes an MCP session.
func (m *Manager) Disconnect(connID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[connID]; ok {
		s.session.Close()
		delete(m.sessions, connID)
		slog.Info("mcp session closed", "connectionID", connID)
	}
}

// ListTools returns the tools available on the MCP server for the given connection.
func (m *Manager) ListTools(ctx context.Context, connID uuid.UUID) ([]ToolInfo, error) {
	session, err := m.getSession(connID)
	if err != nil {
		return nil, err
	}

	result, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return nil, fmt.Errorf("mcp list tools: %w", err)
	}

	tools := make([]ToolInfo, 0, len(result.Tools))
	for _, t := range result.Tools {
		schemaBytes, _ := json.Marshal(t.InputSchema)
		tools = append(tools, ToolInfo{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schemaBytes,
		})
	}
	return tools, nil
}

// CallTool invokes a named tool on the MCP server.
func (m *Manager) CallTool(ctx context.Context, connID uuid.UUID, toolName string, arguments map[string]any) (*mcp.CallToolResult, error) {
	session, err := m.getSession(connID)
	if err != nil {
		return nil, err
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp call tool %s: %w", toolName, err)
	}
	return result, nil
}

// IsConnected checks if a session exists for the given connection.
func (m *Manager) IsConnected(connID uuid.UUID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.sessions[connID]
	return ok
}

// Close shuts down all active sessions.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, s := range m.sessions {
		s.session.Close()
		delete(m.sessions, id)
	}
}

func (m *Manager) getSession(connID uuid.UUID) (*mcp.ClientSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.sessions[connID]
	if !ok {
		return nil, fmt.Errorf("no active mcp session for connection %s", connID)
	}
	return s.session, nil
}

// ToolInfo is a serializable representation of an MCP tool.
type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}
