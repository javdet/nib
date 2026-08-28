package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/mcpclient"
	"github.com/javdet/nib/internal/oauth"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
)

// MCPService orchestrates MCP connection CRUD, OAuth flows, and live MCP sessions.
type MCPService struct {
	repo       repository.MCPRepository
	manager    *mcpclient.Manager
	providers  map[string]oauth.Provider
	stateStore *oauth.StateStore
}

func NewMCPService(
	repo repository.MCPRepository,
	manager *mcpclient.Manager,
	providers []oauth.Provider,
	stateStore *oauth.StateStore,
) *MCPService {
	pm := make(map[string]oauth.Provider, len(providers))
	for _, p := range providers {
		pm[p.Type()] = p
	}
	return &MCPService{
		repo:       repo,
		manager:    manager,
		providers:  pm,
		stateStore: stateStore,
	}
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

// InitiateOAuth starts the OAuth 2.1 + PKCE flow for the given provider type.
// Returns the authorization URL to redirect the user to.
func (s *MCPService) InitiateOAuth(providerType string) (authURL string, err error) {
	provider, ok := s.providers[providerType]
	if !ok {
		return "", fmt.Errorf("unknown provider: %s", providerType)
	}

	state, err := oauth.GenerateState()
	if err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}

	verifier, err := oauth.GenerateCodeVerifier()
	if err != nil {
		return "", fmt.Errorf("generate pkce verifier: %w", err)
	}

	s.stateStore.Put(state, oauth.PKCEParams{
		CodeVerifier: verifier,
		CreatedAt:    time.Now(),
	})

	challenge := oauth.CodeChallengeS256(verifier)
	return provider.AuthCodeURL(state, challenge), nil
}

// HandleOAuthCallback processes the OAuth callback, exchanges the code for tokens,
// creates the MCP connection record, and establishes the MCP session.
func (s *MCPService) HandleOAuthCallback(ctx context.Context, providerType, state, code string) (domain.MCPConnection, error) {
	provider, ok := s.providers[providerType]
	if !ok {
		return domain.MCPConnection{}, fmt.Errorf("unknown provider: %s", providerType)
	}

	params, found := s.stateStore.Pop(state)
	if !found {
		return domain.MCPConnection{}, fmt.Errorf("invalid or expired oauth state")
	}

	tokens, err := provider.Exchange(ctx, code, params.CodeVerifier)
	if err != nil {
		return domain.MCPConnection{}, fmt.Errorf("oauth exchange: %w", err)
	}

	conn := domain.MCPConnection{
		Type:           providerType,
		Name:           providerType,
		ServerURL:      provider.DefaultServerURL(),
		AuthMethod:     "oauth2",
		AccessToken:    tokens.AccessToken,
		RefreshToken:   tokens.RefreshToken,
		TokenExpiresAt: tokens.ExpiresAt,
		Status:         "connecting",
	}

	result, err := s.repo.Create(ctx, conn)
	if err != nil {
		return domain.MCPConnection{}, fmt.Errorf("create connection: %w", err)
	}

	if err := s.manager.Connect(ctx, result.ID, result.ServerURL, tokens.AccessToken, mcpclient.AuthSchemeBearer); err != nil {
		slog.Error("failed to establish mcp session after oauth", "error", err, "connectionID", result.ID)
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
func (s *MCPService) UpdateConnection(ctx context.Context, id uuid.UUID, name, serverURL, apiToken string, metadata json.RawMessage) (domain.MCPConnection, error) {
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

// Reconnect tries to re-establish an MCP session for an existing connection,
// refreshing OAuth tokens if needed.
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
	} else if conn.RefreshToken != "" {
		provider, ok := s.providers[conn.Type]
		if ok {
			refreshed, err := provider.Refresh(ctx, conn.RefreshToken)
			if err != nil {
				slog.Warn("token refresh failed, using existing token", "error", err, "connectionID", id)
			} else {
				token = refreshed.AccessToken
				_ = s.repo.UpdateTokens(ctx, id, refreshed.AccessToken, refreshed.RefreshToken, refreshed.ExpiresAt)
			}
		}
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
			return nil, fmt.Errorf("reconnect for list tools: %w", err)
		}
	}
	return s.manager.ListTools(ctx, connID)
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
