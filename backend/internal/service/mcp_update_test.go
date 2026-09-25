package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/mcpclient"
	"github.com/javdet/nib/internal/repository"
)

// fakeMCPRepo holds one connection and records what Update was asked to write.
type fakeMCPRepo struct {
	conn    domain.MCPConnection
	updates []domain.MCPConnection
}

func (f *fakeMCPRepo) List(context.Context) ([]domain.MCPConnection, error) {
	return []domain.MCPConnection{f.conn}, nil
}

func (f *fakeMCPRepo) GetByID(_ context.Context, id uuid.UUID) (domain.MCPConnection, error) {
	if id != f.conn.ID {
		return domain.MCPConnection{}, repository.ErrNotFound
	}
	return f.conn, nil
}

func (f *fakeMCPRepo) Create(_ context.Context, c domain.MCPConnection) (domain.MCPConnection, error) {
	return c, nil
}

// Update mirrors the SQL: a blank token keeps the stored one.
func (f *fakeMCPRepo) Update(_ context.Context, _ uuid.UUID, c domain.MCPConnection) (domain.MCPConnection, error) {
	f.updates = append(f.updates, c)
	f.conn.Name, f.conn.ServerURL = c.Name, c.ServerURL
	if c.APIToken != "" {
		f.conn.APIToken = c.APIToken
	}
	return f.conn, nil
}

func (f *fakeMCPRepo) UpdateStatus(context.Context, uuid.UUID, string) error { return nil }
func (f *fakeMCPRepo) Delete(context.Context, uuid.UUID) error               { return nil }

// The stored token was issued for one server. Keeping it while the URL moved
// sent it straight to the new address.
func TestMCPServiceUpdateConnectionOriginChangeNeedsToken(t *testing.T) {
	t.Parallel()

	// Port 1 on loopback refuses at once, so the reconnect after an accepted
	// update fails fast without leaving the test.
	const stored = "http://127.0.0.1:1/mcp"
	tests := []struct {
		name       string
		authMethod string
		url        string
		token      string
		wantErr    error
	}{
		{"other host, no token", "api_token", "https://evil.test/mcp", "", ErrMCPTokenRequired},
		{"other port, no token", "api_token", "http://127.0.0.1:2/mcp", "", ErrMCPTokenRequired},
		{"other scheme, no token", "api_token", "https://127.0.0.1:1/mcp", "", ErrMCPTokenRequired},
		{"other host, oauth row", "oauth2", "http://127.0.0.2:1/mcp", "new-token", ErrMCPTokenRequired},
		{"other host, new token", "api_token", "http://127.0.0.2:1/mcp", "new-token", nil},
		{"same origin, new path, no token", "api_token", "http://127.0.0.1:1/v2/mcp", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &fakeMCPRepo{conn: domain.MCPConnection{
				ID: uuid.New(), Type: "custom", Name: "gh", ServerURL: stored,
				AuthMethod: tt.authMethod, APIToken: "old-token",
			}}
			svc := NewMCPService(repo, mcpclient.NewManager())

			_, err := svc.UpdateConnection(context.Background(), repo.conn.ID, "gh", tt.url, tt.token, nil)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("UpdateConnection() error = %v, want %v", err, tt.wantErr)
				}
				if len(repo.updates) != 0 {
					t.Fatalf("repo updated %d times, want 0", len(repo.updates))
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateConnection() error = %v", err)
			}
			if repo.conn.ServerURL != tt.url {
				t.Fatalf("ServerURL = %q, want %q", repo.conn.ServerURL, tt.url)
			}
		})
	}
}
