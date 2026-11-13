package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"slices"
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
