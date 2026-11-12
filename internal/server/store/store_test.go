package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sumiredc/killy/internal/protocol"
	"github.com/sumiredc/killy/internal/server/dbtest"
)

func tableExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(context.Background(), "SELECT to_regclass($1) IS NOT NULL", name).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

func appliedVersions(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM goose_db_version WHERE version_id > 0").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func migrated(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	pool := dbtest.Pool(t)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return New(pool), pool
}

func TestMigrateCreatesTheAgentsTable(t *testing.T) {
	_, pool := migrated(t)
	if !tableExists(t, pool, "agents") {
		t.Error("table agents does not exist after Migrate")
	}
}

func TestMigrateAgainChangesNothing(t *testing.T) {
	_, pool := migrated(t)
	before := appliedVersions(t, pool)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if after := appliedVersions(t, pool); after != before {
		t.Errorf("applied migrations went from %d to %d, want no change", before, after)
	}
}

func TestAFailingMigrationIsRolledBackAndStopsTheRest(t *testing.T) {
	pool := dbtest.Pool(t)
	fsys := fstest.MapFS{
		"00001_first.sql":  {Data: []byte("-- +goose Up\nCREATE TABLE first (x int);\n")},
		"00002_broken.sql": {Data: []byte("-- +goose Up\nCREATE TABLE half (x int);\nSELECT * FROM no_such_table;\n")},
		"00003_after.sql":  {Data: []byte("-- +goose Up\nCREATE TABLE after (x int);\n")},
	}
	if err := migrate(context.Background(), pool, fsys); err == nil {
		t.Fatal("migrate succeeded although migration 2 fails")
	}
	if !tableExists(t, pool, "first") {
		t.Error("migration 1 was not kept")
	}
	if tableExists(t, pool, "half") {
		t.Error("table half exists: the failed migration 2 was not rolled back")
	}
	if tableExists(t, pool, "after") {
		t.Error("table after exists: migration 3 ran after migration 2 failed")
	}
	if n := appliedVersions(t, pool); n != 1 {
		t.Errorf("%d migrations recorded as applied, want 1", n)
	}
}

func TestCreateAgentStoresTheTokenHashAndHostnameOnly(t *testing.T) {
	s, pool := migrated(t)
	hash := sha256.Sum256([]byte("the token"))
	a, err := s.CreateAgent(context.Background(), hash[:], "web-1")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if !protocol.IsUUIDv4(a.ID) || a.Hostname != "web-1" {
		t.Errorf("agent = %+v, want a UUID v4 id and hostname web-1", a)
	}
	var stored []byte
	var hostname string
	if err := pool.QueryRow(context.Background(), "SELECT token_sha256, hostname FROM agents WHERE id = $1", a.ID).Scan(&stored, &hostname); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, hash[:]) || hostname != "web-1" {
		t.Errorf("stored %x, %q; want %x, web-1", stored, hostname, hash)
	}
}

func TestAgentByTokenFindsTheAgentWithThatTokenHash(t *testing.T) {
	s, _ := migrated(t)
	ctx := context.Background()
	h1, h2 := sha256.Sum256([]byte("one")), sha256.Sum256([]byte("two"))
	a1, err := s.CreateAgent(ctx, h1[:], "one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAgent(ctx, h2[:], "two"); err != nil {
		t.Fatal(err)
	}
	got, err := s.AgentByToken(ctx, h1[:])
	if err != nil || got != a1 {
		t.Errorf("AgentByToken = %+v, %v; want %+v", got, err, a1)
	}
}

func TestAgentByTokenOfAnUnknownTokenIsNotFound(t *testing.T) {
	s, _ := migrated(t)
	h := sha256.Sum256([]byte("nobody"))
	if _, err := s.AgentByToken(context.Background(), h[:]); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
