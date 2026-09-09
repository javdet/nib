package toolcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/mcpclient"
	"github.com/javdet/nib/internal/mcpconfig"
)

const embedBatchSize = 64

// ServerLister lists MCP servers from mcp.json configuration and expands the
// ${NAME} references in an entry to the values needed to reach the server.
type ServerLister interface {
	ListServers() ([]mcpconfig.Server, error)
	ResolveServer(ctx context.Context, server mcpconfig.Server) (mcpconfig.Resolved, error)
}

// Indexer syncs the Postgres tool catalog from configured MCP servers.
type Indexer struct {
	mu             sync.Mutex
	running        bool
	dirty          bool
	servers        ServerLister
	store          *Store
	embedder       llm.Embedder
	embeddingModel string
	httpClient     *http.Client
	discoverTools  func(ctx context.Context, serverURL string, headers map[string]string, httpClient *http.Client) ([]mcpclient.ToolInfo, error)
	afterIndex     func(ctx context.Context)
}

// NewIndexer wires dependencies for catalog synchronization.
func NewIndexer(
	servers ServerLister,
	store *Store,
	embedder llm.Embedder,
	embeddingModel string,
	httpClient *http.Client,
) *Indexer {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: mcpclient.DefaultListToolsTimeout}
	}
	return &Indexer{
		servers:        servers,
		store:          store,
		embedder:       embedder,
		embeddingModel: strings.TrimSpace(embeddingModel),
		httpClient:     httpClient,
	}
}

// SetAfterIndex registers a callback invoked after every successful reindex,
// including the coalesced rerun. It reads the catalog back rather than being
// handed this run's discoveries: a server that could not be reached is skipped,
// so its tools stay in the catalog and must not look like they were removed.
func (i *Indexer) SetAfterIndex(hook func(ctx context.Context)) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.afterIndex = hook
}

// ReindexAll discovers tools from every configured MCP server and upserts them into the catalog.
// Unreachable servers are skipped with a warning. Overlapping calls coalesce via a dirty flag.
func (i *Indexer) ReindexAll(ctx context.Context) error {
	if i == nil || i.store == nil || i.servers == nil {
		return fmt.Errorf("toolcatalog: indexer not configured")
	}

	i.mu.Lock()
	if i.running {
		i.dirty = true
		i.mu.Unlock()
		return nil
	}
	i.running = true
	afterIndex := i.afterIndex
	i.mu.Unlock()

	defer func() {
		i.mu.Lock()
		rerun := i.dirty
		i.dirty = false
		i.running = false
		i.mu.Unlock()
		if rerun {
			go func() {
				if err := i.ReindexAll(context.Background()); err != nil {
					slog.Warn("tool catalog reindex", "error", err)
				}
			}()
		}
	}()

	if err := i.reindex(ctx); err != nil {
		return err
	}
	if afterIndex != nil {
		afterIndex(ctx)
	}
	return nil
}

func (i *Indexer) reindex(ctx context.Context) error {
	servers, err := i.servers.ListServers()
	if err != nil {
		return fmt.Errorf("list mcp servers: %w", err)
	}

	keepNames := make([]string, 0, len(servers))
	serverCount := 0
	toolTotal := 0

	for _, server := range servers {
		keepNames = append(keepNames, server.Name)

		urlStr := strings.TrimSpace(server.URL)
		if urlStr == "" {
			slog.Warn("tool catalog: skip stdio server", "server", server.Name)
			continue
		}

		// The raw URL is stored so no expanded secret reaches Postgres; only the
		// resolved copy is used to talk to the server.
		desc := strings.TrimSpace(server.Description)
		serverID, err := i.store.UpsertServer(ctx, server.Name, desc, urlStr)
		if err != nil {
			return fmt.Errorf("upsert server %q: %w", server.Name, err)
		}

		resolved, err := i.servers.ResolveServer(ctx, server)
		if err != nil {
			slog.Warn("tool catalog: resolve server variables failed, skipping server",
				"server", server.Name, "error", err)
			continue
		}

		tools, err := i.listServerTools(ctx, resolved.URL, resolved.Headers)
		if err != nil {
			slog.Warn("tool catalog: mcp tools discovery failed, skipping server",
				"server", server.Name, "error", resolved.RedactError(err))
			continue
		}

		embedTexts := make([]string, len(tools))
		for j, t := range tools {
			embedTexts[j] = ToolEmbedText(t.Name, t.Description)
		}
		embeddings := i.embedToolTexts(ctx, embedTexts)

		seen := make([]string, 0, len(tools))
		for j, t := range tools {
			tn := strings.TrimSpace(t.Name)
			if tn == "" {
				slog.Warn("tool catalog: skip tool with empty name", "server", server.Name)
				continue
			}
			params := t.InputSchema
			if len(params) == 0 {
				params = json.RawMessage(`{}`)
			}
			var emb []float32
			if j < len(embeddings) {
				emb = embeddings[j]
			}
			if err := i.store.UpsertTool(ctx, serverID, tn, t.Description, params, emb, i.embeddingModel); err != nil {
				return fmt.Errorf("upsert tool %q/%q: %w", server.Name, tn, err)
			}
			seen = append(seen, tn)
		}
		if err := i.store.DeleteToolsNotIn(ctx, serverID, seen); err != nil {
			return fmt.Errorf("delete stale tools for %q: %w", server.Name, err)
		}

		toolTotal += len(seen)
		serverCount++
		slog.Info("tool catalog: indexed server", "server", server.Name, "tools", len(seen))
	}

	if err := i.store.DeleteServersNotIn(ctx, keepNames); err != nil {
		return fmt.Errorf("delete stale servers: %w", err)
	}

	if err := i.store.RecomputeToolCategories(ctx); err != nil {
		return fmt.Errorf("recompute tool categories: %w", err)
	}

	slog.Info("tool catalog: reindex done", "servers", serverCount, "tools", toolTotal)
	return nil
}

func (i *Indexer) embedToolTexts(ctx context.Context, texts []string) [][]float32 {
	out := make([][]float32, len(texts))
	if i.embedder == nil || len(texts) == 0 {
		return out
	}

	for start := 0; start < len(texts); start += embedBatchSize {
		end := start + embedBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[start:end]
		vecs, _, err := i.embedder.Embed(ctx, batch)
		if err != nil {
			slog.Warn("tool catalog: embed batch failed, storing without embeddings",
				"batchStart", start, "error", err)
			continue
		}
		if len(vecs) != len(batch) {
			slog.Warn("tool catalog: embedder returned unexpected vector count",
				"got", len(vecs), "want", len(batch))
			continue
		}
		copy(out[start:end], vecs)
	}
	return out
}

func (i *Indexer) listServerTools(ctx context.Context, serverURL string, headers map[string]string) ([]mcpclient.ToolInfo, error) {
	if i.discoverTools != nil {
		return i.discoverTools(ctx, serverURL, headers, i.httpClient)
	}
	return mcpclient.ListToolsFromURLWithHTTPClient(ctx, serverURL, headers, i.httpClient)
}

// ToolEmbedText builds the text embedded for a catalog tool row.
func ToolEmbedText(name, description string) string {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if description == "" {
		return name
	}
	if name == "" {
		return description
	}
	return name + "\n" + description
}

