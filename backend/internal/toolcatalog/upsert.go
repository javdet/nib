package toolcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
)

// UpsertCategory inserts or updates a category by unique name.
func (s *Store) UpsertCategory(ctx context.Context, name, description string) (Category, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Category{}, fmt.Errorf("toolcatalog: category name is required")
	}

	const q = `
INSERT INTO tool_categories (name, description)
VALUES ($1, $2)
ON CONFLICT (name) DO UPDATE SET
	description = EXCLUDED.description
RETURNING id, name, description`

	var c Category
	err := s.pool.QueryRow(ctx, q, name, description).Scan(&c.ID, &c.Name, &c.Description)
	if err != nil {
		return Category{}, fmt.Errorf("toolcatalog: upsert category: %w", err)
	}
	return c, nil
}

// UpsertServer inserts or updates an MCP server by unique name.
func (s *Store) UpsertServer(ctx context.Context, name, description, url string) (uuid.UUID, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return uuid.Nil, fmt.Errorf("toolcatalog: server name is required")
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return uuid.Nil, fmt.Errorf("toolcatalog: server url is required")
	}

	const q = `
INSERT INTO mcp_servers (name, description, url)
VALUES ($1, $2, $3)
ON CONFLICT (name) DO UPDATE SET
	description = EXCLUDED.description,
	url = EXCLUDED.url
RETURNING id`

	var id uuid.UUID
	err := s.pool.QueryRow(ctx, q, name, description, url).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("toolcatalog: upsert server: %w", err)
	}
	return id, nil
}

// UpsertTool inserts or updates a tool row for a server.
// embedding may be nil (stored as NULL; tool remains searchable via full-text search).
func (s *Store) UpsertTool(ctx context.Context, serverID uuid.UUID, name, description string, inputSchema json.RawMessage, embedding []float32) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("toolcatalog: tool name is required")
	}
	if inputSchema == nil {
		inputSchema = json.RawMessage(`{}`)
	}

	var vec any
	if len(embedding) > 0 {
		vec = pgvector.NewVector(embedding)
	}

	const q = `
INSERT INTO mcp_tools (server_id, name, description, input_schema, embedding)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (server_id, name) DO UPDATE SET
	description = EXCLUDED.description,
	input_schema = EXCLUDED.input_schema,
	embedding = EXCLUDED.embedding,
	discovered_at = now()`

	if _, err := s.pool.Exec(ctx, q, serverID, name, description, inputSchema, vec); err != nil {
		return fmt.Errorf("toolcatalog: upsert tool: %w", err)
	}
	return nil
}

// DeleteServersNotIn removes MCP servers whose names are not in keepNames.
// An empty keepNames deletes all servers (and their tools via CASCADE).
func (s *Store) DeleteServersNotIn(ctx context.Context, keepNames []string) error {
	const q = `
DELETE FROM mcp_servers
WHERE NOT (name = ANY($1::text[]))`

	if _, err := s.pool.Exec(ctx, q, keepNames); err != nil {
		return fmt.Errorf("toolcatalog: delete servers not in: %w", err)
	}
	return nil
}

// DeleteToolsNotIn removes tools for the server whose names are not in keepNames.
// An empty keepNames deletes all tools for the server.
func (s *Store) DeleteToolsNotIn(ctx context.Context, serverID uuid.UUID, keepNames []string) error {
	const q = `
DELETE FROM mcp_tools
WHERE server_id = $1
  AND NOT (name = ANY($2::text[]))`

	if _, err := s.pool.Exec(ctx, q, serverID, keepNames); err != nil {
		return fmt.Errorf("toolcatalog: delete tools not in: %w", err)
	}
	return nil
}
