package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/javdet/nib/internal/includedtools"
	"github.com/javdet/nib/internal/config"
	"github.com/javdet/nib/internal/crypto"
	"github.com/javdet/nib/internal/executor"
	"github.com/javdet/nib/internal/handler"
	"github.com/javdet/nib/internal/kb"
	"github.com/javdet/nib/internal/kbdoc"
	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/logging"
	"github.com/javdet/nib/internal/mcpconfig"
	"github.com/javdet/nib/internal/mode"
	"github.com/javdet/nib/internal/mcpclient"
	"github.com/javdet/nib/internal/oauth"
	"github.com/javdet/nib/internal/repository/postgres"
	"github.com/javdet/nib/internal/repository/static"
	"github.com/javdet/nib/internal/rules"
	"github.com/javdet/nib/internal/service"
	"github.com/javdet/nib/internal/skills"
	"github.com/javdet/nib/internal/systemprompts"
	"github.com/javdet/nib/internal/toolcatalog"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
)

func main() {
	if err := run(); err != nil {
		slog.Error("application failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	var configFlag string
	flag.StringVar(&configFlag, "config", "", "path to YAML config file")
	flag.Parse()

	cfg, err := config.Load(resolveConfigPath(configFlag))
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	cleanup, err := logging.Setup(cfg.Log)
	if err != nil {
		return err
	}
	defer cleanup()

	slog.Info("startup",
		"log.enabled", cfg.Log.Enabled,
		"log.file", cfg.Log.File,
		"log.level", cfg.Log.Level,
		"agent.maxIterations", cfg.Agent.MaxIterations,
	)

	initialStore := static.NewStore(cfg.Projects)
	holder := static.NewStoreHolder(initialStore)
	slog.Info("loaded config",
		"configPath", cfg.ConfigPath,
		"projects", len(initialStore.Projects),
		"environments", len(initialStore.Environments),
		"clouds", len(initialStore.Clouds),
		"locations", len(initialStore.Locations),
	)

	cfgManager := config.NewManager(cfg.ConfigPath, cfg.Projects, func(projects []config.ProjectConfig) {
		holder.Swap(static.NewStore(projects))
	})

	ctx := context.Background()

	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseDSN())
	if err != nil {
		return fmt.Errorf("parse database config: %w", err)
	}
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvec.RegisterTypes(ctx, conn)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	slog.Info("connected to database", "dsnSource", cfg.DatabaseDSNSource())

	migrationsPath := os.Getenv("MIGRATIONS_PATH")
	if migrationsPath == "" {
		migrationsPath = "migrations"
	}
	if err := runMigrations(ctx, pool, migrationsPath); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	if err := migratePlanOwnership(ctx, pool, cfg.DataDir); err != nil {
		return fmt.Errorf("migrate plan ownership: %w", err)
	}

	projectRepo := static.NewProjectRepo(holder, cfgManager)
	envRepo := static.NewEnvironmentRepo(holder, cfgManager)
	cloudRepo := static.NewCloudRepo(holder, cfgManager)
	locationRepo := static.NewLocationRepo(holder, cfgManager)
	mcpRepo := postgres.NewMCPRepo(pool)

	projectSvc := service.NewProjectService(projectRepo)
	envSvc := service.NewEnvironmentService(envRepo, projectRepo)
	cloudSvc := service.NewCloudService(cloudRepo, projectRepo)
	locationSvc := service.NewLocationService(locationRepo, cloudRepo)

	mcpManager := mcpclient.NewManager()
	defer mcpManager.Close()

	var oauthProviders []oauth.Provider
	if cfg.OAuth.AtlassianClientID != "" {
		callbackURL := cfg.OAuth.CallbackBaseURL + "/api/v1/mcp/connections/jira/callback"
		jira := oauth.NewAtlassianProvider(oauth.AtlassianConfig{
			ClientID:     cfg.OAuth.AtlassianClientID,
			ClientSecret: cfg.OAuth.AtlassianClientSecret,
			RedirectURL:  callbackURL,
			MCPURL:       cfg.OAuth.AtlassianMCPURL,
		})
		oauthProviders = append(oauthProviders, jira)
		slog.Info("jira oauth provider configured", "callbackURL", callbackURL)
	}

	stateStore := oauth.NewStateStore(10 * time.Minute)
	mcpSvc := service.NewMCPService(mcpRepo, mcpManager, oauthProviders, stateStore)
	defer mcpSvc.Close()

	// Prompts are compiled into the binary: an image built without one must fail to
	// boot rather than silently degrade every chat turn in that mode.
	if err := systemprompts.ValidateEmbeddedDefaults(append(append([]string(nil), mode.Modes...), systemprompts.Auxiliary...)); err != nil {
		return fmt.Errorf("system prompts: %w", err)
	}
	// Same contract for the knowledge base template: every project falls back to
	// it, so an image without one must not start.
	if err := kbdoc.ValidateEmbeddedSkeleton(); err != nil {
		return fmt.Errorf("knowledge base: %w", err)
	}
	systemPromptsSvc := systemprompts.NewService(cfg.DataDir, cfg.Prompts.Dir)
	promptHousekeeping, err := systemPromptsSvc.Prepare()
	if err != nil {
		return fmt.Errorf("system prompts: %w", err)
	}
	rulesSvc := rules.NewService(cfg.DataDir, cfg.Rules.Dir)
	kbDocSvc := kbdoc.NewService(cfg.DataDir, cfg.KnowledgeBaseDir)
	mcpConfigSvc := mcpconfig.NewService(cfg.DataDir, cfg.MCP.File)
	resolvedToolsDir := mode.ResolveDir(cfg.DataDir, cfg.IncludedTools.Dir)
	// The per-mode allow lists ship in the binary and are reconciled onto the
	// data volume here. A failure is not fatal: a mode whose list is missing runs
	// unfiltered, which is worse than the operator's own list but better than a
	// backend that will not start.
	if seeded, err := mode.SeedAllowLists(resolvedToolsDir); err != nil {
		slog.Warn("seed tool allow lists", "dir", resolvedToolsDir, "error", err)
	} else if len(seeded.Created) > 0 || len(seeded.Added) > 0 {
		slog.Info("tool allow lists reconciled",
			"dir", resolvedToolsDir, "created", seeded.Created, "added", seeded.Added)
	}
	includedToolsSvc := includedtools.NewService(resolvedToolsDir)
	executorConfigStore := executor.NewConfigStore(
		cfg.DataDir,
		cfg.Executor.File,
		fmt.Sprintf("http://localhost:%d", cfg.Server.Port),
	)
	executorSvc := executor.NewService(executorConfigStore, executor.Secrets{
		LLMAPIKey:    cfg.Executor.LLMAPIKey,
		LLMModel:     cfg.Executor.LLMModel,
		GitToken:     cfg.Executor.GitToken,
		WebhookToken: cfg.Executor.WebhookToken,
	})
	skillSvc := skills.NewService(cfg.DataDir, cfg.Skills.Dir)
	// Built-in skills are seeded once per data volume, so a fresh install has a
	// usable catalog while operator edits and deletions survive an upgrade. A
	// failure here is not fatal: skills are optional and chat works without them.
	skillSeeding, err := skillSvc.Seed()
	if err != nil {
		slog.Warn("seed built-in skills", "dir", skillSvc.Dir(), "error", err)
	}
	variableRepo := postgres.NewVariableRepo(pool)
	variableSvc := service.NewVariableService(variableRepo)
	companySvc := service.NewCompanyService(variableRepo)
	if err := companySvc.EnsureDefaults(ctx); err != nil {
		slog.Warn("ensure company variables", "error", err)
	}
	if err := variableSvc.EnsureDefaults(ctx); err != nil {
		slog.Warn("ensure builtin variables", "error", err)
	}

	var secretsCipher *crypto.Cipher
	if len(cfg.SecretsEncryptionKey) > 0 {
		secretsCipher, err = crypto.NewCipher(cfg.SecretsEncryptionKey)
		if err != nil {
			return fmt.Errorf("secrets cipher: %w", err)
		}
		slog.Info("secrets encryption configured")
	} else {
		slog.Warn("SECRETS_ENCRYPTION_KEY not set; secrets cannot be written or read, " +
			"so every ${NAME} reference in mcp.json stays unresolved")
	}
	secretRepo := postgres.NewSecretRepo(pool)
	secretSvc := service.NewSecretService(secretRepo, secretsCipher)
	// mcp.json entries reference secrets by name through ${NAME}; the values are
	// substituted only when the backend talks to the MCP server.
	mcpConfigSvc.SetSecretLookup(secretSvc)
	executorSvc.SetSecretLookup(secretSvc)

	dialogRepo := postgres.NewDialogRepo(pool)
	attachmentRepo := postgres.NewAttachmentRepo(pool)
	selectionStore := service.NewSelectionStore()

	llmProvider, err := llm.NewFromConfig(cfg.LLM)
	if err != nil {
		return fmt.Errorf("llm provider: %w", err)
	}
	kbStore := kb.New(pool)
	knowledgeSvc := service.NewKnowledgeService(
		cfg.ConfigPath,
		kb.DefaultCollectionName,
		cfg.LLM.EmbeddingModel,
		kbStore,
		llmProvider,
		kbDocSvc,
	)
	toolCatalogStore := toolcatalog.NewWithPool(pool)
	toolCategorySvc := service.NewToolCategoryService(variableRepo, toolCatalogStore)
	if err := toolCategorySvc.EnsureCategories(ctx); err != nil {
		slog.Warn("ensure tool categories", "error", err)
	}
	toolCatalogIndexer := toolcatalog.NewIndexer(
		mcpConfigSvc,
		toolCatalogStore,
		llmProvider,
		cfg.LLM.EmbeddingModel,
		func() *http.Client {
			t := time.Duration(cfg.LLM.TimeoutSeconds) * time.Second
			if t <= 0 {
				t = mcpclient.DefaultListToolsTimeout
			}
			return &http.Client{Timeout: t}
		}(),
	)
	// Every MCP tool the catalog holds is on in every mode by default, so a
	// server added through the UI is usable without a trip to the Included
	// tools tab and a deleted one leaves no orphan names behind. Reconciling
	// here rather than at chat time keeps the per-turn allow set a file read.
	toolCatalogIndexer.SetAfterIndex(func(ctx context.Context) {
		tools, err := toolCatalogStore.ListTools(ctx)
		if err != nil {
			slog.Warn("sync included tools: list catalog tools", "error", err)
			return
		}
		names := make([]string, 0, len(tools))
		for _, t := range tools {
			names = append(names, t.Name)
		}
		res, err := includedToolsSvc.SyncCatalog(names)
		if err != nil {
			slog.Warn("sync included tools", "error", err)
			return
		}
		if res.Changed() {
			slog.Info("included tools synced with tool catalog",
				"added", res.Added, "removed", res.Removed)
		}
	})
	go func() {
		if err := toolCatalogIndexer.ReindexAll(ctx); err != nil {
			slog.Warn("tool catalog startup reindex", "error", err)
		}
	}()
	toolSearchSvc := service.NewToolSearchService(toolCatalogStore, llmProvider, cfg.LLM.EmbeddingModel)
	dialogSvc := service.NewDialogService(dialogRepo)
	chatSvc := service.NewChatService(
		llmProvider,
		systemPromptsSvc,
		variableRepo,
		selectionStore,
		dialogRepo,
		attachmentRepo,
		mcpSvc,
		mcpConfigSvc,
		includedToolsSvc,
		knowledgeSvc,
		toolSearchSvc,
		toolCategorySvc,
		secretSvc,
		executorSvc,
		skillSvc,
		rulesSvc,
		cfg.LLM.BaseURL,
		cfg.DataDir,
		cfg.IncludedTools.Dir,
		cfg.Agent.MaxIterations,
		service.PlanFanoutConfig{
			Concurrency:        cfg.Agent.PlanFanoutConcurrency,
			StageMaxIterations: cfg.Agent.StageMaxIterations,
			TimeoutMinutes:     cfg.Agent.PlanFanoutTimeoutMinutes,
		},
		service.ActionExecConfig{
			Concurrency:    cfg.Agent.ActionExecConcurrency,
			MaxIterations:  cfg.Agent.ActionExecMaxIterations,
			TimeoutMinutes: cfg.Agent.ActionExecTimeoutMinutes,
		},
	)
	if cfg.Agent.ActionExecConcurrency > 1 {
		slog.Warn("agent.actionExecConcurrency is clamped to 1; one execution runs at a time",
			"configured", cfg.Agent.ActionExecConcurrency)
	}

	// A run interrupted by a restart has nothing left to close it: its goroutine
	// died with the previous process and a container's webhook has nowhere to
	// land. Sweeping them here is what keeps a restart from permanently
	// blocking the one execution slot.
	if rec, err := chatSvc.ReconcileStuckRuns(); err != nil {
		slog.Warn("reconcile stuck runs", "error", err)
	} else if rec.Actions > 0 || rec.Fanouts > 0 {
		slog.Info("closed runs left behind by a previous process",
			"actions", rec.Actions, "fanouts", rec.Fanouts)
	}

	// Both mcp.json edits and secret rotations change what a resolved MCP server
	// looks like, so either has to drop the cached routes and reindex.
	refreshMCPTools := func(reason string) func() {
		return func() {
			chatSvc.InvalidateMCPToolDiscoveryCache()
			go func() {
				if err := toolCatalogIndexer.ReindexAll(context.Background()); err != nil {
					slog.Warn("tool catalog reindex", "reason", reason, "error", err)
				}
			}()
		}
	}
	mcpConfigSvc.SetChangeHook(refreshMCPTools("mcp config change"))
	secretSvc.SetChangeHook(refreshMCPTools("secret change"))
	slog.Info("system prompts configured",
		"dir", systemPromptsSvc.Dir(),
		"editable", systemprompts.EditableName,
		"overridePath", promptHousekeeping.OverridePath,
		"overrideActive", promptHousekeeping.OverrideActive,
		"prunedRedundantOverride", promptHousekeeping.Pruned)
	if len(promptHousekeeping.Stale) > 0 {
		slog.Warn("prompts directory holds files that are no longer used: system prompts now ship with the image and only the discuss override is read from disk",
			"dir", systemPromptsSvc.Dir(), "files", promptHousekeeping.Stale)
	}
	slog.Info("rules configured", "dir", rulesSvc.Dir())
	slog.Info("skills configured",
		"dir", skillSvc.Dir(),
		"builtin", skills.DefaultNames(),
		"seeded", skillSeeding.Created,
		"alreadyPresent", skillSeeding.Skipped)
	slog.Info("mcp config configured", "path", mcpConfigSvc.Path())
	slog.Info("included tools configured",
		"systemDir", resolvedToolsDir,
		"mcpFile", includedToolsSvc.Path(),
		"knownFile", includedToolsSvc.KnownPath())
	slog.Info("llm provider configured",
		"llmEndpoint", cfg.LLM.BaseURL,
		"api", cfg.LLM.API,
		"model", cfg.LLM.Model,
		"embeddingModel", cfg.LLM.EmbeddingModel,
		"reasoningEffort", cfg.LLM.ReasoningEffort,
	)

	router := handler.NewRouter(
		[]string{"http://localhost:5173"},
		projectSvc,
		envSvc,
		cloudSvc,
		locationSvc,
		knowledgeSvc,
		mcpSvc,
		chatSvc,
		systemPromptsSvc,
		rulesSvc,
		mcpConfigSvc,
		includedToolsSvc,
		toolCatalogStore,
		executorSvc,
		skillSvc,
		variableRepo,
		variableSvc,
		secretSvc,
		companySvc,
		dialogSvc,
		selectionStore,
		toolCategorySvc,
		cfg.OAuth.FrontendBaseURL,
		cfg.Executor.WebhookToken,
	)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("starting HTTP server", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http server: %w", err)
		}
		close(errCh)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		slog.Info("received shutdown signal", "signal", sig)
	case err := <-errCh:
		if err != nil {
			return err
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	slog.Info("shutting down HTTP server")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	slog.Info("server stopped gracefully")
	return nil
}

// resolveConfigPath picks the config file: -config flag, then CONFIG_FILE env, then config.yaml.
func resolveConfigPath(flagPath string) string {
	if flagPath != "" {
		return flagPath
	}
	if v := os.Getenv("CONFIG_FILE"); v != "" {
		return v
	}
	return "config.yaml"
}

// runMigrations applies .up.sql files in lexicographic order, tracking applied
// migrations in a schema_migrations table so each file runs at most once.
func runMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("migrations directory not found, skipping", "path", dir)
			return nil
		}
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var upFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e.Name())
		}
	}
	sort.Strings(upFiles)

	for _, name := range upFiles {
		version := strings.TrimSuffix(name, ".up.sql")

		var exists bool
		err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if exists {
			continue
		}

		sql, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}

		if _, err := pool.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			return fmt.Errorf("record migration %s: %w", version, err)
		}

		slog.Info("applied migration", "version", version)
	}

	return nil
}
