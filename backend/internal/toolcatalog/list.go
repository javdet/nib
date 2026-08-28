package toolcatalog

import (
	"context"
	"fmt"
)

const listToolsSQL = `
SELECT s.name AS server_name, t.name, t.description,
       COALESCE(array_remove(array_agg(c.name ORDER BY c.name), NULL), '{}') AS categories
FROM mcp_tools t
JOIN mcp_servers s ON s.id = t.server_id
LEFT JOIN mcp_tool_categories tc ON tc.tool_id = t.id
LEFT JOIN tool_categories c ON c.id = tc.category_id
GROUP BY s.name, t.name, t.description
ORDER BY s.name, t.name`

// CatalogTool is a lightweight tool row for listing the full catalog.
type CatalogTool struct {
	Server      string   `json:"server"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`
}

// ListTools returns all tools in the catalog ordered by server and name.
func (s *Store) ListTools(ctx context.Context) ([]CatalogTool, error) {
	rows, err := s.pool.Query(ctx, listToolsSQL)
	if err != nil {
		return nil, fmt.Errorf("toolcatalog: list tools: %w", err)
	}
	defer rows.Close()

	var out []CatalogTool
	for rows.Next() {
		var t CatalogTool
		var cats []string
		if err := rows.Scan(&t.Server, &t.Name, &t.Description, &cats); err != nil {
			return nil, fmt.Errorf("toolcatalog: scan catalog tool: %w", err)
		}
		if cats != nil && len(cats) > 0 {
			t.Categories = cats
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("toolcatalog: list tools rows: %w", err)
	}
	if out == nil {
		out = []CatalogTool{}
	}
	return out, nil
}
