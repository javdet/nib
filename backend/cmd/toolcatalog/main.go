// Command toolcatalog maintains the Postgres MCP tool catalog (reindex from mcp.json).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/javdet/nib/internal/config"
	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/mcpclient"
	"github.com/javdet/nib/internal/mcpconfig"
	"github.com/javdet/nib/internal/toolcatalog"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
)

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: toolcatalog reindex [flags]")
		return 2
	}
	switch os.Args[1] {
	case "reindex":
		return runReindex(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "toolcatalog: unknown subcommand %q\n", os.Args[1])
		return 2
	}
}

func runReindex(args []string) int {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	fs := flag.NewFlagSet("toolcatalog reindex", flag.ContinueOnError)
	var (
		configPath  string
		mcpJSONPath string
		dsnFlag     string
		timeout     time.Duration
	)
	fs.StringVar(&configPath, "config", "", "YAML config (default mcp.json path, DSN, optional HTTP timeout)")
	fs.StringVar(&mcpJSONPath, "mcp-json", "", "path to Cursor-style mcp.json (default: {DATA_DIR}/mcp.json from -config)")
	fs.StringVar(&dsnFlag, "dsn", "", "Postgres DSN (overrides DATABASE_URL and config knowledge_base.uri / env DB)")
	fs.DurationVar(&timeout, "timeout", 0, "HTTP timeout for MCP discovery (default: llm.timeoutSeconds from -config if set, else 60s)")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: toolcatalog reindex [flags]\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		logger.Error("parse flags", "err", err)
		return 1
	}

	configPath = strings.TrimSpace(configPath)

	mcpPath, err := resolveMCPJSONPath(strings.TrimSpace(mcpJSONPath), configPath)
	if err != nil {
		logger.Error("resolve mcp.json", "err", err)
		return 1
	}

	httpTimeout := timeout
	var cfg config.Config
	if httpTimeout <= 0 && configPath != "" {
		loaded, err := config.Load(configPath)
		if err != nil {
			logger.Error("load config", "path", configPath, "err", err)
			return 1
		}
		cfg = loaded
		httpTimeout = time.Duration(cfg.LLM.TimeoutSeconds) * time.Second
	}
	if httpTimeout <= 0 {
		httpTimeout = mcpclient.DefaultListToolsTimeout
	}
	mcpHTTP := &http.Client{Timeout: httpTimeout}

	dsn, err := resolveDSN(dsnFlag, configPath)
	if err != nil {
		logger.Error("resolve dsn", "err", err)
		return 1
	}

	ctx := context.Background()
	pool, err := newPool(ctx, dsn)
	if err != nil {
		logger.Error("database", "err", err)
		return 1
	}
	defer pool.Close()

	store := toolcatalog.NewWithPool(pool)
	mcpSvc := mcpconfig.NewServiceAtPath(mcpPath)

	var embedder llm.Embedder
	embeddingModel := "text-embedding-3-small"
	if configPath != "" {
		if cfg.ConfigPath == "" {
			cfg, err = config.Load(configPath)
			if err != nil {
				logger.Error("load config for embedder", "path", configPath, "err", err)
				return 1
			}
		}
		provider, err := llm.NewFromConfig(cfg.LLM)
		if err != nil {
			logger.Warn("llm provider unavailable, indexing without embeddings", "err", err)
		} else {
			embedder = provider
			embeddingModel = cfg.LLM.EmbeddingModel
		}
	}

	indexer := toolcatalog.NewIndexer(mcpSvc, store, embedder, embeddingModel, mcpHTTP)
	if err := indexer.ReindexAll(ctx); err != nil {
		logger.Error("reindex", "err", err)
		return 1
	}
	return 0
}

func newPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvec.RegisterTypes(ctx, conn)
	}
	return pgxpool.NewWithConfig(ctx, poolCfg)
}

func resolveMCPJSONPath(flagPath, configPath string) (string, error) {
	if flagPath != "" {
		return flagPath, nil
	}
	if configPath == "" {
		return "", fmt.Errorf("set -mcp-json or -config (defaults mcp.json under DATA_DIR)")
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	return mcpconfig.ResolveFile(cfg.DataDir, cfg.MCP.File), nil
}

func resolveDSN(flagDSN, configPath string) (string, error) {
	if s := strings.TrimSpace(flagDSN); s != "" {
		return s, nil
	}
	if s := strings.TrimSpace(os.Getenv("DATABASE_URL")); s != "" {
		return s, nil
	}
	if strings.TrimSpace(configPath) != "" {
		cfg, err := config.Load(configPath)
		if err != nil {
			return "", fmt.Errorf("load config: %w", err)
		}
		return cfg.DatabaseDSN(), nil
	}
	return "", fmt.Errorf("set -dsn, environment DATABASE_URL, or -config with knowledge_base.uri / DB env")
}
