package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationsTestDSN points at a throwaway Postgres. Override with
// NIB_MIGRATIONS_TEST_DSN; the test skips when nothing is reachable.
func migrationsTestDSN() string {
	if d := strings.TrimSpace(os.Getenv("NIB_MIGRATIONS_TEST_DSN")); d != "" {
		return d
	}
	return "postgres://nib:nib@127.0.0.1:5432/nib?sslmode=disable"
}

// testPool connects and hands back an empty schema, so each test starts from
// nothing regardless of what ran before it.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, migrationsTestDSN())
	if err != nil {
		t.Skipf("postgres not reachable (%v); set NIB_MIGRATIONS_TEST_DSN", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("postgres not reachable (%v); set NIB_MIGRATIONS_TEST_DSN", err)
	}
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		pool.Close()
		t.Fatalf("reset schema: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// writeMigration drops one .up.sql into dir.
func writeMigration(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestRunMigrationsRecordsChecksum(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	dir := t.TempDir()

	body := "CREATE TABLE widgets (id int);"
	writeMigration(t, dir, "000001_widgets.up.sql", body)

	if err := runMigrations(ctx, pool, dir); err != nil {
		t.Fatalf("runMigrations() error = %v", err)
	}

	var got string
	if err := pool.QueryRow(ctx,
		`SELECT checksum FROM schema_migrations WHERE version = '000001_widgets'`).Scan(&got); err != nil {
		t.Fatalf("read checksum: %v", err)
	}
	if want := fmt.Sprintf("%x", sha256.Sum256([]byte(body))); got != want {
		t.Errorf("checksum = %q, want %q", got, want)
	}
}

// A migration that fails must leave no ledger row behind. Before apply and
// record shared a transaction, a crash between them left the DDL committed and
// unrecorded, and the next boot re-ran a bare CREATE TABLE and never started.
func TestRunMigrationsDoesNotRecordAFailedMigration(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeMigration(t, dir, "000001_ok.up.sql", "CREATE TABLE widgets (id int);")
	writeMigration(t, dir, "000002_broken.up.sql",
		"CREATE TABLE gadgets (id int); SELECT this_function_does_not_exist();")

	if err := runMigrations(ctx, pool, dir); err == nil {
		t.Fatal("runMigrations() error = nil, want the broken migration to fail")
	}

	var recorded int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM schema_migrations WHERE version = '000002_broken'`).Scan(&recorded); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if recorded != 0 {
		t.Errorf("ledger rows for the failed migration = %d, want 0", recorded)
	}

	// The DDL it did get through must have rolled back with it, so a retry of
	// the same file is not a duplicate-object error.
	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT to_regclass('public.gadgets') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatalf("check gadgets: %v", err)
	}
	if exists {
		t.Error("gadgets table survived a failed migration; apply and record are not atomic")
	}
}

// An already-applied file is never re-run, and a ledger row written before the
// checksum column existed is backfilled rather than treated as a mismatch.
func TestRunMigrationsBackfillsLegacyChecksum(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	dir := t.TempDir()

	body := "CREATE TABLE widgets (id int);"
	writeMigration(t, dir, "000001_widgets.up.sql", body)

	if _, err := pool.Exec(ctx, `
		CREATE TABLE schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		INSERT INTO schema_migrations (version) VALUES ('000001_widgets')`); err != nil {
		t.Fatalf("seed legacy ledger: %v", err)
	}

	if err := runMigrations(ctx, pool, dir); err != nil {
		t.Fatalf("runMigrations() error = %v", err)
	}

	var checksum *string
	if err := pool.QueryRow(ctx,
		`SELECT checksum FROM schema_migrations WHERE version = '000001_widgets'`).Scan(&checksum); err != nil {
		t.Fatalf("read checksum: %v", err)
	}
	if checksum == nil {
		t.Fatal("checksum was not backfilled")
	}
	if want := fmt.Sprintf("%x", sha256.Sum256([]byte(body))); *checksum != want {
		t.Errorf("checksum = %q, want %q", *checksum, want)
	}

	// The recorded file must not have been applied: the table it creates would
	// already exist, and a bare CREATE TABLE would have errored.
	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT to_regclass('public.widgets') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatalf("check widgets: %v", err)
	}
	if exists {
		t.Error("an already-recorded migration was applied again")
	}
}

// The advisory lock is what serializes overlapping pods during a rolling
// restart. Two concurrent runs over the same directory must both come back
// clean, with the file applied exactly once.
func TestRunMigrationsIsSafeConcurrently(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeMigration(t, dir, "000001_widgets.up.sql", "CREATE TABLE widgets (id int);")

	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- runMigrations(ctx, pool, dir) }()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Errorf("concurrent runMigrations() error = %v", err)
		}
	}

	var applied int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM schema_migrations WHERE version = '000001_widgets'`).Scan(&applied); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if applied != 1 {
		t.Errorf("ledger rows = %d, want 1", applied)
	}
}

// The real migrations directory has to apply through the real runner, which is
// stricter than applying the files by hand: the runner wraps each file in one
// transaction, so anything illegal inside a transaction block (CREATE INDEX
// CONCURRENTLY, say) fails here and nowhere else.
func TestRunMigrationsAppliesTheRealDirectory(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// The pgvector extension is created at initdb time in every deployment, so
	// the migrations are entitled to assume it, and DROP SCHEMA removed it.
	if _, err := pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		t.Fatalf("create vector extension: %v", err)
	}

	dir := filepath.Join("..", "..", "migrations")
	if err := runMigrations(ctx, pool, dir); err != nil {
		t.Fatalf("runMigrations() error = %v", err)
	}

	// Applying twice must be a no-op rather than a duplicate-object error.
	if err := runMigrations(ctx, pool, dir); err != nil {
		t.Fatalf("second runMigrations() error = %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	want := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			want++
		}
	}

	var applied int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatalf("count applied migrations: %v", err)
	}
	if applied != want {
		t.Errorf("applied migrations = %d, want %d", applied, want)
	}
}
