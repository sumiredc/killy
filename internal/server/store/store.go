// Package store is the only code that reads and writes the Killy server's PostgreSQL database.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"time"
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

// ParentKey identifies a parent on one agent: a reused PID has another start time.
type ParentKey struct {
	PID       int
	StartTime int64
}

// SyncFirstDetections makes the agent's first-detection rows match parents: rows for parents not
// in the list are deleted, parents without a row get now, and existing rows keep their time. It
// returns the first-detection time of every parent in the list.
func (s *Store) SyncFirstDetections(ctx context.Context, agentID string, parents []ParentKey, now time.Time) (map[ParentKey]time.Time, error) {
	pids := make([]int32, len(parents))
	starts := make([]int64, len(parents))
	for i, p := range parents {
		pids[i], starts[i] = int32(p.PID), p.StartTime
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("sync first detections: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		DELETE FROM first_detections d
		WHERE d.agent_id = $1
		  AND (d.pid, d.starttime) NOT IN (SELECT * FROM unnest($2::integer[], $3::bigint[]))`,
		agentID, pids, starts); err != nil {
		return nil, fmt.Errorf("delete gone parents: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO first_detections (agent_id, pid, starttime, detected_at)
		SELECT $1, p.pid, p.starttime, $4 FROM unnest($2::integer[], $3::bigint[]) AS p(pid, starttime)
		ON CONFLICT DO NOTHING`,
		agentID, pids, starts, now); err != nil {
		return nil, fmt.Errorf("insert new parents: %w", err)
	}
	rows, err := tx.Query(ctx, "SELECT pid, starttime, detected_at FROM first_detections WHERE agent_id = $1", agentID)
	if err != nil {
		return nil, fmt.Errorf("read first detections: %w", err)
	}
	got := make(map[ParentKey]time.Time, len(parents))
	for rows.Next() {
		var k ParentKey
		var at time.Time
		if err := rows.Scan(&k.PID, &k.StartTime, &at); err != nil {
			return nil, err
		}
		got[k] = at
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("sync first detections: %w", err)
	}
	return got, nil
}
