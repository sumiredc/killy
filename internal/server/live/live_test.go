package live

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/clock"
	"github.com/sumiredc/killy/internal/protocol"
	"github.com/sumiredc/killy/internal/server/dbtest"
	"github.com/sumiredc/killy/internal/server/store"
)

var t0 = time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)

type fixture struct {
	store   *store.Store
	clock   *clock.Fake
	state   *State
	agentID string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := dbtest.Pool(t)
	if err := store.Migrate(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	s := store.New(pool)
	h := sha256.Sum256([]byte("token"))
	a, err := s.CreateAgent(context.Background(), h[:], "web-1")
	if err != nil {
		t.Fatal(err)
	}
	c := clock.NewFake(t0)
	return &fixture{store: s, clock: c, state: New(c, s), agentID: a.ID}
}

// restart is a new Killy server process on the same database.
func (f *fixture) restart() { f.state = New(f.clock, f.store) }

func parent(pid int, start int64, zombies ...int) protocol.Parent {
	return protocol.Parent{PID: pid, StartTime: start, Cmd: "cmd", Zombies: zombies}
}

func (f *fixture) report(t *testing.T, scannedAt time.Time, parents ...protocol.Parent) {
	t.Helper()
	if parents == nil {
		parents = []protocol.Parent{}
	}
	err := f.state.Report(context.Background(), f.agentID, protocol.Snapshot{Hostname: "web-1", Version: "dev", ScannedAt: scannedAt, Parents: parents})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
}

func (f *fixture) parents(t *testing.T) map[int]Parent {
	t.Helper()
	a, ok := f.state.Agent(f.agentID)
	if !ok {
		t.Fatal("no state for the agent")
	}
	m := map[int]Parent{}
	for _, p := range a.Parents {
		m[p.PID] = p
	}
	return m
}

func TestASnapshotReplacesThePreviousOne(t *testing.T) {
	f := newFixture(t)
	f.report(t, t0, parent(100, 5000, 101), parent(200, 7000, 201))
	f.report(t, t0.Add(5*time.Second), parent(200, 7000, 201, 202), parent(300, 9000, 301))
	got := f.parents(t)
	if _, ok := got[100]; ok || len(got) != 2 {
		t.Fatalf("parents %v, want 200 and 300 only", got)
	}
	if z := got[200].Zombies; len(z) != 2 {
		t.Errorf("parent 200 zombies %v, want [201 202]", z)
	}
}

func TestTheFirstDetectionTimeStaysForTheSameParent(t *testing.T) {
	f := newFixture(t)
	f.report(t, t0, parent(100, 5000, 101))
	f.clock.Advance(15 * time.Second)
	f.report(t, t0.Add(15*time.Second), parent(100, 5000, 101, 102))
	if got := f.parents(t)[100].FirstDetectedAt; !got.Equal(t0) {
		t.Errorf("first detected at %v, want %v", got, t0)
	}
}

func TestAnotherStartTimeIsANewParentWithANewDetectionTime(t *testing.T) {
	f := newFixture(t)
	f.report(t, t0, parent(100, 5000, 101))
	f.clock.Advance(15 * time.Second)
	f.report(t, t0.Add(15*time.Second), parent(100, 6000, 101))
	if got := f.parents(t)[100].FirstDetectedAt; !got.Equal(t0.Add(15 * time.Second)) {
		t.Errorf("first detected at %v, want %v", got, t0.Add(15*time.Second))
	}
}

func TestTheFirstDetectionTimeSurvivesARestart(t *testing.T) {
	f := newFixture(t)
	f.report(t, t0, parent(100, 5000, 101))
	f.restart()
	f.clock.Advance(time.Minute)
	f.report(t, t0.Add(time.Minute), parent(100, 5000, 101))
	if got := f.parents(t)[100].FirstDetectedAt; !got.Equal(t0) {
		t.Errorf("first detected at %v after a restart, want %v", got, t0)
	}
}

func TestAParentGoneDuringARestartIsForgottenWithTheFirstSnapshotAfter(t *testing.T) {
	f := newFixture(t)
	f.report(t, t0, parent(100, 5000, 101), parent(200, 7000, 201))
	f.restart()
	f.clock.Advance(time.Minute)
	f.report(t, t0.Add(time.Minute), parent(200, 7000, 201))
	f.clock.Advance(time.Minute)
	f.report(t, t0.Add(2*time.Minute), parent(100, 5000, 101), parent(200, 7000, 201))
	if got := f.parents(t)[100].FirstDetectedAt; !got.Equal(t0.Add(2 * time.Minute)) {
		t.Errorf("parent 100 first detected at %v, want %v (its old time must be gone)", got, t0.Add(2*time.Minute))
	}
}

func TestAnOlderSnapshotIsDiscarded(t *testing.T) {
	f := newFixture(t)
	f.report(t, t0.Add(10*time.Second), parent(200, 7000, 201))
	f.report(t, t0, parent(100, 5000, 101))
	got := f.parents(t)
	if _, ok := got[100]; ok {
		t.Errorf("parents %v: the older snapshot brought back parent 100", got)
	}
	if _, ok := got[200]; !ok {
		t.Errorf("parents %v: want parent 200 from the newer snapshot", got)
	}
}

func TestASnapshotWithTheSameScanTimeIsApplied(t *testing.T) {
	f := newFixture(t)
	f.report(t, t0, parent(100, 5000, 101))
	f.report(t, t0, parent(200, 7000, 201))
	if got := f.parents(t); len(got) != 1 || got[200].PID != 200 {
		t.Errorf("parents %v, want parent 200 only", got)
	}
}

func TestAnAgentThatNeverReportedIsDisconnected(t *testing.T) {
	f := newFixture(t)
	if f.state.Connected(f.agentID) {
		t.Error("Connected = true before any snapshot")
	}
}

func TestAnAgentIsConnectedUntilSixtySecondsAfterItsLastSnapshot(t *testing.T) {
	f := newFixture(t)
	f.report(t, t0)
	f.clock.Advance(DisconnectedAfter - time.Nanosecond)
	if !f.state.Connected(f.agentID) {
		t.Error("disconnected just before 60 s, want connected")
	}
}

func TestAnAgentIsDisconnectedAtExactlySixtySeconds(t *testing.T) {
	f := newFixture(t)
	f.report(t, t0)
	f.clock.Advance(DisconnectedAfter)
	if f.state.Connected(f.agentID) {
		t.Error("connected at exactly 60 s, want disconnected")
	}
	f.clock.Advance(time.Second)
	if f.state.Connected(f.agentID) {
		t.Error("connected at 61 s, want disconnected")
	}
}

func TestADiscardedSnapshotDoesNotKeepTheAgentConnected(t *testing.T) {
	f := newFixture(t)
	f.report(t, t0.Add(time.Hour))
	f.clock.Advance(DisconnectedAfter)
	f.report(t, t0)
	if f.state.Connected(f.agentID) {
		t.Error("connected after only a discarded snapshot, want disconnected")
	}
}

func TestADiscardedSnapshotLeavesTheDetectionTimesAlone(t *testing.T) {
	f := newFixture(t)
	f.report(t, t0.Add(10*time.Second), parent(200, 7000, 201))
	f.clock.Advance(time.Minute)
	f.report(t, t0, parent(100, 5000, 101))
	f.clock.Advance(time.Minute)
	f.report(t, t0.Add(20*time.Second), parent(200, 7000, 201))
	if got := f.parents(t)[200].FirstDetectedAt; !got.Equal(t0) {
		t.Errorf("parent 200 first detected at %v, want %v: the discarded snapshot must not touch the database", got, t0)
	}
}

// gatedDetections holds the first sync until release is closed or the second sync starts.
type gatedDetections struct {
	Detections
	mu      sync.Mutex
	calls   int
	second  chan struct{}
	release chan struct{}
}

func (g *gatedDetections) SyncFirstDetections(ctx context.Context, agentID string, parents []store.ParentKey, now time.Time) (map[store.ParentKey]time.Time, error) {
	g.mu.Lock()
	g.calls++
	n := g.calls
	g.mu.Unlock()
	if n == 1 {
		select {
		case <-g.second:
		case <-time.After(300 * time.Millisecond):
		}
	} else {
		close(g.second)
	}
	return g.Detections.SyncFirstDetections(ctx, agentID, parents, now)
}

func TestTwoSnapshotsOfOneAgentAreAppliedInTurn(t *testing.T) {
	f := newFixture(t)
	g := &gatedDetections{Detections: f.store, second: make(chan struct{})}
	f.state = New(f.clock, g)
	older := protocol.Snapshot{ScannedAt: t0, Parents: []protocol.Parent{parent(100, 5000, 101)}}
	newer := protocol.Snapshot{ScannedAt: t0.Add(5 * time.Second), Parents: []protocol.Parent{parent(200, 7000, 201)}}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = f.state.Report(context.Background(), f.agentID, older) }()
	for g.callCount() == 0 {
		time.Sleep(time.Millisecond)
	}
	// The older snapshot is now inside its database call. Without one-at-a-time handling the
	// newer one would be applied first and then overwritten by the older one.
	if err := f.state.Report(context.Background(), f.agentID, newer); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if got := f.parents(t); len(got) != 1 || got[200].PID != 200 {
		t.Errorf("parents %v, want parent 200 from the newer snapshot", got)
	}
}

func (g *gatedDetections) callCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

type failingDetections struct{}

func (failingDetections) SyncFirstDetections(context.Context, string, []store.ParentKey, time.Time) (map[store.ParentKey]time.Time, error) {
	return nil, errors.New("connection refused")
}

func TestASnapshotThatCannotBeStoredKeepsThePreviousState(t *testing.T) {
	f := newFixture(t)
	f.report(t, t0, parent(100, 5000, 101))
	f.state.detections = failingDetections{}
	err := f.state.Report(context.Background(), f.agentID, protocol.Snapshot{ScannedAt: t0.Add(5 * time.Second), Parents: []protocol.Parent{}})
	if err == nil {
		t.Error("Report succeeded although the database failed")
	}
	if got := f.parents(t); len(got) != 1 {
		t.Errorf("parents %v, want the previous parent 100 kept", got)
	}
}

func TestReadingTheStateDoesNotWaitForTheDatabase(t *testing.T) {
	f := newFixture(t)
	f.report(t, t0, parent(100, 5000, 101))
	g := &gatedDetections{Detections: f.store, second: make(chan struct{})}
	f.state.detections = g
	go func() {
		_ = f.state.Report(context.Background(), f.agentID, protocol.Snapshot{ScannedAt: t0.Add(time.Second), Parents: []protocol.Parent{}})
	}()
	for g.callCount() == 0 {
		time.Sleep(time.Millisecond)
	}
	start := time.Now()
	f.state.Connected(f.agentID)
	f.state.Agent(f.agentID)
	if waited := time.Since(start); waited > 100*time.Millisecond {
		t.Errorf("reading the state waited %v for a snapshot's database call", waited)
	}
}
