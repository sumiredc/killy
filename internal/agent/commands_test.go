package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/agent/kill"
	"github.com/sumiredc/killy/internal/protocol"
)

func TestBackoffDoublesFrom1SecondTo30Seconds(t *testing.T) {
	b := &Backoff{Rand: func() float64 { return 0 }}
	want := []time.Duration{1, 2, 4, 8, 16, 30, 30, 30}
	for i, w := range want {
		if got := b.Next(); got != w*time.Second {
			t.Errorf("attempt %d: %v, want %v", i+1, got, w*time.Second)
		}
	}
}

func TestBackoffAddsUpToAQuarterAtRandom(t *testing.T) {
	high := &Backoff{Rand: func() float64 { return 0.999999 }}
	for i, base := range []time.Duration{1, 2, 4, 8, 16, 30, 30} {
		got := high.Next()
		lo, hi := base*time.Second, base*time.Second+base*time.Second/4
		if got < lo || got >= hi {
			t.Errorf("attempt %d: %v, want in [%v, %v)", i+1, got, lo, hi)
		}
	}
	half := &Backoff{Rand: func() float64 { return 0.5 }}
	if got := half.Next(); got != 1125*time.Millisecond {
		t.Errorf("first wait with rand 0.5: %v, want 1.125s", got)
	}
}

func TestBackoffStartsAgainAfterReset(t *testing.T) {
	b := &Backoff{Rand: func() float64 { return 0 }}
	b.Next()
	b.Next()
	b.Reset()
	if got := b.Next(); got != time.Second {
		t.Errorf("after Reset: %v, want 1s", got)
	}
}

// commandServer is a Killy server's command stream and results endpoint for one agent.
type commandServer struct {
	*httptest.Server
	commands chan string // SSE event blocks to send, one per stream write
	mu       sync.Mutex
	results  []protocol.CommandResult
	auths    []string
	connects int
	// dropAfter closes each stream after that many events (0: keep open).
	got chan protocol.CommandResult
}

func newCommandServer(t *testing.T) *commandServer {
	c := &commandServer{commands: make(chan string, 16), got: make(chan protocol.CommandResult, 16)}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		c.auths = append(c.auths, r.Header.Get("Authorization"))
		c.mu.Unlock()
		switch {
		case r.Method == "GET" && r.URL.Path == "/agent/v1/commands":
			c.mu.Lock()
			c.connects++
			c.mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			for {
				select {
				case <-r.Context().Done():
					return
				case ev, ok := <-c.commands:
					if !ok || ev == "close" {
						return
					}
					fmt.Fprint(w, ev)
					w.(http.Flusher).Flush()
				}
			}
		case r.Method == "POST" && r.URL.Path == "/agent/v1/results":
			var res protocol.CommandResult
			_ = json.NewDecoder(r.Body).Decode(&res)
			c.mu.Lock()
			c.results = append(c.results, res)
			c.mu.Unlock()
			c.got <- res
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(c.Close)
	return c
}

func (c *commandServer) connections() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connects
}

func killEvent(id string) string {
	return "event: kill\ndata: {\"id\":\"" + id + "\",\"ppid\":100,\"starttime\":5000}\n\n"
}

type fakeKiller struct {
	mu    sync.Mutex
	calls []protocol.KillCommand
	out   protocol.Outcome
	err   error
}

func (k *fakeKiller) Handle(cmd protocol.KillCommand) (protocol.Outcome, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.calls = append(k.calls, cmd)
	return k.out, k.err
}

func (k *fakeKiller) count() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.calls)
}

type clientFixture struct {
	server *commandServer
	killer *fakeKiller
	logs   bytes.Buffer
	sleeps chan time.Duration
	cancel context.CancelFunc
	done   chan struct{}
}

func startClient(t *testing.T) *clientFixture {
	f := &clientFixture{server: newCommandServer(t), killer: &fakeKiller{out: protocol.Outcome{Outcome: protocol.Killed, Reaped: 2}}, sleeps: make(chan time.Duration, 16), done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	c := &CommandClient{
		Server:  f.server.URL,
		Token:   issuedToken,
		Client:  &http.Client{},
		Killer:  f.killer,
		Backoff: &Backoff{Rand: func() float64 { return 0 }},
		Log:     slog.New(slog.NewTextHandler(&f.logs, nil)),
		Sleep: func(ctx context.Context, d time.Duration) {
			f.sleeps <- d
			select {
			case <-ctx.Done():
			case <-time.After(10 * time.Millisecond):
			}
		},
	}
	go func() { c.Run(ctx); close(f.done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-f.done:
		case <-time.After(5 * time.Second):
			t.Error("Run did not return after cancel")
		}
	})
	return f
}

func (f *clientFixture) result(t *testing.T) protocol.CommandResult {
	t.Helper()
	select {
	case r := <-f.server.got:
		return r
	case <-time.After(3 * time.Second):
		t.Fatal("no result posted within 3 s")
		return protocol.CommandResult{}
	}
}

func waitConnections(t *testing.T, s *commandServer, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for s.connections() < n {
		if time.Now().After(deadline) {
			t.Fatalf("%d connections, want %d", s.connections(), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAKillCommandIsRunAndItsOutcomePostedBack(t *testing.T) {
	f := startClient(t)
	waitConnections(t, f.server, 1)
	f.server.commands <- killEvent("16fd2706-8baf-433b-82eb-8c7fada847da")
	r := f.result(t)
	want := protocol.CommandResult{ID: "16fd2706-8baf-433b-82eb-8c7fada847da", Type: "kill", Outcome: "killed", Reaped: 2}
	if r != want {
		t.Errorf("result %+v, want %+v", r, want)
	}
	if got := f.killer.calls[0]; got != (protocol.KillCommand{ID: want.ID, PPID: 100, StartTime: 5000}) {
		t.Errorf("killer got %+v", got)
	}
	f.server.mu.Lock()
	defer f.server.mu.Unlock()
	for _, a := range f.server.auths {
		if a != "Bearer "+issuedToken {
			t.Errorf("Authorization %q, want the agent token", a)
		}
	}
}

func TestAKillWithAnInvalidIDGetsNoResult(t *testing.T) {
	f := startClient(t)
	f.killer.err = kill.ErrInvalidKillID
	waitConnections(t, f.server, 1)
	f.server.commands <- killEvent("not-a-uuid")
	select {
	case r := <-f.server.got:
		t.Errorf("posted %+v, want nothing for an ignored command", r)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestOtherEventsAreIgnored(t *testing.T) {
	f := startClient(t)
	waitConnections(t, f.server, 1)
	f.server.commands <- ": hb\n\n"
	f.server.commands <- "event: ping\ndata: {\"id\":\"x\"}\n\n"
	f.server.commands <- killEvent("16fd2706-8baf-433b-82eb-8c7fada847da")
	f.result(t)
	if n := f.killer.count(); n != 1 {
		t.Errorf("killer called %d times, want 1", n)
	}
}

func TestTheStreamIsReopenedAfterItEndsWithBackoff(t *testing.T) {
	f := startClient(t)
	waitConnections(t, f.server, 1)
	f.server.commands <- "close"
	waitConnections(t, f.server, 2)
	select {
	case d := <-f.sleeps:
		if d != time.Second {
			t.Errorf("waited %v before reconnecting, want 1s", d)
		}
	default:
		t.Error("reconnected without waiting")
	}
}

func TestWaitsGrowWhileTheServerIsDown(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer down.Close()
	sleeps := make(chan time.Duration, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &CommandClient{Server: down.URL, Token: issuedToken, Client: &http.Client{}, Killer: &fakeKiller{},
		Backoff: &Backoff{Rand: func() float64 { return 0 }}, Log: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		Sleep: func(ctx context.Context, d time.Duration) { sleeps <- d }}
	go c.Run(ctx)
	for _, want := range []time.Duration{1, 2, 4, 8} {
		select {
		case d := <-sleeps:
			if d != want*time.Second {
				t.Errorf("wait %v, want %v", d, want*time.Second)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("no reconnect attempt")
		}
	}
}

// blockingKiller holds the kill of one ID until release is closed and answers others at once.
type blockingKiller struct {
	slowID  string
	release chan struct{}
}

func (k *blockingKiller) Handle(cmd protocol.KillCommand) (protocol.Outcome, error) {
	if cmd.ID == k.slowID {
		<-k.release
	}
	return protocol.Outcome{Outcome: protocol.Killed, Reaped: 1}, nil
}

func TestASlowKillDoesNotHoldUpTheNextCommand(t *testing.T) {
	server := newCommandServer(t)
	const slow, fast = "16fd2706-8baf-433b-82eb-8c7fada847da", "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	k := &blockingKiller{slowID: slow, release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &CommandClient{Server: server.URL, Token: issuedToken, Client: &http.Client{}, Killer: k,
		Backoff: &Backoff{Rand: func() float64 { return 0 }}, Log: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		Sleep: func(ctx context.Context, d time.Duration) {}}
	go c.Run(ctx)
	waitConnections(t, server, 1)
	server.commands <- killEvent(slow)
	server.commands <- killEvent(fast)
	select {
	case r := <-server.got:
		if r.ID != fast {
			t.Errorf("first result for %s, want the fast kill %s", r.ID, fast)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the second kill waited for the first")
	}
	close(k.release)
	select {
	case r := <-server.got:
		if r.ID != slow {
			t.Errorf("second result for %s, want %s", r.ID, slow)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the slow kill never answered")
	}
}

func TestEveryGoodConnectionStartsTheWaitsAgain(t *testing.T) {
	f := startClient(t)
	for i := 1; i <= 3; i++ {
		waitConnections(t, f.server, i)
		f.server.commands <- "close"
		select {
		case d := <-f.sleeps:
			if d != time.Second {
				t.Errorf("after good connection %d: waited %v, want 1s", i, d)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("no reconnect")
		}
	}
}

func TestRunWaitsForAKillInFlightAndPostsItsResult(t *testing.T) {
	server := newCommandServer(t)
	const slow = "16fd2706-8baf-433b-82eb-8c7fada847da"
	k := &blockingKiller{slowID: slow, release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	c := &CommandClient{Server: server.URL, Token: issuedToken, Client: &http.Client{}, Killer: k,
		Backoff: &Backoff{Rand: func() float64 { return 0 }}, Log: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		Sleep: func(ctx context.Context, d time.Duration) {}}
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	waitConnections(t, server, 1)
	server.commands <- killEvent(slow)
	time.Sleep(50 * time.Millisecond)
	cancel() // the agent is stopping while the kill waits for its zombies
	select {
	case <-done:
		t.Fatal("Run returned with a kill still in flight")
	case <-time.After(100 * time.Millisecond):
	}
	close(k.release)
	select {
	case r := <-server.got:
		if r.ID != slow {
			t.Errorf("result for %s, want %s", r.ID, slow)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the kill's result was not posted after the agent began to stop")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after the kill finished")
	}
}

func TestASilentStreamIsReopened(t *testing.T) {
	server := newCommandServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &CommandClient{Server: server.URL, Token: issuedToken, Client: &http.Client{}, Killer: &fakeKiller{},
		Backoff: &Backoff{Rand: func() float64 { return 0 }}, Log: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		Sleep: func(ctx context.Context, d time.Duration) {}, IdleTimeout: 100 * time.Millisecond}
	go c.Run(ctx)
	// The server sends nothing, not even heartbeats, as on a half-open connection.
	waitConnections(t, server, 2)
}

func TestTheDefaultIdleTimeoutIsThreeHeartbeats(t *testing.T) {
	if got := (&CommandClient{}).idleTimeout(); got != 60*time.Second {
		t.Errorf("idle timeout %v, want 60s (three 20 s heartbeats)", got)
	}
}

func TestAStreamThatKeepsSendingStaysOpen(t *testing.T) {
	server := newCommandServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &CommandClient{Server: server.URL, Token: issuedToken, Client: &http.Client{}, Killer: &fakeKiller{},
		Backoff: &Backoff{Rand: func() float64 { return 0 }}, Log: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		Sleep: func(ctx context.Context, d time.Duration) {}, IdleTimeout: 150 * time.Millisecond}
	go c.Run(ctx)
	waitConnections(t, server, 1)
	// Heartbeats every 50 ms for well over the idle timeout.
	for range 10 {
		server.commands <- ": hb\n\n"
		time.Sleep(50 * time.Millisecond)
	}
	if n := server.connections(); n != 1 {
		t.Errorf("%d connections, want 1: a stream with heartbeats must stay open", n)
	}
}
