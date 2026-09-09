// Command kb provides knowledge-base utilities (ingest documents into pgvector).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/javdet/nib/internal/config"
	"github.com/javdet/nib/internal/embed"

	"github.com/javdet/nib/internal/kb"
)

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: kb ingest [flags] [files...]\n       echo text | kb ingest [flags]\n       kb serve-mcp [flags]")
		return 2
	}
	switch os.Args[1] {
	case "ingest":
		return runIngest(os.Args[2:])
	case "serve-mcp":
		return runServeMCP(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "kb: unknown subcommand %q\n", os.Args[1])
		return 2
	}
}

func runIngest(args []string) int {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	fs := flag.NewFlagSet("kb ingest", flag.ContinueOnError)
	var (
		configPath        string
		dsnFlag           string
		collection        string
		provider          string
		model             string
		chunkSize         int
		chunkOverlap      int
		embedBatch        int
		insertBatch       int
		metric            string
		replace           bool
		timeout           time.Duration
		embeddingsBaseURL string
		googleBaseURL     string
		httpReferer       string
		appTitle          string
	)
	fs.StringVar(&configPath, "config", "", "YAML config path to read knowledge_base.uri (optional if -dsn or DATABASE_URL is set)")
	fs.StringVar(&dsnFlag, "dsn", "", "Postgres DSN (overrides DATABASE_URL and config)")
	fs.StringVar(&collection, "collection", "", "collection name (required)")
	fs.StringVar(&provider, "provider", "openrouter", "embedding provider: openrouter or google")
	fs.StringVar(&model, "model", "", "embedding model id (required)")
	fs.IntVar(&chunkSize, "chunk-size", kb.DefaultChunkSize, "max runes per chunk")
	fs.IntVar(&chunkOverlap, "chunk-overlap", kb.DefaultChunkOverlap, "rune overlap between consecutive chunks")
	fs.IntVar(&embedBatch, "embed-batch", 32, "max texts per embedding API request")
	fs.IntVar(&insertBatch, "insert-batch", 256, "max rows per database insert batch")
	fs.StringVar(&metric, "metric", "cosine", "distance metric label stored on the collection")
	fs.BoolVar(&replace, "replace", false, "delete existing chunks for the same source_uri values before insert")
	fs.DurationVar(&timeout, "timeout", 120*time.Second, "HTTP timeout for embedding requests")
	fs.StringVar(&embeddingsBaseURL, "embeddings-base-url", "", "OpenAI-compatible API base URL ($EMBEDDINGS_BASE_URL)")
	fs.StringVar(&embeddingsBaseURL, "openrouter-base-url", "", "Deprecated alias for -embeddings-base-url")
	fs.StringVar(&googleBaseURL, "google-base-url", "", "Google Generative Language API base (default from embed package or $GOOGLE_API_BASE_URL)")
	fs.StringVar(&httpReferer, "http-referer", "", "HTTP-Referer for OpenRouter ($HTTP_REFERER)")
	fs.StringVar(&appTitle, "app-title", "", "X-Title for OpenRouter ($OPENROUTER_APP_TITLE)")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: kb ingest [flags] [files...]\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		logger.Error("parse flags", "err", err)
		return 1
	}
	if strings.TrimSpace(collection) == "" {
		logger.Error("missing required flag", "flag", "-collection")
		return 1
	}
	if strings.TrimSpace(model) == "" {
		logger.Error("missing required flag", "flag", "-model")
		return 1
	}
	if embedBatch < 1 {
		logger.Error("embed-batch must be at least 1")
		return 1
	}
	if insertBatch < 1 {
		logger.Error("insert-batch must be at least 1")
		return 1
	}

	dsn, err := resolveDSN(dsnFlag, configPath)
	if err != nil {
		logger.Error("resolve dsn", "err", err)
		return 1
	}

	embedder, err := buildEmbedder(provider, model, timeout, embeddingsBaseURL, googleBaseURL, httpReferer, appTitle)
	if err != nil {
		logger.Error("embedder", "err", err)
		return 1
	}

	paths := fs.Args()
	sources, err := loadSources(paths)
	if err != nil {
		logger.Error("load sources", "err", err)
		return 1
	}
	if len(sources) == 0 {
		logger.Error("no input: pass files or stdin")
		return 1
	}

	type row struct {
		source string
		index  int
		text   string
	}
	var rows []row
	for _, src := range sources {
		parts := kb.Chunk(src.content, chunkSize, chunkOverlap)
		if len(parts) == 0 {
			continue
		}
		for i, p := range parts {
			t := strings.TrimSpace(p)
			if t == "" {
				continue
			}
			rows = append(rows, row{source: src.uri, index: i, text: t})
		}
	}
	if len(rows) == 0 {
		logger.Error("no non-empty chunks after splitting")
		return 1
	}

	ctx := context.Background()
	store, err := kb.Open(ctx, dsn)
	if err != nil {
		logger.Error("database", "err", err)
		return 1
	}
	defer store.Close()

	var coll kb.Collection
	dim := 0
	var allChunks []kb.ChunkInput

	for start := 0; start < len(rows); start += embedBatch {
		end := start + embedBatch
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[start:end]
		texts := make([]string, len(batch))
		for i := range batch {
			texts[i] = batch[i].text
		}
		vecs, d, err := embedder.Embed(ctx, texts)
		if err != nil {
			logger.Error("embed", "err", err)
			return 1
		}
		if dim == 0 {
			dim = d
			coll, err = store.UpsertCollection(ctx, collection, dim, model, metric)
			if err != nil {
				if errors.Is(err, kb.ErrCollectionMismatch) {
					logger.Error("collection mismatch", "detail", "existing collection uses different dimensions or model")
				} else {
					logger.Error("upsert collection", "err", err)
				}
				return 1
			}
			logger.Info("collection ready", "id", coll.ID.String(), "dimensions", coll.Dimensions, "model", coll.EmbeddingModel)
		} else if d != dim {
			logger.Error("embedding dimension drift", "got", d, "want", dim)
			return 1
		}
		if len(vecs) != len(batch) {
			logger.Error("embedder row count mismatch", "got", len(vecs), "want", len(batch))
			return 1
		}
		for i := range batch {
			allChunks = append(allChunks, kb.ChunkInput{
				SourceURI:  batch[i].source,
				ChunkIndex: batch[i].index,
				Content:    batch[i].text,
				Metadata:   nil,
				Embedding:  vecs[i],
			})
		}
	}

	uris := uniqueSourceURIs(sources)
	tx, err := store.Begin(ctx)
	if err != nil {
		logger.Error("begin tx", "err", err)
		return 1
	}
	if replace {
		if err := store.DeleteChunksBySourceURIsTx(ctx, tx, coll.ID, uris); err != nil {
			_ = tx.Rollback(ctx)
			logger.Error("delete chunks", "err", err)
			return 1
		}
	}
	for start := 0; start < len(allChunks); start += insertBatch {
		end := start + insertBatch
		if end > len(allChunks) {
			end = len(allChunks)
		}
		if err := store.InsertChunksTx(ctx, tx, coll.ID, allChunks[start:end]); err != nil {
			_ = tx.Rollback(ctx)
			logger.Error("insert chunks", "err", err)
			return 1
		}
	}
	if err := tx.Commit(ctx); err != nil {
		logger.Error("commit", "err", err)
		return 1
	}

	logger.Info("ingest done", "chunks", len(allChunks), "sources", len(uris))
	return 0
}

func resolveDSN(flagDSN, configPath string) (string, error) {
	if s := strings.TrimSpace(flagDSN); s != "" {
		return s, nil
	}
	if s := strings.TrimSpace(os.Getenv("DATABASE_URL")); s != "" {
		return s, nil
	}
	if s := strings.TrimSpace(configPath); s != "" {
		return config.LoadKnowledgeBaseURI(s)
	}
	return "", fmt.Errorf("set -dsn, environment DATABASE_URL, or -config with knowledge_base.uri")
}

func buildEmbedder(provider, model string, timeout time.Duration, embeddingsBase, googleBase, referer, title string) (embed.Embedder, error) {
	p := strings.ToLower(strings.TrimSpace(provider))
	opts := embed.FactoryOptions{Timeout: timeout}
	switch p {
	case "openrouter":
		apiKey := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
		if apiKey == "" {
			return nil, fmt.Errorf("OPENROUTER_API_KEY is required for provider openrouter")
		}
		base := resolveEmbeddingsBaseURL(embeddingsBase)
		opts.OpenRouter = embed.OpenRouterOptions{
			BaseURL:     base,
			Model:       strings.TrimSpace(model),
			APIKey:      apiKey,
			HTTPReferer: firstNonEmpty(referer, os.Getenv("HTTP_REFERER")),
			AppTitle:    firstNonEmpty(title, os.Getenv("OPENROUTER_APP_TITLE")),
			Timeout:     timeout,
		}
	case "google":
		apiKey := firstNonEmpty(os.Getenv("GEMINI_API_KEY"), os.Getenv("GOOGLE_API_KEY"))
		if strings.TrimSpace(apiKey) == "" {
			return nil, fmt.Errorf("GOOGLE_API_KEY or GEMINI_API_KEY is required for provider google")
		}
		base := strings.TrimSpace(googleBase)
		if base == "" {
			base = strings.TrimSpace(os.Getenv("GOOGLE_API_BASE_URL"))
		}
		opts.Google = embed.GoogleOptions{
			BaseURL: base,
			Model:   strings.TrimSpace(model),
			APIKey:  apiKey,
			Timeout: timeout,
		}
	default:
		return nil, fmt.Errorf("unknown provider %q (use openrouter or google)", provider)
	}
	return embed.NewEmbedder(p, opts)
}

func resolveEmbeddingsBaseURL(flagValue string) string {
	if base := strings.TrimSpace(flagValue); base != "" {
		return base
	}
	if base := strings.TrimSpace(os.Getenv("EMBEDDINGS_BASE_URL")); base != "" {
		return base
	}
	return strings.TrimSpace(os.Getenv("OPENROUTER_BASE_URL"))
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(b)
}

type sourceDoc struct {
	uri     string
	content string
}

func loadSources(paths []string) ([]sourceDoc, error) {
	if len(paths) == 0 {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return []sourceDoc{{uri: "stdin", content: string(b)}}, nil
	}
	var out []sourceDoc
	seen := map[string]struct{}{}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "-" {
			b, err := io.ReadAll(os.Stdin)
			if err != nil {
				return nil, fmt.Errorf("read stdin: %w", err)
			}
			out = append(out, sourceDoc{uri: "stdin", content: string(b)})
			continue
		}
		matches, err := filepath.Glob(p)
		if err != nil {
			return nil, fmt.Errorf("glob %q: %w", p, err)
		}
		if len(matches) == 0 && !strings.ContainsAny(p, "*?[") {
			matches = []string{p}
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("no files match %q", p)
		}
		for _, path := range matches {
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			b, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read %q: %w", path, err)
			}
			abs := path
			if a, err := filepath.Abs(path); err == nil {
				abs = a
			}
			out = append(out, sourceDoc{uri: "file://" + filepath.ToSlash(abs), content: string(b)})
		}
	}
	return out, nil
}

func uniqueSourceURIs(srcs []sourceDoc) []string {
	var uris []string
	seen := map[string]struct{}{}
	for _, s := range srcs {
		if _, ok := seen[s.uri]; ok {
			continue
		}
		seen[s.uri] = struct{}{}
		uris = append(uris, s.uri)
	}
	return uris
}
