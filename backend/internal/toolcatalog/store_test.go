package toolcatalog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
)

// integrationDSN returns the Postgres URL for toolcatalog integration tests.
// Override with NIB_TOOLCATALOG_TEST_DSN (e.g. CI service URL).
func integrationDSN() string {
	if d := strings.TrimSpace(os.Getenv("NIB_TOOLCATALOG_TEST_DSN")); d != "" {
		return d
	}
	return "postgres://nib:nib@127.0.0.1:5432/nib?sslmode=disable"
}

func catalogMigrationPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "migrations", "000006_tool_catalog.up.sql"))
}

func catalogEmbeddingsMigrationPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "migrations", "000016_tool_catalog_embeddings.up.sql"))
}

func catalogPatternsMigrationPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "migrations", "000019_tool_category_patterns.up.sql"))
}

func ensureCatalogSchema(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'mcp_tools'
		)`).Scan(&exists)
	if err != nil {
		t.Fatalf("check mcp_tools: %v", err)
	}
	if !exists {
		sqlBytes, err := os.ReadFile(catalogMigrationPath(t))
		if err != nil {
			t.Fatalf("read migration: %v", err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply tool catalog migration: %v", err)
		}
	}

	var hasEmbedding bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = 'mcp_tools' AND column_name = 'embedding'
		)`).Scan(&hasEmbedding)
	if err != nil {
		t.Fatalf("check embedding column: %v", err)
	}
	if !hasEmbedding {
		sqlBytes, err := os.ReadFile(catalogEmbeddingsMigrationPath(t))
		if err != nil {
			t.Fatalf("read embeddings migration: %v", err)
		}
		if _, err := pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
			t.Fatalf("create vector extension: %v", err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply embeddings migration: %v", err)
		}
	}

	var hasPatterns bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'tool_category_patterns'
		)`).Scan(&hasPatterns)
	if err != nil {
		t.Fatalf("check tool_category_patterns: %v", err)
	}
	if !hasPatterns {
		sqlBytes, err := os.ReadFile(catalogPatternsMigrationPath(t))
		if err != nil {
			t.Fatalf("read patterns migration: %v", err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply patterns migration: %v", err)
		}
	}
}

func newTestCatalogStore(ctx context.Context, t *testing.T) *Store {
	t.Helper()
	poolCfg, err := pgxpool.ParseConfig(integrationDSN())
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvec.RegisterTypes(ctx, conn)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatalf("NewWithConfig: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("postgres not reachable (%v); start toolchain postgres or set NIB_TOOLCATALOG_TEST_DSN", err)
	}
	ensureCatalogSchema(ctx, t, pool)
	return NewWithPool(pool)
}

func truncateCatalogTables(ctx context.Context, t *testing.T, s *Store) {
	t.Helper()
	_, err := s.pool.Exec(ctx, `
		TRUNCATE mcp_tool_categories, tool_category_patterns, mcp_tools, mcp_servers, tool_categories RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate catalog tables: %v", err)
	}
}

func TestListTools_returnsAllOrdered(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	srvA, err := s.UpsertServer(ctx, "alpha", "", "https://a/mcp")
	if err != nil {
		t.Fatalf("UpsertServer alpha: %v", err)
	}
	srvB, err := s.UpsertServer(ctx, "beta", "", "https://b/mcp")
	if err != nil {
		t.Fatalf("UpsertServer beta: %v", err)
	}
	if err := s.UpsertTool(ctx, srvB, "z_tool", "last server first name", json.RawMessage(`{}`), nil); err != nil {
		t.Fatalf("UpsertTool z: %v", err)
	}
	if err := s.UpsertTool(ctx, srvA, "a_tool", "first server", json.RawMessage(`{}`), nil); err != nil {
		t.Fatalf("UpsertTool a: %v", err)
	}
	if err := s.UpsertTool(ctx, srvA, "b_tool", "second name", json.RawMessage(`{}`), nil); err != nil {
		t.Fatalf("UpsertTool b: %v", err)
	}

	got, err := s.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListTools len = %d, want 3", len(got))
	}
	want := []CatalogTool{
		{Server: "alpha", Name: "a_tool", Description: "first server"},
		{Server: "alpha", Name: "b_tool", Description: "second name"},
		{Server: "beta", Name: "z_tool", Description: "last server first name"},
	}
	for i := range want {
		if got[i].Server != want[i].Server || got[i].Name != want[i].Name || got[i].Description != want[i].Description {
			t.Fatalf("got[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestListTools_emptyCatalog(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	got, err := s.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListTools = %#v, want empty slice", got)
	}
}

func TestUpsertCategory_UpsertServer_ReplaceCategories_UpsertTool_Search(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	if _, err := s.UpsertCategory(ctx, "Monitoring", "metrics"); err != nil {
		t.Fatalf("UpsertCategory: %v", err)
	}
	if err := s.ReplaceCategoryPatterns(ctx, "Monitoring", []string{"list_alerts"}); err != nil {
		t.Fatalf("ReplaceCategoryPatterns: %v", err)
	}
	srvID, err := s.UpsertServer(ctx, "grafana", "dashboards", "https://example/mcp")
	if err != nil {
		t.Fatalf("UpsertServer: %v", err)
	}
	schema := json.RawMessage(`{"type":"object"}`)
	if err := s.UpsertTool(ctx, srvID, "list_alerts", "List firing alerts", schema, nil); err != nil {
		t.Fatalf("UpsertTool: %v", err)
	}
	if err := s.RecomputeToolCategories(ctx); err != nil {
		t.Fatalf("RecomputeToolCategories: %v", err)
	}

	res, err := s.Search(ctx, "alerts", nil, nil, 10, SearchScopeTool)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Tools) != 1 {
		t.Fatalf("tools hits = %d, want 1", len(res.Tools))
	}
	h := res.Tools[0]
	if h.Name != "list_alerts" || h.Server != "grafana" {
		t.Errorf("hit = server=%q name=%q, want grafana/list_alerts", h.Server, h.Name)
	}
	if len(h.Categories) != 1 || h.Categories[0] != "Monitoring" {
		t.Errorf("categories = %#v, want [Monitoring]", h.Categories)
	}
	if h.Score <= 0 {
		t.Errorf("score = %f, want > 0", h.Score)
	}
}

func TestSearch_categoryFilter_excludesOtherServer(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	for _, c := range []struct{ name, desc string }{
		{"Monitoring", "m"},
		{"Other", "o"},
	} {
		if _, err := s.UpsertCategory(ctx, c.name, c.desc); err != nil {
			t.Fatalf("UpsertCategory %q: %v", c.name, err)
		}
	}
	if err := s.ReplaceCategoryPatterns(ctx, "Monitoring", []string{"alerts"}); err != nil {
		t.Fatalf("ReplaceCategoryPatterns Monitoring: %v", err)
	}
	if err := s.ReplaceCategoryPatterns(ctx, "Other", []string{"post_message"}); err != nil {
		t.Fatalf("ReplaceCategoryPatterns Other: %v", err)
	}

	gid, err := s.UpsertServer(ctx, "grafana", "g", "https://g/mcp")
	if err != nil {
		t.Fatalf("UpsertServer grafana: %v", err)
	}
	if err := s.UpsertTool(ctx, gid, "alerts", "list alerts", json.RawMessage(`{}`), nil); err != nil {
		t.Fatalf("UpsertTool grafana: %v", err)
	}

	sid, err := s.UpsertServer(ctx, "slack", "s", "https://s/mcp")
	if err != nil {
		t.Fatalf("UpsertServer slack: %v", err)
	}
	if err := s.UpsertTool(ctx, sid, "post_message", "post a slack message", json.RawMessage(`{}`), nil); err != nil {
		t.Fatalf("UpsertTool slack: %v", err)
	}
	if err := s.RecomputeToolCategories(ctx); err != nil {
		t.Fatalf("RecomputeToolCategories: %v", err)
	}

	res, err := s.Search(ctx, "message", nil, []string{"Monitoring"}, 10, SearchScopeTool)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, h := range res.Tools {
		if h.Server == "slack" {
			t.Fatalf("unexpected slack hit when filtering Monitoring: %#v", h)
		}
	}
}

func TestSearch_categoryFilter_strict(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	for _, c := range []struct{ name, desc string }{
		{"common", "shared"},
		{"monitoring", "observability"},
	} {
		if _, err := s.UpsertCategory(ctx, c.name, c.desc); err != nil {
			t.Fatalf("UpsertCategory %q: %v", c.name, err)
		}
	}
	if err := s.ReplaceCategoryPatterns(ctx, "common", []string{"ping_host"}); err != nil {
		t.Fatalf("ReplaceCategoryPatterns common: %v", err)
	}
	if err := s.ReplaceCategoryPatterns(ctx, "monitoring", []string{"gadget_dashboard"}); err != nil {
		t.Fatalf("ReplaceCategoryPatterns monitoring: %v", err)
	}

	commonID, err := s.UpsertServer(ctx, "shared-mcp", "common utilities", "https://common/mcp")
	if err != nil {
		t.Fatalf("UpsertServer common: %v", err)
	}
	if err := s.UpsertTool(ctx, commonID, "ping_host", "gadget connectivity probe", json.RawMessage(`{}`), nil); err != nil {
		t.Fatalf("UpsertTool common: %v", err)
	}

	monID, err := s.UpsertServer(ctx, "prom-mcp", "metrics", "https://prom/mcp")
	if err != nil {
		t.Fatalf("UpsertServer monitoring: %v", err)
	}
	if err := s.UpsertTool(ctx, monID, "gadget_dashboard", "open gadget panels", json.RawMessage(`{}`), nil); err != nil {
		t.Fatalf("UpsertTool monitoring: %v", err)
	}
	if err := s.RecomputeToolCategories(ctx); err != nil {
		t.Fatalf("RecomputeToolCategories: %v", err)
	}

	resFiltered, err := s.Search(ctx, "gadget", nil, []string{"monitoring"}, 10, SearchScopeTool)
	if err != nil {
		t.Fatalf("Search with categories: %v", err)
	}
	if len(resFiltered.Tools) != 1 {
		t.Fatalf("filtered search: got %d hits, want 1 (monitoring only)", len(resFiltered.Tools))
	}
	if resFiltered.Tools[0].Name != "gadget_dashboard" {
		t.Errorf("filtered hit = %q, want gadget_dashboard", resFiltered.Tools[0].Name)
	}

	resAll, err := s.Search(ctx, "gadget", nil, nil, 10, SearchScopeTool)
	if err != nil {
		t.Fatalf("Search without categories: %v", err)
	}
	if len(resAll.Tools) != 2 {
		t.Fatalf("unfiltered search: got %d hits, want 2", len(resAll.Tools))
	}
}

func TestSearch_nameMatchRanksAbove_descriptionOnlyMatch(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	if _, err := s.UpsertCategory(ctx, "Cat", ""); err != nil {
		t.Fatalf("UpsertCategory: %v", err)
	}
	if err := s.ReplaceCategoryPatterns(ctx, "Cat", []string{"gadget_probe"}); err != nil {
		t.Fatalf("ReplaceCategoryPatterns: %v", err)
	}
	srvID, err := s.UpsertServer(ctx, "srv", "server", "https://x/mcp")
	if err != nil {
		t.Fatalf("UpsertServer: %v", err)
	}
	// Query uses english websearch_to_tsquery; tool names use simple, descriptions use english.
	if err := s.UpsertTool(ctx, srvID, "gadget_probe", "utility tool", json.RawMessage(`{}`), nil); err != nil {
		t.Fatalf("UpsertTool 1: %v", err)
	}
	if err := s.UpsertTool(ctx, srvID, "other_tool", "use gadget for batch jobs", json.RawMessage(`{}`), nil); err != nil {
		t.Fatalf("UpsertTool 2: %v", err)
	}

	res, err := s.Search(ctx, "gadget", nil, nil, 10, SearchScopeTool)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Tools) < 2 {
		t.Fatalf("want at least 2 hits, got %d", len(res.Tools))
	}
	if res.Tools[0].Name != "gadget_probe" {
		t.Errorf("first hit name = %q, want gadget_probe (name-weighted rank)", res.Tools[0].Name)
	}
	if res.Tools[0].Score < res.Tools[1].Score {
		t.Errorf("first score %f should be >= second %f", res.Tools[0].Score, res.Tools[1].Score)
	}
}

func TestDeleteToolsNotIn_removesStaleNames(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	srvID, err := s.UpsertServer(ctx, "one", "", "https://1/mcp")
	if err != nil {
		t.Fatalf("UpsertServer: %v", err)
	}
	for _, n := range []string{"keep_a", "keep_b", "drop_c"} {
		if err := s.UpsertTool(ctx, srvID, n, "d", json.RawMessage(`{}`), nil); err != nil {
			t.Fatalf("UpsertTool %q: %v", n, err)
		}
	}
	if err := s.DeleteToolsNotIn(ctx, srvID, []string{"keep_a", "keep_b"}); err != nil {
		t.Fatalf("DeleteToolsNotIn: %v", err)
	}
	var names []string
	rows, err := s.pool.Query(ctx, `SELECT name FROM mcp_tools WHERE server_id = $1 ORDER BY name`, srvID)
	if err != nil {
		t.Fatalf("query names: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if len(names) != 2 || names[0] != "keep_a" || names[1] != "keep_b" {
		t.Fatalf("remaining tools = %#v, want [keep_a keep_b]", names)
	}
}

func TestDeleteToolsNotIn_emptyKeep_deletesAll(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	srvID, err := s.UpsertServer(ctx, "emptykeep", "", "https://e/mcp")
	if err != nil {
		t.Fatalf("UpsertServer: %v", err)
	}
	if err := s.UpsertTool(ctx, srvID, "x", "", json.RawMessage(`{}`), nil); err != nil {
		t.Fatalf("UpsertTool: %v", err)
	}
	if err := s.DeleteToolsNotIn(ctx, srvID, []string{}); err != nil {
		t.Fatalf("DeleteToolsNotIn: %v", err)
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM mcp_tools WHERE server_id = $1`, srvID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("tool count = %d, want 0 after empty keep list", n)
	}
}

func TestSearch_scopeServer(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	if _, err := s.UpsertCategory(ctx, "C", ""); err != nil {
		t.Fatalf("UpsertCategory: %v", err)
	}
	if err := s.ReplaceCategoryPatterns(ctx, "C", []string{"noop"}); err != nil {
		t.Fatalf("ReplaceCategoryPatterns: %v", err)
	}
	srvID, err := s.UpsertServer(ctx, "prometheus", "metrics monitoring alerts", "https://p/mcp")
	if err != nil {
		t.Fatalf("UpsertServer: %v", err)
	}
	if err := s.UpsertTool(ctx, srvID, "noop", "noop", json.RawMessage(`{}`), nil); err != nil {
		t.Fatalf("UpsertTool: %v", err)
	}
	if err := s.RecomputeToolCategories(ctx); err != nil {
		t.Fatalf("RecomputeToolCategories: %v", err)
	}

	res, err := s.Search(ctx, "monitoring", nil, nil, 5, SearchScopeServer)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Servers) != 1 {
		t.Fatalf("server hits = %d, want 1", len(res.Servers))
	}
	if res.Servers[0].Name != "prometheus" {
		t.Errorf("server name = %q", res.Servers[0].Name)
	}
	if len(res.Tools) != 0 {
		t.Errorf("tool scope=server: got %d tool hits, want 0", len(res.Tools))
	}
}

func TestSearch_errors(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	if _, err := s.Search(ctx, "", nil, nil, 10, SearchScopeTool); err == nil {
		t.Fatal("expected error for empty query")
	}
	if _, err := s.Search(ctx, "x", nil, nil, 0, SearchScopeTool); err == nil {
		t.Fatal("expected error for limit < 1")
	}
	if _, err := s.Search(ctx, "x", nil, nil, 1, SearchScope("invalid")); err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestReplaceCategoryPatterns_unknownCategory(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	err := s.ReplaceCategoryPatterns(ctx, "Nope", []string{"tool_*"})
	if err == nil || !strings.Contains(err.Error(), "unknown category") {
		t.Fatalf("expected unknown category error, got: %v", err)
	}
}

func TestUpsertServer_roundTripID(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	id1, err := s.UpsertServer(ctx, "same", "d1", "https://a/mcp")
	if err != nil {
		t.Fatalf("UpsertServer: %v", err)
	}
	id2, err := s.UpsertServer(ctx, "same", "d2", "https://b/mcp")
	if err != nil {
		t.Fatalf("UpsertServer second: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("second upsert changed id: %v vs %v", id1, id2)
	}
	var gotURL string
	if err := s.pool.QueryRow(ctx, `SELECT url FROM mcp_servers WHERE id = $1`, id1).Scan(&gotURL); err != nil {
		t.Fatalf("select url: %v", err)
	}
	if gotURL != "https://b/mcp" {
		t.Errorf("url = %q, want updated https://b/mcp", gotURL)
	}
}

func TestSearch_bothScope_returnsToolsAndServers(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	if _, err := s.UpsertCategory(ctx, "Obs", ""); err != nil {
		t.Fatalf("UpsertCategory: %v", err)
	}
	if err := s.ReplaceCategoryPatterns(ctx, "Obs", []string{"check_health"}); err != nil {
		t.Fatalf("ReplaceCategoryPatterns: %v", err)
	}
	srvID, err := s.UpsertServer(ctx, "observe", "monitoring platform", "https://o/mcp")
	if err != nil {
		t.Fatalf("UpsertServer: %v", err)
	}
	if err := s.UpsertTool(ctx, srvID, "check_health", "health monitoring probe", json.RawMessage(`{}`), nil); err != nil {
		t.Fatalf("UpsertTool: %v", err)
	}
	if err := s.RecomputeToolCategories(ctx); err != nil {
		t.Fatalf("RecomputeToolCategories: %v", err)
	}

	res, err := s.Search(ctx, "monitoring", nil, nil, 5, SearchScopeBoth)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Tools) == 0 {
		t.Fatal("want at least one tool hit")
	}
	if len(res.Servers) == 0 {
		t.Fatal("want at least one server hit")
	}
}

func TestSearch_scan_populatesServerID(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	srvID, err := s.UpsertServer(ctx, "idsrv", "", "https://i/mcp")
	if err != nil {
		t.Fatalf("UpsertServer: %v", err)
	}
	if err := s.UpsertTool(ctx, srvID, "t1", "alpha beta uniquegamma", json.RawMessage(`{}`), nil); err != nil {
		t.Fatalf("UpsertTool: %v", err)
	}
	res, err := s.Search(ctx, "uniquegamma", nil, nil, 5, SearchScopeTool)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Tools) != 1 {
		t.Fatalf("hits = %d", len(res.Tools))
	}
	if res.Tools[0].ServerID == uuid.Nil {
		t.Fatal("ServerID not populated")
	}
}

func testUnitEmbedding(dim int, active int, value float32) []float32 {
	v := make([]float32, dim)
	if active >= 0 && active < dim {
		v[active] = value
	}
	return v
}

func TestDeleteServersNotIn_removesStaleServers(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	if _, err := s.UpsertServer(ctx, "keep", "", "https://keep/mcp"); err != nil {
		t.Fatalf("UpsertServer keep: %v", err)
	}
	if _, err := s.UpsertServer(ctx, "drop", "", "https://drop/mcp"); err != nil {
		t.Fatalf("UpsertServer drop: %v", err)
	}
	if err := s.DeleteServersNotIn(ctx, []string{"keep"}); err != nil {
		t.Fatalf("DeleteServersNotIn: %v", err)
	}
	var names []string
	rows, err := s.pool.Query(ctx, `SELECT name FROM mcp_servers ORDER BY name`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		names = append(names, n)
	}
	if len(names) != 1 || names[0] != "keep" {
		t.Fatalf("remaining servers = %#v, want [keep]", names)
	}
}

func TestSearch_hybrid_vectorMatch(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	srvID, err := s.UpsertServer(ctx, "vec-srv", "", "https://v/mcp")
	if err != nil {
		t.Fatalf("UpsertServer: %v", err)
	}
	vecA := testUnitEmbedding(1536, 0, 1)
	vecB := testUnitEmbedding(1536, 1, 1)
	if err := s.UpsertTool(ctx, srvID, "alpha_tool", "unrelated text", json.RawMessage(`{}`), vecA); err != nil {
		t.Fatalf("UpsertTool alpha: %v", err)
	}
	if err := s.UpsertTool(ctx, srvID, "beta_tool", "other unrelated text", json.RawMessage(`{}`), vecB); err != nil {
		t.Fatalf("UpsertTool beta: %v", err)
	}

	res, err := s.Search(ctx, "unrelated", vecA, nil, 5, SearchScopeTool)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Tools) == 0 {
		t.Fatal("want hybrid hits")
	}
	if res.Tools[0].Name != "alpha_tool" {
		t.Fatalf("first hit = %q, want alpha_tool", res.Tools[0].Name)
	}
}
