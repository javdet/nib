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
//
// The name is lowercased because dialog category lists are (see
// service.normalizeTagList) and the two are matched against each other: a
// category stored as "Kubernetes" would be invisible to every dialog.
func (s *Store) UpsertCategory(ctx context.Context, name, description string) (Category, error) {
	name = CanonicalCategoryName(name)
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
	url = EXCLUDED.url,
	last_seen_at = now()
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
//
// embeddingModel is stored alongside the vector so a model change is detectable:
// without it, vectors from two different spaces accumulate in one column and
// searches quietly get worse with nothing to point at.
func (s *Store) UpsertTool(ctx context.Context, serverID uuid.UUID, name, description string, inputSchema json.RawMessage, embedding []float32, embeddingModel string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("toolcatalog: tool name is required")
	}
	if inputSchema == nil {
		inputSchema = json.RawMessage(`{}`)
	}

	// The model is recorded only when a vector actually was: a NULL embedding
	// with a model name would look like a successful embed at that model.
	var vec, model any
	if len(embedding) > 0 {
		vec = pgvector.NewVector(embedding)
		model = strings.TrimSpace(embeddingModel)
	}

	const q = `
INSERT INTO mcp_tools (server_id, name, description, input_schema, embedding, embedding_model)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (server_id, name) DO UPDATE SET
	description = EXCLUDED.description,
	input_schema = EXCLUDED.input_schema,
	embedding = EXCLUDED.embedding,
	embedding_model = EXCLUDED.embedding_model,
	last_seen_at = now()`

	if _, err := s.pool.Exec(ctx, q, serverID, name, description, inputSchema, vec, model); err != nil {
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
