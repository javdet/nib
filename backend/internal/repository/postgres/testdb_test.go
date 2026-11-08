package postgres

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool connects to the Postgres named by NIB_POSTGRES_TEST_DSN, resets its
// public schema and applies every migration, so each test starts from the
// schema the backend would run on. It skips when the variable is unset: these
// tests need a real server (pgvector/pgvector:pg18), and the unit suite must
// not.
//
// The schema is shared, so tests using it do not run in parallel with each
// other.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("NIB_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("NIB_POSTGRES_TEST_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "migrations", "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("find migrations: %v (%d files)", err, len(files))
	}
	sort.Strings(files)
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(f), err)
		}
	}
	return pool
}

// testDialog inserts a root dialog and returns its id.
func testDialog(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO chat_dialogs (mode, title) VALUES ('main', 'test') RETURNING id`,
	).Scan(&id); err != nil {
		t.Fatalf("insert dialog: %v", err)
	}
	return id
}
