package toolcatalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/pgvector/pgvector-go"
)

const rrfK = 60.0

const toolSearchFTSSQL = `
SELECT t.id, t.server_id, t.name, t.description, t.input_schema,
       s.name AS server_name,
       COALESCE(array_remove(array_agg(c.name ORDER BY c.name), NULL), '{}') AS categories,
       ts_rank_cd(t.fts, q) AS score
FROM mcp_tools t
JOIN mcp_servers s ON s.id = t.server_id
LEFT JOIN mcp_tool_categories tc ON tc.tool_id = t.id
LEFT JOIN tool_categories c ON c.id = tc.category_id
CROSS JOIN LATERAL websearch_to_tsquery('english', $1) AS q
WHERE t.fts @@ q
  AND ($2::text[] IS NULL OR EXISTS (
	SELECT 1 FROM mcp_tool_categories tc2
	JOIN tool_categories c2 ON c2.id = tc2.category_id
	WHERE tc2.tool_id = t.id AND c2.name = ANY($2)
  ))
GROUP BY t.id, s.id, q
ORDER BY score DESC
LIMIT $3`

const toolSearchHybridSQL = `
WITH kw AS (
	SELECT t.id,
	       ROW_NUMBER() OVER (ORDER BY ts_rank_cd(t.fts, q) DESC) AS kw_rank
	FROM mcp_tools t
	JOIN mcp_servers s ON s.id = t.server_id
	CROSS JOIN LATERAL websearch_to_tsquery('english', $1) AS q
	WHERE t.fts @@ q
	  AND ($3::text[] IS NULL OR EXISTS (
		SELECT 1 FROM mcp_tool_categories tc2
		JOIN tool_categories c2 ON c2.id = tc2.category_id
		WHERE tc2.tool_id = t.id AND c2.name = ANY($3)
	  ))
),
vec AS (
	SELECT t.id,
	       ROW_NUMBER() OVER (ORDER BY t.embedding <=> $2::vector) AS vec_rank
	FROM mcp_tools t
	JOIN mcp_servers s ON s.id = t.server_id
	WHERE t.embedding IS NOT NULL
	  AND ($3::text[] IS NULL OR EXISTS (
		SELECT 1 FROM mcp_tool_categories tc2
		JOIN tool_categories c2 ON c2.id = tc2.category_id
		WHERE tc2.tool_id = t.id AND c2.name = ANY($3)
	  ))
),
combined AS (
	SELECT COALESCE(kw.id, vec.id) AS id,
	       COALESCE(1.0 / ($4 + kw.kw_rank), 0) + COALESCE(1.0 / ($4 + vec.vec_rank), 0) AS score
	FROM kw
	FULL OUTER JOIN vec ON kw.id = vec.id
)
SELECT t.id, t.server_id, t.name, t.description, t.input_schema,
       s.name AS server_name,
       COALESCE(array_remove(array_agg(c.name ORDER BY c.name), NULL), '{}') AS categories,
       combined.score
FROM combined
JOIN mcp_tools t ON t.id = combined.id
JOIN mcp_servers s ON s.id = t.server_id
LEFT JOIN mcp_tool_categories tc ON tc.tool_id = t.id
LEFT JOIN tool_categories c ON c.id = tc.category_id
GROUP BY t.id, s.id, combined.score
ORDER BY combined.score DESC
LIMIT $5`

const serverSearchFTSSQL = `
SELECT s.id, s.name, s.description, s.url,
       COALESCE(array_remove(array_agg(c.name ORDER BY c.name), NULL), '{}') AS categories,
       ts_rank_cd(s.fts, q) AS score
FROM mcp_servers s
LEFT JOIN mcp_tools t ON t.server_id = s.id
LEFT JOIN mcp_tool_categories tc ON tc.tool_id = t.id
LEFT JOIN tool_categories c ON c.id = tc.category_id
CROSS JOIN LATERAL websearch_to_tsquery('english', $1) AS q
WHERE s.fts @@ q
  AND ($2::text[] IS NULL OR EXISTS (
	SELECT 1 FROM mcp_tools t2
	JOIN mcp_tool_categories tc2 ON tc2.tool_id = t2.id
	JOIN tool_categories c2 ON c2.id = tc2.category_id
	WHERE t2.server_id = s.id AND c2.name = ANY($2)
  ))
GROUP BY s.id, q
ORDER BY score DESC
LIMIT $3`

const serverSearchHybridSQL = `
WITH kw AS (
	SELECT s.id,
	       ROW_NUMBER() OVER (ORDER BY ts_rank_cd(s.fts, q) DESC) AS kw_rank
	FROM mcp_servers s
	CROSS JOIN LATERAL websearch_to_tsquery('english', $1) AS q
	WHERE s.fts @@ q
	  AND ($3::text[] IS NULL OR EXISTS (
		SELECT 1 FROM mcp_tools t2
		JOIN mcp_tool_categories tc2 ON tc2.tool_id = t2.id
		JOIN tool_categories c2 ON c2.id = tc2.category_id
		WHERE t2.server_id = s.id AND c2.name = ANY($3)
	  ))
),
vec AS (
	SELECT s.id,
	       ROW_NUMBER() OVER (ORDER BY MIN(t.embedding <=> $2::vector)) AS vec_rank
	FROM mcp_servers s
	JOIN mcp_tools t ON t.server_id = s.id
	WHERE t.embedding IS NOT NULL
	  AND ($3::text[] IS NULL OR EXISTS (
		SELECT 1 FROM mcp_tool_categories tc2
		JOIN tool_categories c2 ON c2.id = tc2.category_id
		WHERE tc2.tool_id = t.id AND c2.name = ANY($3)
	  ))
	GROUP BY s.id
),
combined AS (
	SELECT COALESCE(kw.id, vec.id) AS id,
	       COALESCE(1.0 / ($4 + kw.kw_rank), 0) + COALESCE(1.0 / ($4 + vec.vec_rank), 0) AS score
	FROM kw
	FULL OUTER JOIN vec ON kw.id = vec.id
)
SELECT s.id, s.name, s.description, s.url,
       COALESCE(array_remove(array_agg(c.name ORDER BY c.name), NULL), '{}') AS categories,
       combined.score
FROM combined
JOIN mcp_servers s ON s.id = combined.id
LEFT JOIN mcp_tools t ON t.server_id = s.id
LEFT JOIN mcp_tool_categories tc ON tc.tool_id = t.id
LEFT JOIN tool_categories c ON c.id = tc.category_id
GROUP BY s.id, combined.score
ORDER BY combined.score DESC
LIMIT $5`

// Search runs full-text and/or hybrid (RRF of FTS + cosine) search over tools and/or servers.
// queryEmbedding enables hybrid ranking; nil falls back to FTS-only.
func (s *Store) Search(ctx context.Context, query string, queryEmbedding []float32, categories []string, limit int, scope SearchScope) (SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return SearchResult{}, fmt.Errorf("toolcatalog: search query is required")
	}
	if limit < 1 {
		return SearchResult{}, fmt.Errorf("toolcatalog: limit must be at least 1")
	}

	switch scope {
	case SearchScopeTool, SearchScopeServer, SearchScopeBoth, "":
	default:
		return SearchResult{}, fmt.Errorf("toolcatalog: invalid search scope %q", scope)
	}
	if scope == "" {
		scope = SearchScopeTool
	}

	catArg := normalizeCategoryArg(categories)
	useHybrid := len(queryEmbedding) > 0

	var out SearchResult
	var err error

	if scope == SearchScopeTool || scope == SearchScopeBoth {
		out.Tools, err = s.searchTools(ctx, query, queryEmbedding, catArg, limit, useHybrid)
		if err != nil {
			return SearchResult{}, err
		}
	}
	if scope == SearchScopeServer || scope == SearchScopeBoth {
		out.Servers, err = s.searchServers(ctx, query, queryEmbedding, catArg, limit, useHybrid)
		if err != nil {
			return SearchResult{}, err
		}
	}
	return out, nil
}

func normalizeCategoryArg(categories []string) any {
	if len(categories) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(categories))
	out := make([]string, 0, len(categories))
	for _, c := range categories {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *Store) searchTools(ctx context.Context, query string, queryEmbedding []float32, catArg any, limit int, useHybrid bool) ([]SearchHit, error) {
	var rows pgxRows
	var err error

	if useHybrid {
		qv := pgvector.NewVector(queryEmbedding)
		rows, err = s.pool.Query(ctx, toolSearchHybridSQL, query, qv, catArg, rrfK, limit)
	} else {
		rows, err = s.pool.Query(ctx, toolSearchFTSSQL, query, catArg, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("toolcatalog: search tools: %w", err)
	}
	defer rows.Close()

	return scanToolHits(rows)
}

func (s *Store) searchServers(ctx context.Context, query string, queryEmbedding []float32, catArg any, limit int, useHybrid bool) ([]ServerHit, error) {
	var rows pgxRows
	var err error

	if useHybrid {
		qv := pgvector.NewVector(queryEmbedding)
		rows, err = s.pool.Query(ctx, serverSearchHybridSQL, query, qv, catArg, rrfK, limit)
	} else {
		rows, err = s.pool.Query(ctx, serverSearchFTSSQL, query, catArg, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("toolcatalog: search servers: %w", err)
	}
	defer rows.Close()

	return scanServerHits(rows)
}

// pgxRows is satisfied by pgx.Rows for scanning helpers.
type pgxRows interface {
	Close()
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanToolHits(rows pgxRows) ([]SearchHit, error) {
	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		var cats []string
		if err := rows.Scan(
			&h.ID, &h.ServerID, &h.Name, &h.Description, &h.InputSchema,
			&h.Server, &cats, &h.Score,
		); err != nil {
			return nil, fmt.Errorf("toolcatalog: scan tool hit: %w", err)
		}
		if len(cats) > 0 {
			h.Categories = cats
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("toolcatalog: tool search rows: %w", err)
	}
	return hits, nil
}

func scanServerHits(rows pgxRows) ([]ServerHit, error) {
	var hits []ServerHit
	for rows.Next() {
		var h ServerHit
		var cats []string
		if err := rows.Scan(
			&h.ID, &h.Name, &h.Description, &h.URL, &cats, &h.Score,
		); err != nil {
			return nil, fmt.Errorf("toolcatalog: scan server hit: %w", err)
		}
		if len(cats) > 0 {
			h.Categories = cats
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("toolcatalog: server search rows: %w", err)
	}
	return hits, nil
}
