// Package memstore is an in-memory stand-in for the PostgreSQL store, for unit tests. It keeps
// the same rules as the SQL in package store: tokens are looked up by their SHA-256, first
// detections are synced to the reported set of parents.
package memstore

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"time"
	"uuid"

	"github.com/sumiredc/killy/internal/server/store"
)

// Memory is safe for concurrent use.
type Memory struct {
	mu sync.Mutex
	// Err, when set, is returned by every call, as when the database cannot be reached.
	Err        error
	agents     []AgentRow
	detections map[string]map[store.ParentKey]time.Time
}

// AgentRow is what the agents table would hold.
type AgentRow struct {
	ID          string
	TokenSHA256 []byte
	Hostname    string
}

func New() *Memory {
	return &Memory{detections: map[string]map[store.ParentKey]time.Time{}}
}

func (m *Memory) CreateAgent(_ context.Context, tokenSHA256 []byte, hostname string) (store.Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return store.Agent{}, m.Err
	}
	for _, a := range m.agents {
		if bytes.Equal(a.TokenSHA256, tokenSHA256) {
			return store.Agent{}, errors.New("memstore: duplicate token_sha256 (UNIQUE in the agents table)")
		}
	}
	row := AgentRow{ID: uuid.NewV4().String(), TokenSHA256: bytes.Clone(tokenSHA256), Hostname: hostname}
	m.agents = append(m.agents, row)
	return store.Agent{ID: row.ID, Hostname: row.Hostname}, nil
}

func (m *Memory) AgentByToken(_ context.Context, tokenSHA256 []byte) (store.Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return store.Agent{}, m.Err
	}
	for _, a := range m.agents {
		if bytes.Equal(a.TokenSHA256, tokenSHA256) {
			return store.Agent{ID: a.ID, Hostname: a.Hostname}, nil
		}
	}
	return store.Agent{}, store.ErrNotFound
}

// Agents returns a copy of the stored agent rows.
func (m *Memory) Agents() []AgentRow {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.agents)
}

func (m *Memory) SyncFirstDetections(_ context.Context, agentID string, parents []store.ParentKey, now time.Time) (map[store.ParentKey]time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	// timestamptz keeps microseconds; returning finer times would let a test pass that fails on PostgreSQL.
	now = now.Truncate(time.Microsecond)
	old := m.detections[agentID]
	next := make(map[store.ParentKey]time.Time, len(parents))
	for _, p := range parents {
		if at, ok := old[p]; ok {
			next[p] = at
		} else if _, dup := next[p]; !dup {
			next[p] = now
		}
	}
	m.detections[agentID] = next
	got := make(map[store.ParentKey]time.Time, len(next))
	for k, v := range next {
		got[k] = v
	}
	return got, nil
}

// Detections returns a copy of the agent's first-detection rows.
func (m *Memory) Detections(agentID string) map[store.ParentKey]time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	got := map[store.ParentKey]time.Time{}
	for k, v := range m.detections[agentID] {
		got[k] = v
	}
	return got
}
