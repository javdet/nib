package toolcatalog

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/mcpclient"
	"github.com/javdet/nib/internal/mcpconfig"
)

type fakeServerLister struct {
	servers []mcpconfig.Server
	err     error
	// resolver expands a server entry; nil means pass the entry through.
	resolver func(mcpconfig.Server) (mcpconfig.Resolved, error)
}

func (f *fakeServerLister) ListServers() ([]mcpconfig.Server, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.servers, nil
}

func (f *fakeServerLister) ResolveServer(_ context.Context, server mcpconfig.Server) (mcpconfig.Resolved, error) {
	if f.resolver != nil {
		return f.resolver(server)
	}
	return mcpconfig.Resolved{Server: server}, nil
}

type stubEmbedder struct {
	vecs [][]float32
	err  error
}

func (s *stubEmbedder) Embed(_ context.Context, texts []string) ([][]float32, int, error) {
	if s.err != nil {
		return nil, 0, s.err
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		if i < len(s.vecs) {
			out[i] = s.vecs[i]
		} else {
			out[i] = testUnitEmbedding(1536, 0, 1)
		}
	}
	return out, 1536, nil
}

func TestToolEmbedText(t *testing.T) {
	t.Parallel()
	if ToolEmbedText("foo", "") != "foo" {
		t.Fatal("name only")
	}
	if ToolEmbedText("", "bar") != "bar" {
		t.Fatal("description only")
	}
	if ToolEmbedText("foo", "bar") != "foo\nbar" {
		t.Fatal("both")
	}
}

func TestIndexer_skipsUnreachableServer(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	lister := &fakeServerLister{
		servers: []mcpconfig.Server{
			{Name: "bad", ServerEntry: mcpconfig.ServerEntry{URL: "http://127.0.0.1:1", Description: "down"}},
			{Name: "good", ServerEntry: mcpconfig.ServerEntry{URL: "https://good/mcp", Description: "up"}},
		},
	}
	indexer := NewIndexer(lister, s, &stubEmbedder{}, "text-embedding-3-small", &http.Client{})
	indexer.discoverTools = func(_ context.Context, serverURL string, _ map[string]string, _ *http.Client) ([]mcpclient.ToolInfo, error) {
		if strings.Contains(serverURL, "127.0.0.1") {
			return nil, errors.New("connection refused")
		}
		return []mcpclient.ToolInfo{{Name: "ok_tool", Description: "works"}}, nil
	}

	if err := indexer.ReindexAll(ctx); err != nil {
		t.Fatalf("ReindexAll: %v", err)
	}

	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM mcp_tools`).Scan(&count); err != nil {
		t.Fatalf("count tools: %v", err)
	}
	if count != 1 {
		t.Fatalf("tool count = %d, want 1 from reachable server only", count)
	}
}

func TestIndexer_skipsServerWithUnresolvedVariable(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	lister := &fakeServerLister{
		servers: []mcpconfig.Server{
			{Name: "unresolved", ServerEntry: mcpconfig.ServerEntry{
				URL:     "https://unresolved/mcp",
				Headers: map[string]string{"Authorization": "Bearer ${MCP_MISSING_TOKEN}"},
			}},
			{Name: "good", ServerEntry: mcpconfig.ServerEntry{URL: "https://good/mcp"}},
		},
		resolver: func(server mcpconfig.Server) (mcpconfig.Resolved, error) {
			if server.Name == "unresolved" {
				return mcpconfig.Resolved{}, mcpconfig.ErrUnresolvedVariable
			}
			return mcpconfig.Resolved{Server: server}, nil
		},
	}
	indexer := NewIndexer(lister, s, &stubEmbedder{}, "text-embedding-3-small", &http.Client{})
	indexer.discoverTools = func(_ context.Context, _ string, _ map[string]string, _ *http.Client) ([]mcpclient.ToolInfo, error) {
		return []mcpclient.ToolInfo{{Name: "ok_tool", Description: "works"}}, nil
	}

	if err := indexer.ReindexAll(ctx); err != nil {
		t.Fatalf("ReindexAll: %v", err)
	}

	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM mcp_tools`).Scan(&count); err != nil {
		t.Fatalf("count tools: %v", err)
	}
	if count != 1 {
		t.Fatalf("tool count = %d, want 1 from the resolvable server only", count)
	}

	// The unresolved server keeps its row so it stays visible in the UI, and its
	// URL is stored raw so no expanded value reaches Postgres.
	var url string
	if err := s.pool.QueryRow(ctx,
		`SELECT url FROM mcp_servers WHERE name = 'unresolved'`).Scan(&url); err != nil {
		t.Fatalf("select unresolved server: %v", err)
	}
	if url != "https://unresolved/mcp" {
		t.Fatalf("stored url = %q, want the raw configured url", url)
	}
}

func TestIndexer_prunesStaleServersAndTools(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	call := 0
	lister := &fakeServerLister{
		servers: []mcpconfig.Server{
			{Name: "live", ServerEntry: mcpconfig.ServerEntry{URL: "https://live/mcp"}},
		},
	}
	indexer := NewIndexer(lister, s, &stubEmbedder{}, "text-embedding-3-small", &http.Client{})
	indexer.discoverTools = func(_ context.Context, _ string, _ map[string]string, _ *http.Client) ([]mcpclient.ToolInfo, error) {
		call++
		if call == 1 {
			return []mcpclient.ToolInfo{{Name: "keep", Description: "d"}}, nil
		}
		return []mcpclient.ToolInfo{{Name: "new_tool", Description: "d"}}, nil
	}

	if err := indexer.ReindexAll(ctx); err != nil {
		t.Fatalf("first reindex: %v", err)
	}
	if err := indexer.ReindexAll(ctx); err != nil {
		t.Fatalf("second reindex: %v", err)
	}

	var names []string
	rows, err := s.pool.Query(ctx, `SELECT name FROM mcp_tools ORDER BY name`)
	if err != nil {
		t.Fatalf("query tools: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		names = append(names, n)
	}
	if len(names) != 1 || names[0] != "new_tool" {
		t.Fatalf("tools after refresh = %#v, want [new_tool]", names)
	}

	lister.servers = nil
	if err := indexer.ReindexAll(ctx); err != nil {
		t.Fatalf("empty reindex: %v", err)
	}
	var serverCount int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM mcp_servers`).Scan(&serverCount); err != nil {
		t.Fatalf("count servers: %v", err)
	}
	if serverCount != 0 {
		t.Fatalf("server count = %d, want 0 after config cleared", serverCount)
	}
}

func TestIndexer_embedFailure_stillIndexesWithoutEmbeddings(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	lister := &fakeServerLister{
		servers: []mcpconfig.Server{
			{Name: "srv", ServerEntry: mcpconfig.ServerEntry{URL: "https://srv/mcp"}},
		},
	}
	indexer := NewIndexer(lister, s, &stubEmbedder{err: errors.New("embed down")}, "text-embedding-3-small", &http.Client{})
	indexer.discoverTools = func(_ context.Context, _ string, _ map[string]string, _ *http.Client) ([]mcpclient.ToolInfo, error) {
		return []mcpclient.ToolInfo{{Name: "t1", Description: "d"}}, nil
	}
	if err := indexer.ReindexAll(ctx); err != nil {
		t.Fatalf("ReindexAll: %v", err)
	}

	var embedding any
	if err := s.pool.QueryRow(ctx, `SELECT embedding FROM mcp_tools WHERE name = 't1'`).Scan(&embedding); err != nil {
		t.Fatalf("select embedding: %v", err)
	}
	if embedding != nil {
		t.Fatalf("embedding = %#v, want NULL", embedding)
	}
}

func TestIndexer_skipsStdioServer(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	lister := &fakeServerLister{
		servers: []mcpconfig.Server{
			{Name: "stdio-srv", ServerEntry: mcpconfig.ServerEntry{Command: "node", Description: "local"}},
		},
	}
	indexer := NewIndexer(lister, s, nil, "", &http.Client{})
	if err := indexer.ReindexAll(ctx); err != nil {
		t.Fatalf("ReindexAll: %v", err)
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM mcp_servers`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("stdio server should not be indexed, got %d servers", count)
	}
}

func TestIndexer_reindexCoalescesWhileRunning(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	truncateCatalogTables(ctx, t, s)

	lister := &fakeServerLister{
		servers: []mcpconfig.Server{
			{Name: "srv", ServerEntry: mcpconfig.ServerEntry{URL: "https://srv/mcp"}},
		},
	}
	indexer := NewIndexer(lister, s, nil, "", &http.Client{})
	indexer.discoverTools = func(_ context.Context, _ string, _ map[string]string, _ *http.Client) ([]mcpclient.ToolInfo, error) {
		return nil, nil
	}

	indexer.mu.Lock()
	indexer.running = true
	indexer.mu.Unlock()

	if err := indexer.ReindexAll(ctx); err != nil {
		t.Fatalf("ReindexAll while running: %v", err)
	}
	indexer.mu.Lock()
	if !indexer.dirty {
		t.Fatal("expected dirty flag while reindex running")
	}
	indexer.running = false
	indexer.mu.Unlock()
}

func TestIndexer_listServersError(t *testing.T) {
	ctx := context.Background()
	s := newTestCatalogStore(ctx, t)
	indexer := NewIndexer(&fakeServerLister{err: errors.New("list failed")}, s, nil, "", &http.Client{})
	if err := indexer.ReindexAll(ctx); err == nil || !strings.Contains(err.Error(), "list mcp servers") {
		t.Fatalf("err = %v", err)
	}
}
