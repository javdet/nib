package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
)

func TestMCPRepoCreate(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewMCPRepo(pool)

	// A nil metadata becomes {}: the column is constrained to a JSON object.
	conn, err := repo.Create(ctx, domain.MCPConnection{
		Type: "token", Name: "grafana", ServerURL: "http://grafana:8000/mcp",
		AuthMethod: "api_token", APIToken: "secret", Status: "connecting",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	assertJSON(t, "Metadata", conn.Metadata, map[string]any{})
	if conn.APIToken != "secret" || conn.AccessToken != "" || conn.RefreshToken != "" {
		t.Fatalf("Create tokens = api %q access %q refresh %q, want only the API token", conn.APIToken, conn.AccessToken, conn.RefreshToken)
	}
	// Unset tokens are stored as NULL rather than as empty strings.
	if n := countRows(t, pool,
		`SELECT count(*) FROM mcp_connections WHERE id = $1 AND access_token IS NULL AND refresh_token IS NULL`, conn.ID); n != 1 {
		t.Fatal("unset OAuth tokens were not stored as NULL")
	}

	if _, err := repo.Create(ctx, domain.MCPConnection{
		Type: "token", Name: "grafana", ServerURL: "http://other/mcp", AuthMethod: "api_token", Status: "connecting",
	}); !errors.Is(err, repository.ErrAlreadyExists) {
		t.Fatalf("duplicate (type, name) Create = %v, want ErrAlreadyExists", err)
	}
	// The same name under another type is a different connection.
	if _, err := repo.Create(ctx, domain.MCPConnection{
		Type: "oauth", Name: "grafana", ServerURL: "http://other/mcp", AuthMethod: "oauth", Status: "connecting",
	}); err != nil {
		t.Fatalf("Create same name, other type: %v", err)
	}

	for name, c := range map[string]domain.MCPConnection{
		"unknown status":      {Type: "token", Name: "a", ServerURL: "u", AuthMethod: "m", Status: "disconnected"},
		"non-object metadata": {Type: "token", Name: "b", ServerURL: "u", AuthMethod: "m", Status: "connected", Metadata: json.RawMessage(`[1]`)},
	} {
		if _, err := repo.Create(ctx, c); err == nil || errors.Is(err, repository.ErrAlreadyExists) {
			t.Errorf("Create(%s) = %v, want a constraint error", name, err)
		}
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 || list[0].ID != conn.ID {
		t.Fatalf("List = %d rows starting %v, want 2 oldest first", len(list), list)
	}
}

func TestMCPRepoUpdateKeepsWhatTheRequestOmits(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewMCPRepo(pool)

	conn, err := repo.Create(ctx, domain.MCPConnection{
		Type: "token", Name: "grafana", ServerURL: "http://grafana:8000/mcp", AuthMethod: "api_token",
		APIToken: "secret", Status: "connected", Metadata: json.RawMessage(`{"team":"sre"}`),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// A blank token and a nil metadata mean "not supplied", not "clear it".
	got, err := repo.Update(ctx, conn.ID, domain.MCPConnection{Name: "grafana-2", ServerURL: "http://grafana:9000/mcp"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Name != "grafana-2" || got.ServerURL != "http://grafana:9000/mcp" {
		t.Fatalf("Update = %+v, want the new name and URL", got)
	}
	if got.APIToken != "secret" {
		t.Errorf("APIToken after a blank update = %q, want the stored one kept", got.APIToken)
	}
	assertJSON(t, "Metadata after an omitted update", got.Metadata, map[string]any{"team": "sre"})

	got, err = repo.Update(ctx, conn.ID, domain.MCPConnection{
		Name: "grafana-2", ServerURL: "http://grafana:9000/mcp", APIToken: "rotated", Metadata: json.RawMessage(`{"team":"ops"}`),
	})
	if err != nil {
		t.Fatalf("Update with values: %v", err)
	}
	if got.APIToken != "rotated" {
		t.Errorf("APIToken = %q, want rotated", got.APIToken)
	}
	assertJSON(t, "Metadata", got.Metadata, map[string]any{"team": "ops"})
	if got.Type != "token" || got.Status != "connected" {
		t.Errorf("Update changed type/status to %q/%q, want them untouched", got.Type, got.Status)
	}

	if err := repo.UpdateStatus(ctx, conn.ID, "error"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	byID, err := repo.GetByID(ctx, conn.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if byID.Status != "error" {
		t.Fatalf("Status = %q, want error", byID.Status)
	}

	if err := repo.Delete(ctx, conn.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	unknown := conn.ID
	for name, err := range map[string]error{
		"GetByID":      errOnly(repo.GetByID(ctx, unknown)),
		"Update":       errOnly(repo.Update(ctx, unknown, domain.MCPConnection{Name: "x", ServerURL: "y"})),
		"UpdateStatus": repo.UpdateStatus(ctx, unknown, "connected"),
		"Delete":       repo.Delete(ctx, uuid.New()),
	} {
		if !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("%s on a missing connection = %v, want ErrNotFound", name, err)
		}
	}
}

// assertJSON compares by value: jsonb does not keep the bytes it was given.
func assertJSON(t *testing.T, what string, raw json.RawMessage, want map[string]any) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("%s = %s: %v", what, raw, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}
