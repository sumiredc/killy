// Package store is the only code that reads and writes the Killy server's PostgreSQL database.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// ErrNotFound means no row matched.
var ErrNotFound = errors.New("not found")

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Migrate applies the migrations embedded in the binary that the database does not have yet.
// Each migration runs in its own transaction, so one that fails leaves no trace and stops here.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	return migrate(ctx, pool, sub)
}

func migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, fsys)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

type Agent struct {
	ID       string
	Hostname string
}

// CreateAgent stores a new agent under a fresh UUID v4. tokenSHA256 is the SHA-256 of its token;
// the token itself is never stored.
func (s *Store) CreateAgent(ctx context.Context, tokenSHA256 []byte, hostname string) (Agent, error) {
	a := Agent{ID: uuid.NewV4().String(), Hostname: hostname}
	_, err := s.pool.Exec(ctx, "INSERT INTO agents (id, token_sha256, hostname) VALUES ($1, $2, $3)", a.ID, tokenSHA256, hostname)
	if err != nil {
		return Agent{}, fmt.Errorf("create agent: %w", err)
	}
	return a, nil
}

// AgentByToken returns the agent whose token has the SHA-256 tokenSHA256, or ErrNotFound.
func (s *Store) AgentByToken(ctx context.Context, tokenSHA256 []byte) (Agent, error) {
	var a Agent
	err := s.pool.QueryRow(ctx, "SELECT id::text, hostname FROM agents WHERE token_sha256 = $1", tokenSHA256).Scan(&a.ID, &a.Hostname)
	if errors.Is(err, pgx.ErrNoRows) {
		return Agent{}, ErrNotFound
	}
	if err != nil {
		return Agent{}, fmt.Errorf("find agent: %w", err)
	}
	return a, nil
}
