package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/protocol"
)

// snapshotSink accepts POST /agent/v1/snapshots like the Killy server and keeps what it got.
type snapshotSink struct {
	*httptest.Server
	status int

	mu    sync.Mutex
	got   []protocol.Snapshot
	auths []string
}

func newSnapshotSink(t *testing.T) *snapshotSink {
	s := &snapshotSink{status: http.StatusNoContent}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/agent/v1/snapshots" {
			http.NotFound(w, r)
			return
		}
		var snap protocol.Snapshot
		_ = json.NewDecoder(r.Body).Decode(&snap)
		s.mu.Lock()
		s.got = append(s.got, snap)
		s.auths = append(s.auths, r.Header.Get("Authorization"))
		s.mu.Unlock()
		w.WriteHeader(s.status)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *snapshotSink) received() ([]protocol.Snapshot, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]protocol.Snapshot(nil), s.got...), append([]string(nil), s.auths...)
}

// runOnce runs the agent for one tick and returns its log.
func runOnce(t *testing.T, opts RunOptions) (string, error) {
	t.Helper()
	var logs bytes.Buffer
	ticks := make(chan time.Time, 1)
	ticks <- t0
	ctx, cancel := context.WithCancel(context.Background())
	opts.Ticks = ticks
	opts.Log = slog.New(slog.NewTextHandler(&logs, nil))
	if opts.Client == nil {
		opts.Client = http.DefaultClient
	}
	if opts.Scanner == nil {
		opts.Scanner = &fakeScanner{parents: []protocol.Parent{parentA}}
	}
	if opts.PidfdAvailable == nil {
		opts.PidfdAvailable = func() bool { return true }
	}
	if opts.Killer == nil {
		opts.Killer = &fakeKiller{}
	}
	opts.StreamClient = http.DefaultClient
	opts.Rand = func() float64 { return 0 }
	opts.Sleep = SleepContext
	done := make(chan error, 1)
	go func() { done <- Run(ctx, opts) }()
	time.Sleep(200 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		return logs.String(), err
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
		return "", nil
	}
}

func TestRunWithoutAgentJSONAsksToPairFirst(t *testing.T) {
	_, err := runOnce(t, RunOptions{ConfigPath: configPath(t)})
	if !errors.Is(err, ErrNotPaired) || !strings.Contains(err.Error(), "killy agent pair") {
		t.Errorf("err = %v, want ErrNotPaired asking to run killy agent pair", err)
	}
}

func TestRunReportsToTheSavedServerWithTheSavedToken(t *testing.T) {
	sink := newSnapshotSink(t)
	path := configPath(t)
	writeConfig(t, path, Config{Server: sink.URL, Token: issuedToken})
	if _, err := runOnce(t, RunOptions{ConfigPath: path, Hostname: "web-1", Version: "dev"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, auths := sink.received()
	if len(got) != 1 || len(got[0].Parents) != 1 || got[0].Parents[0].PID != 100 || got[0].Hostname != "web-1" {
		t.Fatalf("received %+v, want one snapshot from web-1 with parent 100", got)
	}
	if auths[0] != "Bearer "+issuedToken {
		t.Errorf("Authorization = %q, want the saved token", auths[0])
	}
}

func TestRunLogsThatPidfdIsUnavailableAndKeepsReporting(t *testing.T) {
	sink := newSnapshotSink(t)
	path := configPath(t)
	writeConfig(t, path, Config{Server: sink.URL, Token: issuedToken})
	logs, err := runOnce(t, RunOptions{ConfigPath: path, PidfdAvailable: func() bool { return false }})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(logs, "pidfd") || !strings.Contains(logs, "level=ERROR") {
		t.Errorf("log %q, want an error about pidfd", logs)
	}
	if got, _ := sink.received(); len(got) != 1 {
		t.Errorf("received %d snapshots, want 1: reporting goes on without pidfd", len(got))
	}
}

func TestSendSnapshotReportsARefusal(t *testing.T) {
	sink := newSnapshotSink(t)
	sink.status = http.StatusUnauthorized
	err := sendSnapshot(context.Background(), http.DefaultClient, Config{Server: sink.URL, Token: issuedToken},
		protocol.Snapshot{ScannedAt: t0, Parents: []protocol.Parent{}})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("err = %v, want an error naming the 401", err)
	}
}

func TestRunAlsoOpensTheCommandStream(t *testing.T) {
	server := newCommandServer(t)
	path := configPath(t)
	writeConfig(t, path, Config{Server: server.URL, Token: issuedToken})
	var logs bytes.Buffer
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, RunOptions{ConfigPath: path, Client: http.DefaultClient, Log: slog.New(slog.NewTextHandler(&logs, nil)),
			Scanner: &fakeScanner{}, PidfdAvailable: func() bool { return true }, Ticks: ticks,
			Killer: &fakeKiller{out: protocol.Outcome{Outcome: protocol.AlreadyGone}}, StreamClient: http.DefaultClient,
			Rand: func() float64 { return 0 }, Sleep: SleepContext})
	}()
	waitConnections(t, server, 1)
	server.commands <- killEvent("16fd2706-8baf-433b-82eb-8c7fada847da")
	select {
	case r := <-server.got:
		if r.Outcome != protocol.AlreadyGone {
			t.Errorf("result %+v, want already_gone", r)
		}
	case <-time.After(3 * time.Second):
		t.Error("no result")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestRunAnswersAPingWithItsOwnVersion(t *testing.T) {
	server := newCommandServer(t)
	path := configPath(t)
	writeConfig(t, path, Config{Server: server.URL, Token: issuedToken})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, RunOptions{ConfigPath: path, Client: http.DefaultClient, Log: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
			Version: "9.9.9", Scanner: &fakeScanner{}, PidfdAvailable: func() bool { return true }, Ticks: make(chan time.Time),
			Killer: &fakeKiller{}, StreamClient: http.DefaultClient, Rand: func() float64 { return 0 }, Sleep: SleepContext})
	}()
	waitConnections(t, server, 1)
	server.commands <- pingEvent("p-1")
	select {
	case r := <-server.got:
		if r.Type != protocol.ResultPong || r.Version != "9.9.9" {
			t.Errorf("result %+v, want a pong with the version Run was given", r)
		}
	case <-time.After(3 * time.Second):
		t.Error("no pong")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}
