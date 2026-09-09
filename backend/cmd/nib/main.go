package main

import (
	"context"
	"crypto/sha256"
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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/javdet/nib/internal/config"
	"github.com/javdet/nib/internal/crypto"
	"github.com/javdet/nib/internal/executor"
	"github.com/javdet/nib/internal/handler"
	"github.com/javdet/nib/internal/hostenv"
	"github.com/javdet/nib/internal/includedtools"
	"github.com/javdet/nib/internal/kb"
	"github.com/javdet/nib/internal/kbdoc"
	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/logging"
	"github.com/javdet/nib/internal/mcpclient"
	"github.com/javdet/nib/internal/mcpconfig"
	"github.com/javdet/nib/internal/metrics"
	"github.com/javdet/nib/internal/mode"
	"github.com/javdet/nib/internal/nibdocs"
	"github.com/javdet/nib/internal/oauth"
	"github.com/javdet/nib/internal/repository/postgres"
	"github.com/javdet/nib/internal/repository/static"
	"github.com/javdet/nib/internal/rules"
	"github.com/javdet/nib/internal/service"
	"github.com/javdet/nib/internal/skills"
	"github.com/javdet/nib/internal/systemprompts"
	"github.com/javdet/nib/internal/toolcatalog"
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

	metricsSvc, err := metrics.Setup(metrics.Config{
		Enabled:        cfg.Metrics.Enabled,
		Host:           cfg.Metrics.Host,
		Port:           cfg.Metrics.Port,
		Path:           cfg.Metrics.Path,
		RefreshSeconds: cfg.Metrics.RefreshSeconds,
	})
	if err != nil {
		return err
	}

	slog.Info("startup",
		"log.enabled", cfg.Log.Enabled,
		"log.file", cfg.Log.File,
		"log.level", cfg.Log.Level,
		"agent.maxIterations", cfg.Agent.MaxIterations,
		"metrics.enabled", cfg.Metrics.Enabled,
		"metrics.addr", metricsSvc.Addr(),
		"metrics.path", cfg.Metrics.Path,
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

	if err := metricsSvc.RegisterPool(metrics.PgxPoolStats(pool)); err != nil {
		return err
	}

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
	// And for nib's own documentation: it is what the agent answers questions
	// about configuring nib from, so an image built without it must fail here
	// rather than at the tool call.
	if err := nibdocs.ValidateEmbedded(nibdocs.RepoGuide); err != nil {
		return fmt.Errorf("nib documentation: %w", err)
	}
	if err := skills.ValidateSystemSkills(); err != nil {
		return fmt.Errorf("system skills: %w", err)
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
	// The Environment block of the mode prompts is rendered from these, so they
	// are detected here and reconciled into the variable table: a value nothing
	// has touched follows the host, an operator's edit is left alone.
	hostInfo := hostenv.Detect()
	slog.Info("host environment",
		"runtime", hostInfo.Runtime, "namespace", hostInfo.KubeNamespace,
		"distro", hostInfo.Distro, "kernel", hostInfo.Kernel,
		"platform", hostInfo.GOOS+"/"+hostInfo.GOARCH,
		"shell", hostInfo.Shell, "workdir", hostInfo.WorkDir,
		"uid", hostInfo.UID, "onPath", hostInfo.OnPath)
	hostEnvSvc := service.NewHostEnvService(variableRepo, cfg.DataDir)
	if hostVars, err := hostEnvSvc.EnsureDefaults(ctx, hostInfo); err != nil {
		slog.Warn("ensure host environment variables", "error", err)
	} else if len(hostVars.Created) > 0 || len(hostVars.Refreshed) > 0 || len(hostVars.Overridden) > 0 {
		slog.Info("host environment variables reconciled",
			"created", hostVars.Created, "refreshed", hostVars.Refreshed,
			"operatorOwned", hostVars.Overridden)
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
	// Decorating at this one seam instruments every consumer of the client:
	// the chat service, knowledge, the tool-catalog indexer and tool search all
	// take this same value.
	llmProvider = llm.NewMetered(llmProvider, cfg.LLM.Model)
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
	// chat_attachments cascades away with its dialog and with the messages a
	// retry rewinds, taking the only record of what is on disk with it. An
	// AFTER DELETE trigger queues each path; this drains the queue.
	go service.NewAttachmentSweeper(attachmentRepo, cfg.DataDir).Run(ctx)
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
	} else {
		metrics.AddStuckRunsReconciled(rec.Actions, rec.Fanouts)
		if rec.Actions > 0 || rec.Fanouts > 0 {
			slog.Info("closed runs left behind by a previous process",
				"actions", rec.Actions, "fanouts", rec.Fanouts)
		}
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

	// Registered after `defer pool.Close()` above, so LIFO stops the refresher
	// before the pool it pings is closed.
	stopMetrics := metricsSvc.StartRefresher(ctx, metrics.RefreshSources{
		Plans: chatSvc,
		DB:    pool,
	}, cfg.Metrics.RefreshInterval())
	defer stopMetrics()

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

	// Metrics get a listener of their own: both the ingress and the frontend
	// nginx route only /api to the backend, so a separate port keeps /metrics
	// off the public host while an in-cluster scraper still reaches it.
	var metricsSrv *http.Server
	if metricsSvc.Enabled() {
		mux := http.NewServeMux()
		mux.Handle(metricsSvc.Path(), metricsSvc.Handler())
		metricsSrv = &http.Server{
			Addr:         metricsSvc.Addr(),
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
		go func() {
			slog.Info("starting metrics server", "addr", metricsSrv.Addr, "path", metricsSvc.Path())
			// Logged rather than fatal: losing metrics must never take the API
			// down with it.
			if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("metrics server", "error", err)
			}
		}()
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

	if metricsSrv != nil {
		if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
			slog.Warn("metrics server shutdown", "error", err)
		}
	}

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

// migrationAdvisoryLockKey serializes the whole migration run across processes.
// Rolling restarts overlap pods, and two of them applying the same file at once
// either collides on duplicate DDL or on the schema_migrations primary key.
const migrationAdvisoryLockKey = "nib_schema_migrations"

// runMigrations applies .up.sql files in lexicographic order, tracking applied
// migrations in a schema_migrations table so each file runs at most once.
//
// The whole run holds one advisory lock on a single dedicated connection, and
// each file's DDL and its ledger row are written in one transaction, so a crash
// can never leave the schema ahead of the ledger (which would re-run the file
// and fail on the migrations that use bare CREATE TABLE / ADD COLUMN).
func runMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
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

	// One connection for the whole run: an advisory lock belongs to the session
	// that took it, so it cannot be taken through the pool.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtext($1))`, migrationAdvisoryLockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		// Best effort: the lock is released anyway when the session ends.
		if _, err := conn.Exec(context.WithoutCancel(ctx),
			`SELECT pg_advisory_unlock(hashtext($1))`, migrationAdvisoryLockKey); err != nil {
			slog.Warn("release migration lock failed", "error", err)
		}
	}()

	if err := ensureMigrationLedger(ctx, conn); err != nil {
		return err
	}

	for _, name := range upFiles {
		version := strings.TrimSuffix(name, ".up.sql")

		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		sum := fmt.Sprintf("%x", sha256.Sum256(body))

		var applied bool
		var recorded *string
		err = conn.QueryRow(ctx,
			`SELECT true, checksum FROM schema_migrations WHERE version = $1`, version).Scan(&applied, &recorded)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if applied {
			if err := reconcileMigrationChecksum(ctx, conn, version, sum, recorded); err != nil {
				return err
			}
			continue
		}

		if err := applyMigration(ctx, conn, version, string(body), sum); err != nil {
			return err
		}
		slog.Info("applied migration", "version", version)
	}

	return nil
}

// ensureMigrationLedger creates schema_migrations and brings an older ledger
// (one without the checksum column) up to the current shape.
func ensureMigrationLedger(ctx context.Context, conn *pgxpool.Conn) error {
	_, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}
	if _, err := conn.Exec(ctx,
		`ALTER TABLE schema_migrations ADD COLUMN IF NOT EXISTS checksum TEXT`); err != nil {
		return fmt.Errorf("add schema_migrations.checksum: %w", err)
	}
	return nil
}

// reconcileMigrationChecksum backfills the checksum of a migration that was
// applied before the column existed, and warns when a recorded file has since
// been edited — an edit is silently never re-applied, so it has to be visible.
func reconcileMigrationChecksum(ctx context.Context, conn *pgxpool.Conn, version, sum string, recorded *string) error {
	if recorded == nil {
		if _, err := conn.Exec(ctx,
			`UPDATE schema_migrations SET checksum = $2 WHERE version = $1 AND checksum IS NULL`,
			version, sum); err != nil {
			return fmt.Errorf("backfill checksum for %s: %w", version, err)
		}
		return nil
	}
	if *recorded != sum {
		slog.Warn("migration file changed after it was applied; it will not re-run",
			"version", version, "recorded_checksum", *recorded, "file_checksum", sum)
	}
	return nil
}

// applyMigration runs one migration file and records it in the same transaction,
// so the ledger and the schema can never disagree.
func applyMigration(ctx context.Context, conn *pgxpool.Conn, version, body, sum string) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", version, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, body); err != nil {
		return fmt.Errorf("apply migration %s: %w", version, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)`, version, sum); err != nil {
		return fmt.Errorf("record migration %s: %w", version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %s: %w", version, err)
	}
	return nil
}
