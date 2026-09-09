package toolcatalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CategoryWithPatterns is a category with its assignment patterns and matched tool count.
type CategoryWithPatterns struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Patterns    []string `json:"patterns"`
	ToolCount   int      `json:"toolCount"`
}

// MatchesPattern reports whether toolName matches pattern (exact or prefix*), case-insensitive.
func MatchesPattern(pattern, toolName string) bool {
	pattern = strings.TrimSpace(pattern)
	toolName = strings.TrimSpace(toolName)
	if pattern == "" || toolName == "" {
		return false
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		if prefix == "" {
			return true
		}
		return strings.HasPrefix(strings.ToLower(toolName), strings.ToLower(prefix))
	}
	return strings.EqualFold(toolName, pattern)
}

// ListCategoriesWithPatterns returns every category with patterns and matched tool counts.
func (s *Store) ListCategoriesWithPatterns(ctx context.Context) ([]CategoryWithPatterns, error) {
	const q = `
SELECT c.name, c.description,
       COALESCE(array_agg(p.pattern ORDER BY p.pattern) FILTER (WHERE p.pattern IS NOT NULL), '{}') AS patterns,
       COALESCE(tc.tool_count, 0) AS tool_count
FROM tool_categories c
LEFT JOIN tool_category_patterns p ON p.category_id = c.id
LEFT JOIN (
	SELECT category_id, count(DISTINCT tool_id) AS tool_count
	FROM mcp_tool_categories
	GROUP BY category_id
) tc ON tc.category_id = c.id
GROUP BY c.id, c.name, c.description, tc.tool_count
ORDER BY c.name`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("toolcatalog: list categories with patterns: %w", err)
	}
	defer rows.Close()

	var out []CategoryWithPatterns
	for rows.Next() {
		var item CategoryWithPatterns
		var patterns []string
		if err := rows.Scan(&item.Name, &item.Description, &patterns, &item.ToolCount); err != nil {
			return nil, fmt.Errorf("toolcatalog: scan category with patterns: %w", err)
		}
		if patterns == nil {
			patterns = []string{}
		}
		item.Patterns = patterns
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("toolcatalog: list categories rows: %w", err)
	}
	if out == nil {
		out = []CategoryWithPatterns{}
	}
	return out, nil
}

// ReplaceCategoryPatterns replaces all patterns for a category and recomputes tool assignments.
func (s *Store) ReplaceCategoryPatterns(ctx context.Context, categoryName string, patterns []string) error {
	categoryName = CanonicalCategoryName(categoryName)
	if categoryName == "" {
		return fmt.Errorf("toolcatalog: category name is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("toolcatalog: begin replace patterns: %w", err)
	}

	var catID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM tool_categories WHERE lower(name) = $1`, categoryName).Scan(&catID)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("toolcatalog: unknown category %q", categoryName)
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("toolcatalog: resolve category %q: %w", categoryName, err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM tool_category_patterns WHERE category_id = $1`, catID); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("toolcatalog: clear patterns: %w", err)
	}

	seen := make(map[string]struct{}, len(patterns))
	for _, raw := range patterns {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		if _, err := tx.Exec(ctx,
			`INSERT INTO tool_category_patterns (category_id, pattern) VALUES ($1, $2)`,
			catID, p,
		); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("toolcatalog: insert pattern %q: %w", p, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("toolcatalog: commit replace patterns: %w", err)
	}

	return s.RecomputeToolCategories(ctx)
}

// RecomputeToolCategories rebuilds mcp_tool_categories from all patterns and tools.
func (s *Store) RecomputeToolCategories(ctx context.Context) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("toolcatalog: begin recompute categories: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM mcp_tool_categories`); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("toolcatalog: clear tool categories: %w", err)
	}

	const insertQ = `
INSERT INTO mcp_tool_categories (tool_id, category_id)
SELECT DISTINCT t.id, p.category_id
FROM mcp_tools t
JOIN tool_category_patterns p ON (
	(p.pattern NOT LIKE '%*' AND lower(t.name) = lower(p.pattern))
	OR (p.pattern LIKE '%*'
		AND left(lower(t.name), length(p.pattern) - 1)
			= left(lower(p.pattern), length(p.pattern) - 1))
)
ON CONFLICT DO NOTHING`

	if _, err := tx.Exec(ctx, insertQ); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("toolcatalog: insert tool categories: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("toolcatalog: commit recompute categories: %w", err)
	}
	return nil
}

// CanonicalCategoryName is the single spelling of a category name: trimmed and
// lowercased. Dialog category lists are normalized the same way, and the two are
// matched against each other, so both sides have to agree on the form.
//
// It is what the tool_categories_name_lower_key unique index enforces.
func CanonicalCategoryName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// DeleteCategoriesNotIn removes categories whose names are not in keepNames.
// Compared canonically, so a row that predates the lowercasing is matched by the
// keep list rather than deleted as unknown.
func (s *Store) DeleteCategoriesNotIn(ctx context.Context, keepNames []string) error {
	const q = `
DELETE FROM tool_categories
WHERE NOT (lower(name) = ANY($1::text[]))`

	keep := make([]string, 0, len(keepNames))
	for _, name := range keepNames {
		keep = append(keep, CanonicalCategoryName(name))
	}

	if _, err := s.pool.Exec(ctx, q, keep); err != nil {
		return fmt.Errorf("toolcatalog: delete categories not in: %w", err)
	}
	return nil
}

// ListToolsByCategory returns tools assigned to the given category name.
func (s *Store) ListToolsByCategory(ctx context.Context, categoryName string) ([]CatalogTool, error) {
	categoryName = CanonicalCategoryName(categoryName)
	if categoryName == "" {
		return nil, fmt.Errorf("toolcatalog: category name is required")
	}

	const q = `
SELECT s.name AS server_name, t.name, t.description,
       COALESCE(array_remove(array_agg(c.name ORDER BY c.name), NULL), '{}') AS categories
FROM mcp_tools t
JOIN mcp_servers s ON s.id = t.server_id
JOIN mcp_tool_categories tc ON tc.tool_id = t.id
JOIN tool_categories cat ON cat.id = tc.category_id AND lower(cat.name) = $1
LEFT JOIN mcp_tool_categories tc2 ON tc2.tool_id = t.id
LEFT JOIN tool_categories c ON c.id = tc2.category_id
GROUP BY s.name, t.name, t.description
ORDER BY s.name, t.name`

	return s.scanCatalogTools(ctx, q, categoryName)
}

// ListUncategorizedTools returns tools with no category assignment.
func (s *Store) ListUncategorizedTools(ctx context.Context) ([]CatalogTool, error) {
	const q = `
SELECT s.name AS server_name, t.name, t.description, NULL::text[] AS categories
FROM mcp_tools t
JOIN mcp_servers s ON s.id = t.server_id
WHERE NOT EXISTS (
	SELECT 1 FROM mcp_tool_categories tc WHERE tc.tool_id = t.id
)
ORDER BY s.name, t.name`

	return s.scanCatalogTools(ctx, q)
}

func (s *Store) scanCatalogTools(ctx context.Context, query string, args ...any) ([]CatalogTool, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("toolcatalog: list catalog tools: %w", err)
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
		return nil, fmt.Errorf("toolcatalog: catalog tool rows: %w", err)
	}
	if out == nil {
		out = []CatalogTool{}
	}
	return out, nil
}
