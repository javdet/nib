package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/mcpclient"
	"github.com/javdet/nib/internal/repository"
)

// ErrMCPConnectionUnavailable marks a live MCP session that could not be
// established or used. The handler reports the cause rather than a bare 500,
// which is the difference between "internal server error" in the tools dialog
// and a message naming the rejected credential or unreachable host.
var ErrMCPConnectionUnavailable = errors.New("mcp connection unavailable")

// ErrInvalidMCPMetadata marks a metadata field that is present but is not a JSON
// object, so it cannot be stored in a column every reader treats as one.
var ErrInvalidMCPMetadata = errors.New("mcp connection metadata must be a JSON object")

// ErrMCPTokenRequired is returned when a connection is moved to another origin
// without a new token. The stored token was issued for the old server; sending
// it to whatever URL an update names would hand it to that host.
var ErrMCPTokenRequired = errors.New("a new API token is required when the server URL moves to another origin")

// normalizeMCPMetadata reduces the shapes a metadata field arrives in to the two
// the column allows.
//
// json.RawMessage is a json.Unmarshaler, so a request body of "metadata": null
// arrives as the four bytes `null` -- non-nil, length 4 -- and used to be stored
// verbatim as a JSON null that no reader handles. An absent field returns nil,
// which the update path reads as "leave what is there".
func normalizeMCPMetadata(meta json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(meta)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if string(trimmed) == "null" {
		return json.RawMessage("{}"), nil
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &probe); err != nil {
		return nil, fmt.Errorf("%w", ErrInvalidMCPMetadata)
	}
	return json.RawMessage(trimmed), nil
}

// MCPService orchestrates MCP connection CRUD and live MCP sessions.
type MCPService struct {
	repo    repository.MCPRepository
	manager *mcpclient.Manager
}

func NewMCPService(repo repository.MCPRepository, manager *mcpclient.Manager) *MCPService {
	return &MCPService{repo: repo, manager: manager}
}

func (s *MCPService) ListConnections(ctx context.Context) ([]domain.MCPConnection, error) {
	conns, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	for i := range conns {
		if s.manager.IsConnected(conns[i].ID) {
			conns[i].Status = "connected"
		}
	}
	return conns, nil
}

func (s *MCPService) GetConnection(ctx context.Context, id uuid.UUID) (domain.MCPConnection, error) {
	conn, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.MCPConnection{}, fmt.Errorf("get connection: %w", err)
	}
	if s.manager.IsConnected(conn.ID) {
		conn.Status = "connected"
	}
	return conn, nil
}

// CreateConnectionWithToken creates a new MCP connection that authenticates via API token.
func (s *MCPService) CreateConnectionWithToken(ctx context.Context, connType, name, serverURL, apiToken string, metadata json.RawMessage) (domain.MCPConnection, error) {
	metadata, err := normalizeMCPMetadata(metadata)
	if err != nil {
		return domain.MCPConnection{}, err
	}

	conn := domain.MCPConnection{
		Type:       connType,
		Name:       name,
		ServerURL:  serverURL,
		AuthMethod: "api_token",
		APIToken:   apiToken,
		Status:     "connecting",
		Metadata:   metadata,
	}

	result, err := s.repo.Create(ctx, conn)
	if err != nil {
		return domain.MCPConnection{}, fmt.Errorf("create connection: %w", err)
	}

	if err := s.manager.Connect(ctx, result.ID, serverURL, apiToken, authSchemeForType(connType)); err != nil {
		slog.Error("failed to establish mcp session", "error", err, "connectionID", result.ID)
		_ = s.repo.UpdateStatus(ctx, result.ID, "error")
		result.Status = "error"
		return result, nil
	}

	_ = s.repo.UpdateStatus(ctx, result.ID, "connected")
	result.Status = "connected"
	return result, nil
}

// UpdateConnection updates an existing MCP connection's name, server URL, API token, and metadata.
// If the API token changed, it reconnects the MCP session with the new credentials.
//
// A blank token keeps the stored one only while the URL stays on the same
// origin (scheme, host and port); moving elsewhere takes a new token.
func (s *MCPService) UpdateConnection(ctx context.Context, id uuid.UUID, name, serverURL, apiToken string, metadata json.RawMessage) (domain.MCPConnection, error) {
	metadata, err := normalizeMCPMetadata(metadata)
	if err != nil {
		return domain.MCPConnection{}, err
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.MCPConnection{}, fmt.Errorf("get connection: %w", err)
	}
	if !mcpclient.SameOrigin(existing.ServerURL, serverURL) {
		switch {
		case existing.AuthMethod == "oauth2":
			// The session of an OAuth row authenticates with its access token,
			// which an update cannot replace, so a new token would not help.
			return domain.MCPConnection{}, fmt.Errorf(
				"%w: a connection left over from OAuth cannot move to another server — add a new token connection instead",
				ErrMCPTokenRequired)
		case strings.TrimSpace(apiToken) == "":
			return domain.MCPConnection{}, ErrMCPTokenRequired
		}
	}

	conn := domain.MCPConnection{
		Name:      name,
		ServerURL: serverURL,
		APIToken:  apiToken,
		Metadata:  metadata,
	}

	result, err := s.repo.Update(ctx, id, conn)
	if err != nil {
		return domain.MCPConnection{}, fmt.Errorf("update connection: %w", err)
	}

	s.manager.Disconnect(id)

	token := result.APIToken
	scheme := authSchemeForType(result.Type)
	if result.AuthMethod == "oauth2" {
		token = result.AccessToken
		scheme = mcpclient.AuthSchemeBearer
	}

	if err := s.manager.Connect(ctx, result.ID, result.ServerURL, token, scheme); err != nil {
		slog.Error("failed to re-establish mcp session after update", "error", err, "connectionID", result.ID)
		_ = s.repo.UpdateStatus(ctx, result.ID, "error")
		result.Status = "error"
		return result, nil
	}

	_ = s.repo.UpdateStatus(ctx, result.ID, "connected")
	result.Status = "connected"
	return result, nil
}

func (s *MCPService) DeleteConnection(ctx context.Context, id uuid.UUID) error {
	s.manager.Disconnect(id)
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete connection: %w", err)
	}
	return nil
}

// Reconnect tries to re-establish an MCP session for an existing connection.
// A row left over from the removed OAuth flow connects with the access token it
// holds; nothing refreshes it any more, so an expired one has to be re-added as
// a token connection.
func (s *MCPService) Reconnect(ctx context.Context, id uuid.UUID) error {
	conn, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get connection for reconnect: %w", err)
	}

	token := conn.AccessToken
	scheme := mcpclient.AuthSchemeBearer
	if conn.AuthMethod == "api_token" {
		token = conn.APIToken
		scheme = authSchemeForType(conn.Type)
	}

	if err := s.manager.Connect(ctx, id, conn.ServerURL, token, scheme); err != nil {
		_ = s.repo.UpdateStatus(ctx, id, "error")
		return fmt.Errorf("reconnect mcp session: %w", err)
	}

	_ = s.repo.UpdateStatus(ctx, id, "connected")
	return nil
}

func (s *MCPService) ListTools(ctx context.Context, connID uuid.UUID) ([]mcpclient.ToolInfo, error) {
	if !s.manager.IsConnected(connID) {
		if err := s.Reconnect(ctx, connID); err != nil {
			return nil, fmt.Errorf("%w: reconnect for list tools: %w", ErrMCPConnectionUnavailable, err)
		}
	}
	tools, err := s.manager.ListTools(ctx, connID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMCPConnectionUnavailable, err)
	}
	return tools, nil
}

func (s *MCPService) CallTool(ctx context.Context, connID uuid.UUID, toolName string, arguments map[string]any) (any, error) {
	if !s.manager.IsConnected(connID) {
		if err := s.Reconnect(ctx, connID); err != nil {
			return nil, fmt.Errorf("reconnect for call tool: %w", err)
		}
	}
	return s.manager.CallTool(ctx, connID, toolName, arguments)
}

// Close shuts down all MCP sessions managed by this service.
func (s *MCPService) Close() {
	s.manager.Close()
}

// authSchemeForType returns the appropriate auth scheme for API token connections.
// Jira uses Basic auth; all other providers default to Bearer.
func authSchemeForType(connType string) mcpclient.AuthScheme {
	if connType == "jira" {
		return mcpclient.AuthSchemeBasic
	}
	return mcpclient.AuthSchemeBearer
}
