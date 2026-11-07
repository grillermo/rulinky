package store

import (
	"context"
	_ "embed"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

const defaultTestDatabaseURL = "postgres://grillermo@192.168.1.1/rulinky_test"

// OpenTest returns a Store on a fresh, empty schema inside the test database
// (TEST_DATABASE_URL, whose name must end in _test). The schema is dropped
// when the test ends, so tests and packages can run in parallel.
func OpenTest(t testing.TB) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = defaultTestDatabaseURL
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatalf("refusing to use database %q: its name must end in _test", cfg.ConnConfig.Database)
	}
	ctx := context.Background()
	schema := "t_" + strings.ReplaceAll(newUUID(), "-", "")

	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close(ctx)
	})
	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		t.Fatal(err)
	}
	return New(pool)
}
