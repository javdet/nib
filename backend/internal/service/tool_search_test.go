package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/javdet/nib/internal/toolcatalog"
)

type fakeCatalogSearcher struct {
	gotQuery           string
	gotQueryEmbedding  []float32
	gotCategories      []string
	gotLimit           int
	gotScope           toolcatalog.SearchScope
	result             toolcatalog.SearchResult
	err                error
	gotLookupName      string
	lookupTool         toolcatalog.Tool
	lookupFound        bool
	lookupErr          error
}

func (f *fakeCatalogSearcher) Search(_ context.Context, query string, queryEmbedding []float32, categories []string, limit int, scope toolcatalog.SearchScope) (toolcatalog.SearchResult, error) {
	f.gotQuery = query
	f.gotQueryEmbedding = queryEmbedding
	f.gotCategories = categories
	f.gotLimit = limit
	f.gotScope = scope
	return f.result, f.err
}

func (f *fakeCatalogSearcher) GetToolByName(_ context.Context, name string) (toolcatalog.Tool, bool, error) {
	f.gotLookupName = name
	return f.lookupTool, f.lookupFound, f.lookupErr
}

type fakeEmbedder struct {
	vecs [][]float32
	err  error
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	out := make([][]float32, len(texts))
	for i, text := range texts {
		out[i] = []float32{float32(len(text))}
	}
	if len(f.vecs) > 0 {
		return f.vecs, len(f.vecs[0]), nil
	}
	return out, 1, nil
}

func TestToolSearchToolDef(t *testing.T) {
	t.Parallel()
	def := ToolSearchToolDef()
	if def.Name != toolcatalog.ToolSearchToolName {
		t.Fatalf("name = %q, want %q", def.Name, toolcatalog.ToolSearchToolName)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(def.Parameters, &schema); err != nil {
		t.Fatalf("unmarshal parameters: %v", err)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "query" {
		t.Fatalf("required = %#v, want [query]", schema.Required)
	}
}

func TestExecuteToolSearch_validation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("nil store", func(t *testing.T) {
		t.Parallel()
		svc := &ToolSearchService{}
		out, err := svc.ExecuteToolSearch(ctx, map[string]any{"query": "x"})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if out != "tool catalog is not configured" {
			t.Fatalf("out = %q", out)
		}
	})

	t.Run("empty query", func(t *testing.T) {
		t.Parallel()
		svc := NewToolSearchService(&fakeCatalogSearcher{}, nil, "")
		out, err := svc.ExecuteToolSearch(ctx, map[string]any{"query": "  "})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if out != "query is empty" {
			t.Fatalf("out = %q", out)
		}
	})

	t.Run("invalid scope", func(t *testing.T) {
		t.Parallel()
		svc := NewToolSearchService(&fakeCatalogSearcher{}, nil, "")
		out, err := svc.ExecuteToolSearch(ctx, map[string]any{
			"query": "pods",
			"scope": "cluster",
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if out != `invalid scope "cluster" (want tool, server, or both)` {
			t.Fatalf("out = %q", out)
		}
	})
}

func TestExecuteToolSearch_argParsing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := &fakeCatalogSearcher{
		result: toolcatalog.SearchResult{
			Tools: []toolcatalog.SearchHit{
				{
					Tool: toolcatalog.Tool{
						Name:        "get_pod",
						Description: "fetch pod",
						Server:      "k8s",
					},
					Score: 0.9,
				},
			},
		},
	}
	embedder := &fakeEmbedder{vecs: [][]float32{{0.42}}}
	svc := NewToolSearchService(fake, embedder, "text-embedding-3-small")

	_, err := svc.ExecuteToolSearch(ctx, map[string]any{
		"query":      "  pod status  ",
		"categories": []any{"  k8s  ", "", "obs"},
		"limit":      float64(99),
		"scope":      "tool",
	})
	if err != nil {
		t.Fatalf("ExecuteToolSearch: %v", err)
	}
	if fake.gotQuery != "pod status" {
		t.Errorf("query = %q, want trimmed pod status", fake.gotQuery)
	}
	if len(fake.gotQueryEmbedding) != 1 || fake.gotQueryEmbedding[0] != 0.42 {
		t.Errorf("query embedding = %#v, want [0.42]", fake.gotQueryEmbedding)
	}
	if len(fake.gotCategories) != 2 || fake.gotCategories[0] != "k8s" || fake.gotCategories[1] != "obs" {
		t.Errorf("categories = %#v, want [k8s obs]", fake.gotCategories)
	}
	if fake.gotLimit != toolSearchMaxLimit {
		t.Errorf("limit = %d, want max %d", fake.gotLimit, toolSearchMaxLimit)
	}
	if fake.gotScope != toolcatalog.SearchScopeTool {
		t.Errorf("scope = %q, want tool", fake.gotScope)
	}
}

func TestExecuteToolSearch_embedFailureFallsBackToFTS(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := &fakeCatalogSearcher{result: toolcatalog.SearchResult{}}
	svc := NewToolSearchService(fake, &fakeEmbedder{err: errors.New("embed down")}, "text-embedding-3-small")

	_, err := svc.ExecuteToolSearch(ctx, map[string]any{"query": "pods"})
	if err != nil {
		t.Fatalf("ExecuteToolSearch: %v", err)
	}
	if fake.gotQueryEmbedding != nil {
		t.Fatalf("expected nil embedding fallback, got %#v", fake.gotQueryEmbedding)
	}
}

func TestExecuteToolSearch_outputToolScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := &fakeCatalogSearcher{
		result: toolcatalog.SearchResult{
			Tools: []toolcatalog.SearchHit{
				{
					Tool: toolcatalog.Tool{
						Name:        "list_alerts",
						Description: "alerts",
						Server:      "grafana",
						Categories:  []string{"Monitoring"},
					},
					Score: 1.25,
				},
			},
		},
	}
	svc := NewToolSearchService(fake, nil, "")

	out, err := svc.ExecuteToolSearch(ctx, map[string]any{"query": "alerts"})
	if err != nil {
		t.Fatalf("ExecuteToolSearch: %v", err)
	}
	var rows []toolSearchResponseRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].Server != "grafana" || rows[0].Name != "list_alerts" {
		t.Errorf("row = %+v", rows[0])
	}
	if rows[0].Score != 1.25 {
		t.Errorf("score = %v, want 1.25", rows[0].Score)
	}
}

func TestExecuteToolSearch_outputServerScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := &fakeCatalogSearcher{
		result: toolcatalog.SearchResult{
			Servers: []toolcatalog.ServerHit{
				{
					Server: toolcatalog.Server{
						Name:        "prometheus",
						Description: "metrics",
						URL:         "https://prom/mcp",
						Categories:  []string{"Obs"},
					},
					Score: 0.5,
				},
			},
		},
	}
	svc := NewToolSearchService(fake, nil, "")

	out, err := svc.ExecuteToolSearch(ctx, map[string]any{
		"query": "metrics",
		"scope": "server",
	})
	if err != nil {
		t.Fatalf("ExecuteToolSearch: %v", err)
	}
	var rows []toolSearchServerResponseRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "prometheus" || rows[0].URL != "https://prom/mcp" {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestExecuteToolSearch_outputBothScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := &fakeCatalogSearcher{
		result: toolcatalog.SearchResult{
			Tools: []toolcatalog.SearchHit{
				{Tool: toolcatalog.Tool{Name: "t1", Server: "srv"}, Score: 1},
			},
			Servers: []toolcatalog.ServerHit{
				{Server: toolcatalog.Server{Name: "srv", URL: "https://s/mcp"}, Score: 2},
			},
		},
	}
	svc := NewToolSearchService(fake, nil, "")

	out, err := svc.ExecuteToolSearch(ctx, map[string]any{
		"query": "srv",
		"scope": "both",
	})
	if err != nil {
		t.Fatalf("ExecuteToolSearch: %v", err)
	}
	var both toolSearchBothResponse
	if err := json.Unmarshal([]byte(out), &both); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(both.Tools) != 1 || both.Tools[0].Name != "t1" {
		t.Errorf("tools = %#v", both.Tools)
	}
	if len(both.Servers) != 1 || both.Servers[0].Name != "srv" {
		t.Errorf("servers = %#v", both.Servers)
	}
}
