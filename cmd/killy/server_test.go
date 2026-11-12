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
		http.NotFoundHandler())
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
			http.NotFoundHandler())
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
