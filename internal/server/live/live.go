// Package live keeps the latest snapshot of every agent in memory.
package live

import (
	"context"
	"sync"
	"time"

	"github.com/sumiredc/killy/internal/clock"
	"github.com/sumiredc/killy/internal/protocol"
	"github.com/sumiredc/killy/internal/server/store"
)

// DisconnectedAfter is how long after its last snapshot an agent counts as disconnected.
const DisconnectedAfter = 60 * time.Second

// Detections keeps first-detection times; *store.Store (PostgreSQL) in production.
type Detections interface {
	SyncFirstDetections(ctx context.Context, agentID string, parents []store.ParentKey, now time.Time) (map[store.ParentKey]time.Time, error)
}

// Parent is a reported parent with the time the Killy server first saw it.
type Parent struct {
	protocol.Parent
	FirstDetectedAt time.Time
}

// Agent is what the Killy server knows about one agent right now.
type Agent struct {
	Hostname  string
	Version   string
	ScannedAt time.Time
	// ReceivedAt is when the Killy server accepted the snapshot, by its own clock.
	ReceivedAt time.Time
	Parents    []Parent
}

// State is safe for concurrent use. A restart empties it; agents fill it again with their next snapshot.
type State struct {
	clock      clock.Clock
	detections Detections

	mu     sync.Mutex
	agents map[string]*entry
}

type entry struct {
	// reporting is held for a whole Report, database call included, so two snapshots of one
	// agent are applied in turn and the scan-time check sees the one applied just before.
	reporting sync.Mutex
	// mu guards agent only and is never held across the database, so readers do not wait for it.
	mu    sync.Mutex
	agent *Agent
}

func (e *entry) current() *Agent {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.agent
}

func New(c clock.Clock, d Detections) *State {
	return &State{clock: c, detections: d, agents: map[string]*entry{}}
}

// Report replaces the agent's snapshot. A snapshot scanned before the last accepted one is
// discarded without error, so a late report cannot bring back a parent that is gone.
func (s *State) Report(ctx context.Context, agentID string, snap protocol.Snapshot) error {
	e := s.entry(agentID)
	e.reporting.Lock()
	defer e.reporting.Unlock()
	if prev := e.current(); prev != nil && snap.ScannedAt.Before(prev.ScannedAt) {
		return nil
	}
	keys := make([]store.ParentKey, len(snap.Parents))
	for i, p := range snap.Parents {
		keys[i] = store.ParentKey{PID: p.PID, StartTime: p.StartTime}
	}
	now := s.clock.Now()
	detected, err := s.detections.SyncFirstDetections(ctx, agentID, keys, now)
	if err != nil {
		return err
	}
	parents := make([]Parent, len(snap.Parents))
	for i, p := range snap.Parents {
		parents[i] = Parent{Parent: p, FirstDetectedAt: detected[keys[i]]}
	}
	a := &Agent{Hostname: snap.Hostname, Version: snap.Version, ScannedAt: snap.ScannedAt, ReceivedAt: now, Parents: parents}
	e.mu.Lock()
	e.agent = a
	e.mu.Unlock()
	return nil
}

func (s *State) entry(agentID string) *entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.agents[agentID]
	if !ok {
		e = &entry{}
		s.agents[agentID] = e
	}
	return e
}

// Agent returns the agent's latest snapshot, if it has sent one since the Killy server started.
func (s *State) Agent(agentID string) (Agent, bool) {
	s.mu.Lock()
	e, ok := s.agents[agentID]
	s.mu.Unlock()
	if !ok {
		return Agent{}, false
	}
	a := e.current()
	if a == nil {
		return Agent{}, false
	}
	return *a, true
}

// Connected reports whether the agent sent a snapshot less than DisconnectedAfter ago; from
// exactly DisconnectedAfter on it is disconnected. An agent that has not reported since the Killy
// server started is disconnected.
func (s *State) Connected(agentID string) bool {
	a, ok := s.Agent(agentID)
	return ok && s.clock.Now().Before(a.ReceivedAt.Add(DisconnectedAfter))
}
