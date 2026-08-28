package toolcatalog

import (
	"context"
	"fmt"
	"strings"
)

const getToolByNameSQL = `
SELECT t.id, t.server_id, t.name, t.description, t.input_schema,
       s.name AS server_name,
       COALESCE(array_remove(array_agg(c.name ORDER BY c.name), NULL), '{}') AS categories
FROM mcp_tools t
JOIN mcp_servers s ON s.id = t.server_id
LEFT JOIN mcp_tool_categories tc ON tc.tool_id = t.id
LEFT JOIN tool_categories c ON c.id = tc.category_id
WHERE t.name = $1
GROUP BY t.id, s.id
ORDER BY s.name
LIMIT 1`

// GetToolByName returns the indexed tool with the given exact name. The boolean
// result is false when no such tool exists. When the same name is exported by
// several servers, the first server in alphabetical order wins, matching the
// deterministic duplicate resolution used when building an agent tool catalog.
func (s *Store) GetToolByName(ctx context.Context, name string) (Tool, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Tool{}, false, fmt.Errorf("toolcatalog: tool name is required")
	}

	rows, err := s.pool.Query(ctx, getToolByNameSQL, name)
	if err != nil {
		return Tool{}, false, fmt.Errorf("toolcatalog: get tool %q: %w", name, err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Tool{}, false, fmt.Errorf("toolcatalog: get tool %q: %w", name, err)
		}
		return Tool{}, false, nil
	}

	var t Tool
	var cats []string
	if err := rows.Scan(
		&t.ID, &t.ServerID, &t.Name, &t.Description, &t.InputSchema,
		&t.Server, &cats,
	); err != nil {
		return Tool{}, false, fmt.Errorf("toolcatalog: scan tool %q: %w", name, err)
	}
	if len(cats) > 0 {
		t.Categories = cats
	}
	return t, true, nil
}
