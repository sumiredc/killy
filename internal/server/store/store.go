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
// It returns the schema version the database has afterwards.
func Migrate(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return 0, err
	}
	return migrate(ctx, pool, sub)
}

func migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) (int64, error) {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, fsys)
	if err != nil {
		return 0, fmt.Errorf("load migrations: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return 0, fmt.Errorf("migrate: %w", err)
	}
	v, err := p.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return v, nil
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

type Device struct {
	ID string
}

// Link is one phone's view of one agent.
type Link struct {
	AgentID string
	Name    string
	Notify  bool
}

// CreateDevice stores a new device under a fresh UUID v4. pushToken may be empty.
func (s *Store) CreateDevice(ctx context.Context, tokenSHA256 []byte, pushToken string) (Device, error) {
	d := Device{ID: uuid.NewV4().String()}
	var push *string
	if pushToken != "" {
		push = &pushToken
	}
	_, err := s.pool.Exec(ctx, "INSERT INTO devices (id, token_sha256, push_token) VALUES ($1, $2, $3)", d.ID, tokenSHA256, push)
	if err != nil {
		return Device{}, fmt.Errorf("create device: %w", err)
	}
	return d, nil
}

// DeviceByToken returns the device whose token has the SHA-256 tokenSHA256, or ErrNotFound.
func (s *Store) DeviceByToken(ctx context.Context, tokenSHA256 []byte) (Device, error) {
	var d Device
	err := s.pool.QueryRow(ctx, "SELECT id::text FROM devices WHERE token_sha256 = $1", tokenSHA256).Scan(&d.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, fmt.Errorf("find device: %w", err)
	}
	return d, nil
}

// AgentByID returns the agent, or ErrNotFound.
func (s *Store) AgentByID(ctx context.Context, id string) (Agent, error) {
	var a Agent
	err := s.pool.QueryRow(ctx, "SELECT id::text, hostname FROM agents WHERE id = $1", id).Scan(&a.ID, &a.Hostname)
	if errors.Is(err, pgx.ErrNoRows) {
		return Agent{}, ErrNotFound
	}
	if err != nil {
		return Agent{}, fmt.Errorf("find agent: %w", err)
	}
	return a, nil
}

// LinkDevice links the device to the agent. A new link is named name, or fallback when name is
// nil; an existing link keeps its name unless name is given. The notification switch of a new
// link is on.
func (s *Store) LinkDevice(ctx context.Context, deviceID, agentID string, name *string, fallback string) (Link, error) {
	l := Link{AgentID: agentID}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO links (device_id, agent_id, name) VALUES ($1, $2, COALESCE($3, $4))
		ON CONFLICT (device_id, agent_id) DO UPDATE SET name = COALESCE($3, links.name)
		RETURNING name, notify`,
		deviceID, agentID, name, fallback).Scan(&l.Name, &l.Notify)
	if err != nil {
		return Link{}, fmt.Errorf("link device: %w", err)
	}
	return l, nil
}

// UpdateLink changes this device's link only; a nil name or notify leaves that field as it is. It
// returns ErrNotFound when the device is not linked to the agent.
func (s *Store) UpdateLink(ctx context.Context, deviceID, agentID string, name *string, notify *bool) (Link, error) {
	l := Link{AgentID: agentID}
	err := s.pool.QueryRow(ctx, `
		UPDATE links SET name = COALESCE($3, name), notify = COALESCE($4, notify)
		WHERE device_id = $1 AND agent_id = $2
		RETURNING name, notify`,
		deviceID, agentID, name, notify).Scan(&l.Name, &l.Notify)
	if errors.Is(err, pgx.ErrNoRows) {
		return Link{}, ErrNotFound
	}
	if err != nil {
		return Link{}, fmt.Errorf("update link: %w", err)
	}
	return l, nil
}

// UnlinkDevice removes this device's link only, or returns ErrNotFound.
func (s *Store) UnlinkDevice(ctx context.Context, deviceID, agentID string) error {
	tag, err := s.pool.Exec(ctx, "DELETE FROM links WHERE device_id = $1 AND agent_id = $2", deviceID, agentID)
	if err != nil {
		return fmt.Errorf("unlink device: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// LinkedServer is an agent as one device sees it.
type LinkedServer struct {
	AgentID  string
	Name     string
	Notify   bool
	Hostname string
}

// LinksOfDevice returns the agents linked to the device, in the order they were linked.
func (s *Store) LinksOfDevice(ctx context.Context, deviceID string) ([]LinkedServer, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT l.agent_id::text, l.name, l.notify, a.hostname
		FROM links l JOIN agents a ON a.id = l.agent_id
		WHERE l.device_id = $1
		ORDER BY l.created_at, l.agent_id`, deviceID)
	if err != nil {
		return nil, fmt.Errorf("list links of device: %w", err)
	}
	defer rows.Close()
	servers := []LinkedServer{}
	for rows.Next() {
		var l LinkedServer
		if err := rows.Scan(&l.AgentID, &l.Name, &l.Notify, &l.Hostname); err != nil {
			return nil, err
		}
		servers = append(servers, l)
	}
	return servers, rows.Err()
}

// DevicesOfAgent returns the IDs of the devices linked to the agent.
func (s *Store) DevicesOfAgent(ctx context.Context, agentID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, "SELECT device_id::text FROM links WHERE agent_id = $1", agentID)
	if err != nil {
		return nil, fmt.Errorf("list devices of agent: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// UpdateDevice changes the device's push token or app-wide switch; a nil field is left as it is.
// A push token moves: any other device that had it loses it, so one phone gets one push. It
// returns ErrNotFound when there is no such device.
func (s *Store) UpdateDevice(ctx context.Context, deviceID string, pushToken *string, notify *bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("update device: %w", err)
	}
	defer tx.Rollback(ctx)
	if pushToken != nil {
		if _, err := tx.Exec(ctx, "UPDATE devices SET push_token = NULL WHERE push_token = $1 AND id <> $2", *pushToken, deviceID); err != nil {
			return fmt.Errorf("move push token: %w", err)
		}
	}
	tag, err := tx.Exec(ctx, "UPDATE devices SET push_token = COALESCE($2, push_token), notify = COALESCE($3, notify) WHERE id = $1",
		deviceID, pushToken, notify)
	if err != nil {
		return fmt.Errorf("update device: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("update device: %w", err)
	}
	return nil
}

// PushTarget is a device to notify of an agent's parents, with the agent's name on that device.
type PushTarget struct {
	DeviceID  string
	PushToken string
	Name      string
}

// Notified is one parent a device was already notified of. FirstDetectedAt is the parent's
// first-detection time, so a record meant for one episode of a parent cannot land on the next.
type Notified struct {
	Parent          ParentKey
	FirstDetectedAt time.Time
	DeviceID        string
}

// PushTargets returns the devices linked to the agent that have a push token and both the
// app-wide and the server's switch on.
func (s *Store) PushTargets(ctx context.Context, agentID string) ([]PushTarget, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id::text, d.push_token, l.name
		FROM links l JOIN devices d ON d.id = l.device_id
		WHERE l.agent_id = $1 AND l.notify AND d.notify AND d.push_token IS NOT NULL
		ORDER BY l.created_at, d.id`, agentID)
	if err != nil {
		return nil, fmt.Errorf("list push targets: %w", err)
	}
	defer rows.Close()
	var targets []PushTarget
	for rows.Next() {
		var t PushTarget
		if err := rows.Scan(&t.DeviceID, &t.PushToken, &t.Name); err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// NotifyState is what the notifier needs from the store about one agent, read in one statement
// so both parts are from the same moment: the agent's parents with their first-detection times,
// and the devices already notified of each.
type NotifyState struct {
	FirstDetected map[ParentKey]time.Time
	Notified      []Notified
}

// NotifyState returns the agent's first-detection rows and its notified rows.
func (s *Store) NotifyState(ctx context.Context, agentID string) (NotifyState, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT f.pid, f.starttime, f.detected_at, n.device_id::text
		FROM first_detections f
		LEFT JOIN notified_parents n ON n.agent_id = f.agent_id AND n.pid = f.pid AND n.starttime = f.starttime
		WHERE f.agent_id = $1`, agentID)
	if err != nil {
		return NotifyState{}, fmt.Errorf("read notify state: %w", err)
	}
	defer rows.Close()
	st := NotifyState{FirstDetected: map[ParentKey]time.Time{}}
	for rows.Next() {
		var k ParentKey
		var at time.Time
		var device *string
		if err := rows.Scan(&k.PID, &k.StartTime, &at, &device); err != nil {
			return NotifyState{}, err
		}
		st.FirstDetected[k] = at
		if device != nil {
			st.Notified = append(st.Notified, Notified{Parent: k, FirstDetectedAt: at, DeviceID: *device})
		}
	}
	return st, rows.Err()
}

// RecordNotified records the pairs as notified. A pair whose parent left the snapshot (or left
// and came back, with a new first-detection time), or whose device was unlinked, meanwhile is
// dropped; a pair already recorded is left as it is.
func (s *Store) RecordNotified(ctx context.Context, agentID string, pairs []Notified) error {
	if len(pairs) == 0 {
		return nil
	}
	pids := make([]int32, len(pairs))
	starts := make([]int64, len(pairs))
	detected := make([]time.Time, len(pairs))
	devices := make([]string, len(pairs))
	for i, p := range pairs {
		pids[i], starts[i], detected[i], devices[i] = int32(p.Parent.PID), p.Parent.StartTime, p.FirstDetectedAt, p.DeviceID
	}
	// The joins drop the pairs whose rows are gone instead of failing on the foreign keys.
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO notified_parents (agent_id, pid, starttime, device_id)
		SELECT f.agent_id, f.pid, f.starttime, l.device_id
		FROM unnest($2::integer[], $3::bigint[], $4::timestamptz[], $5::uuid[]) AS p(pid, starttime, detected_at, device_id)
		JOIN first_detections f ON f.agent_id = $1 AND f.pid = p.pid AND f.starttime = p.starttime AND f.detected_at = p.detected_at
		JOIN links l ON l.agent_id = $1 AND l.device_id = p.device_id
		ON CONFLICT DO NOTHING`,
		agentID, pids, starts, detected, devices); err != nil {
		return fmt.Errorf("record notified parents: %w", err)
	}
	return nil
}

// ClearPushToken removes the device's push token if it is still pushToken, so a newer one the app
// registered meanwhile is kept.
func (s *Store) ClearPushToken(ctx context.Context, deviceID, pushToken string) error {
	if _, err := s.pool.Exec(ctx,
		"UPDATE devices SET push_token = NULL WHERE id = $1 AND push_token = $2", deviceID, pushToken); err != nil {
		return fmt.Errorf("clear push token: %w", err)
	}
	return nil
}
