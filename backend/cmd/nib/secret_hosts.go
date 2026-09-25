package main

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/javdet/nib/internal/mcpconfig"
)

// secretHostsMigrationVersion is recorded in schema_migrations so the binding
// below runs at most once, like migratePlanOwnership.
const secretHostsMigrationVersion = "000039_secret_allowed_hosts_backfill"

// bindExistingSecretHosts binds every secret the current mcp.json references to
// the hosts of the servers referencing it, so an install upgrading into
// per-secret allowed hosts keeps working. It runs once: after that the only way
// to add a host is to enter the secret's value again, and an mcp.json edited
// later gains nothing from this.
//
// Only the global scope is bound, since that is the only one ${NAME} reads.
func bindExistingSecretHosts(ctx context.Context, pool *pgxpool.Pool, mcpCfg *mcpconfig.Service) error {
	var applied bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`,
		secretHostsMigrationVersion,
	).Scan(&applied); err != nil {
		return fmt.Errorf("check secret hosts migration: %w", err)
	}
	if applied {
		return nil
	}

	// An mcp.json that does not parse binds nothing but still records the
	// migration: its servers were not working anyway, and retrying on a later
	// boot would bind whatever the file says by then.
	var refs map[string][]string
	servers, err := mcpCfg.ListServers()
	if err != nil {
		slog.Warn("secret hosts: mcp.json unreadable, no secret bound; set allowed hosts under Variables → Secrets",
			"error", err)
	} else {
		refs = mcpconfig.SecretHostRefs(servers)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin secret hosts migration: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	names := make([]string, 0, len(refs))
	for name := range refs {
		names = append(names, name)
	}
	sort.Strings(names)

	bound := 0
	for _, name := range names {
		tag, err := tx.Exec(ctx,
			`UPDATE prompt_secrets
			 SET allowed_hosts = ARRAY(SELECT DISTINCT h FROM unnest(allowed_hosts || $2::text[]) AS h ORDER BY h)
			 WHERE scope = 'global' AND scope_name = '' AND name = $1`,
			name, refs[name])
		if err != nil {
			return fmt.Errorf("bind secret %q hosts: %w", name, err)
		}
		if tag.RowsAffected() > 0 {
			bound++
			slog.Info("secret hosts: bound existing secret", "secret", name, "hosts", refs[name])
		}
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT (version) DO NOTHING`,
		secretHostsMigrationVersion,
	); err != nil {
		return fmt.Errorf("record secret hosts migration: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit secret hosts migration: %w", err)
	}

	slog.Info("secret hosts migration applied", "secrets_bound", bound, "secrets_referenced", len(refs))
	return nil
}
