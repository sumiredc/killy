// Package dbtest gives each test its own empty PostgreSQL schema.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EnvURL names the variable holding the URL of the PostgreSQL the tests may use.
const EnvURL = "KILLY_TEST_DATABASE_URL"

// Pool returns a pool whose connections use a new schema, dropped when the test ends. Without
// KILLY_TEST_DATABASE_URL the test is skipped, except in CI (CI is set), where it fails, so a
// missing database cannot make CI pass by skipping.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv(EnvURL)
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatalf("%s is not set; CI must run the database tests", EnvURL)
		}
		t.Skipf("%s is not set; skipping a test that needs PostgreSQL (see the Makefile)", EnvURL)
	}
	ctx := context.Background()

	b := make([]byte, 8)
	_, _ = rand.Read(b)
	schema := "test_" + hex.EncodeToString(b)
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect to %s: %v", EnvURL, err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close(context.Background())
	})

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse %s: %v", EnvURL, err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
