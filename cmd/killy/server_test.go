package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestServeDoesNotListenWhenTheMigrationFails(t *testing.T) {
	listened := false
	// The timeout only bounds a wrong serve that goes on to listen.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := serve(ctx, quiet,
		func(context.Context) error { return errors.New("migration 2 failed") },
		func() (net.Listener, error) { listened = true; return net.Listen("tcp", "127.0.0.1:0") },
		http.NotFoundHandler(), func() {})
	if err == nil {
		t.Error("serve returned nil after a failed migration")
	}
	if listened {
		t.Error("serve listened although the migration failed")
	}
}

func TestServeMigratesBeforeListening(t *testing.T) {
	var steps []string
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, quiet,
			func(context.Context) error { steps = append(steps, "migrate"); return nil },
			func() (net.Listener, error) {
				steps = append(steps, "listen")
				cancel()
				return net.Listen("tcp", "127.0.0.1:0")
			},
			http.NotFoundHandler(), func() {})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not stop after the context was cancelled")
	}
	if !slices.Equal(steps, []string{"migrate", "listen"}) {
		t.Errorf("steps %v, want migrate then listen", steps)
	}
}

func TestEveryTicksAtOnceAndThenEveryInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := time.Now()
	ticks := every(ctx, 50*time.Millisecond)
	var at []time.Duration
	for range 3 {
		select {
		case <-ticks:
			at = append(at, time.Since(start))
		case <-time.After(2 * time.Second):
			t.Fatal("no tick")
		}
	}
	if at[0] > 40*time.Millisecond || at[1] < 40*time.Millisecond || at[2] < 90*time.Millisecond {
		t.Errorf("ticks at %v, want one at once, then about every 50 ms", at)
	}
}

func TestServeShutsDownPromptlyWithAStreamOpen(t *testing.T) {
	stop := make(chan struct{})
	opened := make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(opened)
		select {
		case <-stop:
		case <-r.Context().Done():
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	addr := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, quiet,
			func(context.Context) error { return nil },
			func() (net.Listener, error) {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				if err == nil {
					addr <- ln.Addr().String()
				}
				return ln, err
			},
			h, func() { close(stop) })
	}()
	res, err := http.Get("http://" + <-addr)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	<-opened
	start := time.Now()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve: %v, want a clean stop", err)
		}
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("shutdown took %v, want well under the 10 s limit", d)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not return")
	}
}

func TestTheDemoAgentIDIsReadAsAUUIDInLowerCase(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for raw, want := range map[string]string{
		"":                                     "",
		"3e7d58a1-d083-4b00-b0c5-105c3e845947": "3e7d58a1-d083-4b00-b0c5-105c3e845947",
		"3E7D58A1-D083-4B00-B0C5-105C3E845947": "3e7d58a1-d083-4b00-b0c5-105c3e845947",
		// Not a UUID: PostgreSQL would answer an error, not "no such agent"; treated as unset.
		"demo-machine": "",
	} {
		if got := demoAgentID(raw, log); got != want {
			t.Errorf("demoAgentID(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestADemoAgentIDThatIsNotAUUIDIsLogged(t *testing.T) {
	var buf bytes.Buffer
	demoAgentID("demo-machine", slog.New(slog.NewTextHandler(&buf, nil)))
	if !strings.Contains(buf.String(), "KILLY_DEMO_AGENT_ID") || !strings.Contains(buf.String(), "demo-machine") {
		t.Errorf("log %q, want a line naming KILLY_DEMO_AGENT_ID and the value", buf.String())
	}
	buf.Reset()
	demoAgentID("", slog.New(slog.NewTextHandler(&buf, nil)))
	if buf.Len() != 0 {
		t.Errorf("log %q for an unset value, want nothing", buf.String())
	}
}

func TestTheServerDoesNotStartWithACORSOriginThatIsNotAnOrigin(t *testing.T) {
	// The database is never reached: the setting is checked first, so a mistake cannot become
	// "every origin" or a setting that silently matches nothing.
	t.Setenv("KILLY_DATABASE_URL", "postgres://killy@127.0.0.1:1/killy")
	for _, value := range []string{"*", "https://killy.sumire-sakamoto.co.jp/demo", "https://a.example,"} {
		t.Setenv("KILLY_CORS_ORIGINS", value)
		var stderr bytes.Buffer
		if code := runServer([]string{"-listen", "127.0.0.1:0"}, &stderr); code != 1 {
			t.Errorf("KILLY_CORS_ORIGINS=%q: exit code %d, want 1", value, code)
		}
		if !strings.Contains(stderr.String(), "KILLY_CORS_ORIGINS") {
			t.Errorf("KILLY_CORS_ORIGINS=%q: output %q, want a line naming the variable", value, stderr.String())
		}
	}
}
