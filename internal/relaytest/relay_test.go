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
	"os"
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
	// cgroup, when set, is the content of 100/cgroup.
	cgroup string
	// gate, when set, holds the first read of 100/stat until a second one comes (or 500 ms pass),
	// so two kills that are not kept apart both read the parent before either signals.
	gate     bool
	arrivals int
	opened   chan struct{}
}

func (m *machine) Open(name string) (fs.File, error) {
	root := m.files()
	// What was read is fixed before the wait, as a check reads the process before it signals.
	if name == "100/stat" {
		m.waitAtGate()
	}
	return root.Open(name)
}

func (m *machine) files() fstest.MapFS {
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
	if m.cgroup != "" {
		root["100/cgroup"] = &fstest.MapFile{Data: []byte(m.cgroup)}
	}
	return root
}

func (m *machine) waitAtGate() {
	m.mu.Lock()
	if !m.gate {
		m.mu.Unlock()
		return
	}
	m.arrivals++
	if m.arrivals == 1 {
		m.opened = make(chan struct{})
		opened := m.opened
		m.mu.Unlock()
		select {
		case <-opened:
		case <-time.After(500 * time.Millisecond):
		}
		m.mu.Lock()
		m.gate = false
		m.mu.Unlock()
		return
	}
	close(m.opened)
	m.gate = false
	m.mu.Unlock()
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

// relay is a Killy server with one paired agent running its command stream, and one phone linked to it.
type relay struct {
	url, device, agentID string
	machine              *machine
	outcomes             string
}

func startRelay(t *testing.T, version string) *relay {
	t.Helper()
	srv := newServer()
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	var pair map[string]string
	post(t, ts.URL+"/agent/v1/pairings", "", `{"hostname":"web-1"}`, &pair)
	var dev map[string]string
	post(t, ts.URL+"/app/v1/devices", "", `{}`, &dev)
	r := &relay{url: ts.URL, device: dev["device_token"], agentID: pair["agent_id"], machine: &machine{},
		outcomes: filepath.Join(t.TempDir(), "outcomes.jsonl")}
	if status := post(t, ts.URL+"/app/v1/pairings/claim", r.device, `{"code":"`+pair["code"]+`"}`, nil); status != http.StatusOK {
		t.Fatalf("claim: %d", status)
	}

	runAgent(t, srv, ts.URL, pair, r.machine, r.outcomes, version, false)
	return r
}

// runAgent runs the agent's command client for the paired agent until the test ends, and waits
// until its command stream is attached.
func runAgent(t *testing.T, srv *server.Server, url string, pair map[string]string, m *machine, outcomes, version string, demo bool) {
	t.Helper()
	store, err := kill.OpenFileStore(outcomes)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	killer := &kill.Killer{Proc: &proc.Scanner{Root: m}, Pidfd: pidfds{m}, Store: store, Clock: &instant{}, SelfPID: func() int { return 500 },
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Demo: demo}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	client := &agent.CommandClient{Server: url, Token: pair["agent_token"], Client: &http.Client{}, Killer: killer, Version: version,
		Backoff: &agent.Backoff{Rand: func() float64 { return 0 }}, Log: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		Sleep: agent.SleepContext}
	go client.Run(ctx)
	deadline := time.Now().Add(3 * time.Second)
	for !srv.Commands.Attached(pair["agent_id"]) {
		if time.Now().After(deadline) {
			t.Fatal("the agent's command stream never attached")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestTheSameKillIDTwiceSignalsOnceEndToEnd(t *testing.T) {
	r := startRelay(t, "dev")
	body := `{"id":"16fd2706-8baf-433b-82eb-8c7fada847da","server_id":"` + r.agentID + `","ppid":100,"starttime":5000}`
	var first, again map[string]any
	if status := post(t, r.url+"/app/v1/kills", r.device, body, &first); status != http.StatusOK {
		t.Fatalf("first kill: %d", status)
	}
	if status := post(t, r.url+"/app/v1/kills", r.device, body, &again); status != http.StatusOK {
		t.Fatalf("resent kill: %d", status)
	}
	// Without the agent's record of the ID the resend would end already_gone (the parent is a zombie
	// by then), so the equal outcomes are what show the record answered; the count shows no second signal.
	if first["outcome"] != "killed" || fmt.Sprint(first) != fmt.Sprint(again) {
		t.Errorf("outcomes %v and %v, want killed twice", first, again)
	}
	if n := r.machine.Signals(); n != 1 {
		t.Errorf("SIGKILL sent %d times, want 1", n)
	}
}

func TestACheckGoesToTheAgentAndBackAndChangesNothingEndToEnd(t *testing.T) {
	r := startRelay(t, "1.2.3")
	var got map[string]any
	if status := post(t, r.url+"/app/v1/servers/"+r.agentID+"/check", r.device, "", &got); status != http.StatusOK {
		t.Fatalf("check: %d", status)
	}
	if fmt.Sprint(got) != "map[result:ok version:1.2.3]" {
		t.Errorf("check %v, want {result: ok, version: 1.2.3} only", got)
	}
	if n := r.machine.Signals(); n != 0 {
		t.Errorf("SIGKILL sent %d times by a check, want 0", n)
	}
	if b, err := os.ReadFile(r.outcomes); err != nil || len(b) != 0 {
		t.Errorf("outcomes file %q (%v), want it empty after a check", b, err)
	}
}

// demoRelay is a Killy server offering a demo-mode agent as the public demo, whose parent 100 is in
// summon's cgroup, and two phones that added it with "Try the demo server".
type demoRelay struct {
	url, agentID string
	devices      [2]string
	machine      *machine
}

func startDemoRelay(t *testing.T) *demoRelay {
	t.Helper()
	srv := newServer()
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	var pair map[string]string
	post(t, ts.URL+"/agent/v1/pairings", "", `{"hostname":"demo-linux-01"}`, &pair)
	srv.DemoAgentID = pair["agent_id"]
	r := &demoRelay{url: ts.URL, agentID: pair["agent_id"], machine: &machine{cgroup: "0::" + proc.SummonCgroup + "\n"}}
	for i := range r.devices {
		var dev map[string]string
		post(t, ts.URL+"/app/v1/devices", "", `{}`, &dev)
		r.devices[i] = dev["device_token"]
		if status := post(t, ts.URL+"/app/v1/demo", r.devices[i], "", nil); status != http.StatusOK {
			t.Fatalf("add the demo server: %d", status)
		}
	}
	runAgent(t, srv, ts.URL, pair, r.machine, filepath.Join(t.TempDir(), "outcomes.jsonl"), "dev", true)
	return r
}

// kill sends a kill of the demo parent with a new ID from phone i.
func (r *demoRelay) kill(t *testing.T, i int, id string) string {
	t.Helper()
	var got map[string]any
	body := `{"id":"` + id + `","server_id":"` + r.agentID + `","ppid":100,"starttime":5000}`
	if status := post(t, r.url+"/app/v1/kills", r.devices[i], body, &got); status != http.StatusOK {
		t.Errorf("kill from phone %d: %d", i, status)
	}
	return fmt.Sprint(got["outcome"])
}

func TestTwoVisitorsKillingTheSameDemoParentInTurnSignalOnce(t *testing.T) {
	r := startDemoRelay(t)
	first := r.kill(t, 0, "2a1f0c3e-5d6b-4c7a-8e9f-0a1b2c3d4e5f")
	second := r.kill(t, 1, "7b8c9d0e-1f2a-4b3c-9d4e-5f6a7b8c9d0e")
	if first != "killed" || second != "already_gone" {
		t.Errorf("outcomes %s then %s, want killed then already_gone", first, second)
	}
	if n := r.machine.Signals(); n != 1 {
		t.Errorf("SIGKILL sent %d times, want 1", n)
	}
}

func TestTwoVisitorsKillingTheSameDemoParentAtOnceSignalOnce(t *testing.T) {
	r := startDemoRelay(t)
	r.machine.mu.Lock()
	r.machine.gate = true
	r.machine.mu.Unlock()
	ids := [2]string{"3c4d5e6f-7a8b-4c9d-8e0f-1a2b3c4d5e6f", "8d9e0f1a-2b3c-4d5e-af6a-7b8c9d0e1f2a"}
	var outcomes [2]string
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Go(func() { outcomes[i] = r.kill(t, i, ids[i]) })
	}
	wg.Wait()
	got := map[string]int{outcomes[0]: 1}
	got[outcomes[1]]++
	if got["killed"] != 1 || got["already_gone"] != 1 {
		t.Errorf("outcomes %v, want one killed and one already_gone", outcomes)
	}
	if n := r.machine.Signals(); n != 1 {
		t.Errorf("SIGKILL sent %d times, want 1", n)
	}
}
