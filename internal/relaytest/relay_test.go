// Package relaytest runs the Killy server and the agent's command client together.
package relaytest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/sumiredc/killy/internal/agent"
	"github.com/sumiredc/killy/internal/agent/kill"
	"github.com/sumiredc/killy/internal/clock"
	"github.com/sumiredc/killy/internal/proc"
	"github.com/sumiredc/killy/internal/server"
	"github.com/sumiredc/killy/internal/server/command"
	"github.com/sumiredc/killy/internal/server/live"
	"github.com/sumiredc/killy/internal/server/memstore"
	"github.com/sumiredc/killy/internal/server/pairing"
	"github.com/sumiredc/killy/internal/server/ratelimit"
	"github.com/sumiredc/killy/internal/server/sse"
)

// machine is a fake Linux with one parent (PID 100) holding one zombie (PID 101). A SIGKILL to the
// parent turns it into a zombie and its zombie is reaped.
type machine struct {
	mu      sync.Mutex
	parentZ bool
	reaped  bool
	signals int
}

func (m *machine) Open(name string) (fs.File, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := "S"
	if m.parentZ {
		state = "Z"
	}
	root := fstest.MapFS{
		"1/stat":      {Data: []byte(stat(1, "S", 0, 1))},
		"100/stat":    {Data: []byte(stat(100, state, 1, 5000))},
		"500/stat":    {Data: []byte(stat(500, "S", 1, 900))},
		"100/cmdline": {Data: []byte("sleep\x00600\x00")},
	}
	if !m.reaped {
		root["101/stat"] = &fstest.MapFile{Data: []byte(stat(101, "Z", 100, 5001))}
	}
	return root.Open(name)
}

func stat(pid int, state string, ppid int, start int64) string {
	return fmt.Sprintf("%d (p) %s %d 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 %d 0 0\n", pid, state, ppid, start)
}

func (m *machine) Signals() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.signals
}

type pidfds struct{ m *machine }

func (p pidfds) Open(pid int) (kill.Pidfd, error) {
	if pid != 100 {
		return -1, syscall.ESRCH
	}
	return 3, nil
}

func (p pidfds) SendKill(kill.Pidfd) error {
	p.m.mu.Lock()
	defer p.m.mu.Unlock()
	p.m.signals++
	p.m.parentZ, p.m.reaped = true, true
	return nil
}

func (p pidfds) Close(kill.Pidfd) error { return nil }

// instant is a kill clock whose Sleep returns at once.
type instant struct {
	mu  sync.Mutex
	now time.Time
}

func (c *instant) Now() time.Time        { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *instant) Sleep(d time.Duration) { c.mu.Lock(); defer c.mu.Unlock(); c.now = c.now.Add(d) }

func newServer() *server.Server {
	c := clock.System{}
	mem := memstore.New()
	return &server.Server{
		Store:               mem,
		Codes:               pairing.New(c),
		FirstRuns:           ratelimit.New(server.FirstRunLimit, server.FirstRunPeriod, c),
		DeviceRegistrations: ratelimit.New(server.DeviceRegistrationLimit, server.DeviceRegistrationPeriod, c),
		FailedClaims:        ratelimit.New(server.FailedClaimLimit, server.FailedClaimPeriod, c),
		Kills:               ratelimit.New(server.KillLimit, server.KillPeriod, c),
		Commands:            command.New(c, time.After),
		Live:                live.New(c, mem),
		Broker:              sse.NewBroker(),
		HeartbeatInterval:   time.Hour,
		Log:                 slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func post(t *testing.T, url, token, body string, into any) int {
	t.Helper()
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if into != nil {
		_ = json.NewDecoder(res.Body).Decode(into)
	}
	return res.StatusCode
}

func TestTheSameKillIDTwiceSignalsOnceEndToEnd(t *testing.T) {
	srv := newServer()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	var pair struct{ AgentToken, AgentID, Code string }
	var pairRaw map[string]string
	post(t, ts.URL+"/agent/v1/pairings", "", `{"hostname":"web-1"}`, &pairRaw)
	pair.AgentToken, pair.AgentID, pair.Code = pairRaw["agent_token"], pairRaw["agent_id"], pairRaw["code"]
	var dev map[string]string
	post(t, ts.URL+"/app/v1/devices", "", `{}`, &dev)
	device := dev["device_token"]
	if status := post(t, ts.URL+"/app/v1/pairings/claim", device, `{"code":"`+pair.Code+`"}`, nil); status != http.StatusOK {
		t.Fatalf("claim: %d", status)
	}

	m := &machine{}
	store, err := kill.OpenFileStore(filepath.Join(t.TempDir(), "outcomes.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	killer := &kill.Killer{Proc: &proc.Scanner{Root: m}, Pidfd: pidfds{m}, Store: store, Clock: &instant{}, SelfPID: func() int { return 500 },
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &agent.CommandClient{Server: ts.URL, Token: pair.AgentToken, Client: &http.Client{}, Killer: killer,
		Backoff: &agent.Backoff{Rand: func() float64 { return 0 }}, Log: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		Sleep: agent.SleepContext}
	go client.Run(ctx)
	deadline := time.Now().Add(3 * time.Second)
	for !srv.Commands.Attached(pair.AgentID) {
		if time.Now().After(deadline) {
			t.Fatal("the agent's command stream never attached")
		}
		time.Sleep(time.Millisecond)
	}

	body := `{"id":"16fd2706-8baf-433b-82eb-8c7fada847da","server_id":"` + pair.AgentID + `","ppid":100,"starttime":5000}`
	var first, again map[string]any
	if status := post(t, ts.URL+"/app/v1/kills", device, body, &first); status != http.StatusOK {
		t.Fatalf("first kill: %d", status)
	}
	if status := post(t, ts.URL+"/app/v1/kills", device, body, &again); status != http.StatusOK {
		t.Fatalf("resent kill: %d", status)
	}
	// Without the agent's record of the ID the resend would end already_gone (the parent is a zombie
	// by then), so the equal outcomes are what show the record answered; the count shows no second signal.
	if first["outcome"] != "killed" || fmt.Sprint(first) != fmt.Sprint(again) {
		t.Errorf("outcomes %v and %v, want killed twice", first, again)
	}
	if n := m.Signals(); n != 1 {
		t.Errorf("SIGKILL sent %d times, want 1", n)
	}
}
