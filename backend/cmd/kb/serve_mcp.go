package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/javdet/nib/internal/kbstore"
	"github.com/jackc/pgx/v5"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	mcpSearchDefaultLimit = 5
	mcpSearchMaxLimit     = 100
)

type mcpRuntime struct {
	store              *kbstore.Store
	provider           string
	modelOverride      string
	timeout            time.Duration
	embeddingsBaseURL  string
	googleBaseURL      string
	httpReferer        string
	appTitle           string
}

func runServeMCP(args []string) int {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	fs := flag.NewFlagSet("kb serve-mcp", flag.ContinueOnError)
	var (
		configPath        string
		dsnFlag           string
		provider          string
		model             string
		timeout           time.Duration
		embeddingsBaseURL string
		googleBaseURL     string
		httpReferer       string
		appTitle          string
		transport         string
		httpAddr          string
		httpPath          string
		httpStateless     bool
	)
	fs.StringVar(&configPath, "config", "", "YAML config path to read knowledge_base.uri (optional if -dsn or DATABASE_URL is set)")
	fs.StringVar(&dsnFlag, "dsn", "", "Postgres DSN (overrides DATABASE_URL and config)")
	fs.StringVar(&provider, "provider", "openrouter", "embedding provider: openrouter or google (must match how the collection was ingested)")
	fs.StringVar(&model, "model", "", "if set, every search collection must use this embedding_model (otherwise the collection row supplies the model)")
	fs.DurationVar(&timeout, "timeout", 120*time.Second, "HTTP timeout for embedding requests")
	fs.StringVar(&embeddingsBaseURL, "embeddings-base-url", "", "OpenAI-compatible API base URL ($EMBEDDINGS_BASE_URL)")
	fs.StringVar(&embeddingsBaseURL, "openrouter-base-url", "", "Deprecated alias for -embeddings-base-url")
	fs.StringVar(&googleBaseURL, "google-base-url", "", "Google Generative Language API base (default from embed package or $GOOGLE_API_BASE_URL)")
	fs.StringVar(&httpReferer, "http-referer", "", "HTTP-Referer for OpenRouter ($HTTP_REFERER)")
	fs.StringVar(&appTitle, "app-title", "", "X-Title for OpenRouter ($OPENROUTER_APP_TITLE)")
	fs.StringVar(&transport, "transport", "stdio", "MCP transport: stdio or http (streamable HTTP)")
	fs.StringVar(&httpAddr, "http-addr", ":8081", "HTTP listen address when -transport=http (e.g. :8081 or 127.0.0.1:8081)")
	fs.StringVar(&httpPath, "http-path", "/mcp", "HTTP endpoint path when -transport=http")
	fs.BoolVar(&httpStateless, "http-stateless", false, "run streamable HTTP server without session tracking")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: kb serve-mcp [flags]\n")
		fmt.Fprintf(fs.Output(), "Stdio MCP server exposing knowledge_search (embed query + pgvector cosine search).\n\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		logger.Error("parse flags", "err", err)
		return 1
	}

	dsn, err := resolveDSN(dsnFlag, configPath)
	if err != nil {
		logger.Error("resolve dsn", "err", err)
		return 1
	}

	ctx := context.Background()
	store, err := kbstore.New(ctx, dsn)
	if err != nil {
		logger.Error("database", "err", err)
		return 1
	}
	defer store.Close()

	rt := &mcpRuntime{
		store:             store,
		provider:          provider,
		modelOverride:     strings.TrimSpace(model),
		timeout:           timeout,
		embeddingsBaseURL: embeddingsBaseURL,
		googleBaseURL:     googleBaseURL,
		httpReferer:       httpReferer,
		appTitle:          appTitle,
	}

	mcpServer := server.NewMCPServer(
		"nib-kb",
		"1.0.0",
	)

	searchTool := mcp.NewTool("knowledge_search",
		mcp.WithDescription("Embed a natural-language query and return the closest kb_chunks rows from the named collection (cosine similarity via pgvector)."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query text"),
		),
		mcp.WithString("collection",
			mcp.Required(),
			mcp.Description("kb_collections.name"),
		),
		mcp.WithNumber("limit",
			mcp.Description(fmt.Sprintf("Maximum hits (default %d, max %d)", mcpSearchDefaultLimit, mcpSearchMaxLimit)),
		),
	)

	mcpServer.AddTool(searchTool, rt.handleKnowledgeSearch)

	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "", "stdio":
		if err := server.ServeStdio(mcpServer); err != nil {
			logger.Error("mcp server", "err", err)
			return 1
		}
	case "http", "streamable-http", "streamable":
		opts := []server.StreamableHTTPOption{
			server.WithEndpointPath(httpPath),
		}
		if httpStateless {
			opts = append(opts, server.WithStateLess(true))
		}
		httpServer := server.NewStreamableHTTPServer(mcpServer, opts...)
		fmt.Fprintf(os.Stderr, "nib-kb: streamable HTTP listening on %s%s\n", httpAddr, httpPath)
		if err := httpServer.Start(httpAddr); err != nil {
			logger.Error("mcp http server", "err", err)
			return 1
		}
	default:
		logger.Error("unknown -transport", "value", transport)
		return 2
	}
	return 0
}

func (rt *mcpRuntime) handleKnowledgeSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return mcp.NewToolResultError("query is empty"), nil
	}

	collectionName, err := req.RequireString("collection")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	collectionName = strings.TrimSpace(collectionName)
	if collectionName == "" {
		return mcp.NewToolResultError("collection is empty"), nil
	}

	limit := req.GetInt("limit", mcpSearchDefaultLimit)
	if limit < 1 {
		limit = mcpSearchDefaultLimit
	}
	if limit > mcpSearchMaxLimit {
		limit = mcpSearchMaxLimit
	}

	coll, err := rt.store.GetCollectionByName(ctx, collectionName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return mcp.NewToolResultError(fmt.Sprintf("unknown collection %q", collectionName)), nil
		}
		return mcp.NewToolResultError(err.Error()), nil
	}

	if rt.modelOverride != "" && coll.EmbeddingModel != rt.modelOverride {
		return mcp.NewToolResultError(fmt.Sprintf("collection %q uses embedding_model %q, server -model is %q", collectionName, coll.EmbeddingModel, rt.modelOverride)), nil
	}

	embedder, err := buildEmbedder(rt.provider, coll.EmbeddingModel, rt.timeout, rt.embeddingsBaseURL, rt.googleBaseURL, rt.httpReferer, rt.appTitle)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	vecs, _, err := embedder.Embed(ctx, []string{query})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if len(vecs) != 1 {
		return mcp.NewToolResultError("embedder returned unexpected row count"), nil
	}

	hits, err := rt.store.SearchCosine(ctx, coll.ID, vecs[0], limit)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	type row struct {
		Content    string          `json:"content"`
		SourceURI  string          `json:"source_uri"`
		Metadata   json.RawMessage `json:"metadata"`
		Score      float64         `json:"score"`
		Collection string          `json:"collection"`
		Model      string          `json:"embedding_model"`
	}
	out := make([]row, 0, len(hits))
	for _, h := range hits {
		meta := h.Metadata
		if len(meta) == 0 {
			meta = json.RawMessage(`{}`)
		}
		out = append(out, row{
			Content:    h.Content,
			SourceURI:  h.SourceURI,
			Metadata:   meta,
			Score:      h.Score,
			Collection: coll.Name,
			Model:      coll.EmbeddingModel,
		})
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}
