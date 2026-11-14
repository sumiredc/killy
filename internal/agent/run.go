package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/sumiredc/killy/internal/protocol"
)

// ErrNotPaired means agent.json does not exist yet.
var ErrNotPaired = errors.New("this server is not paired yet; run killy agent pair --server <url> first")

type RunOptions struct {
	ConfigPath string
	Client     *http.Client
	Log        *slog.Logger
	Hostname   string
	Version    string
	Scanner    Scanner
	// PidfdAvailable reports whether kills can be sent at all on this kernel.
	PidfdAvailable func() bool
	// Ticks drives the scans: one value per scan.
	Ticks <-chan time.Time
	// Killer runs the kills that arrive on the command stream.
	Killer Killer
	// StreamClient opens the command stream; it must have no overall timeout.
	StreamClient *http.Client
	// Rand jitters the reconnect waits; math/rand/v2.Float64 in production.
	Rand func() float64
	// Sleep waits between reconnect attempts and returns early when ctx is done.
	Sleep func(ctx context.Context, d time.Duration)
}

// SleepContext waits for d or until ctx is done.
func SleepContext(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// Run reports snapshots to the Killy server saved in agent.json until ctx is done. Every
// connection it makes is outbound; it never listens.
func Run(ctx context.Context, opts RunOptions) error {
	cfg, err := LoadConfig(opts.ConfigPath)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotPaired
	}
	if err != nil {
		return err
	}
	if err := checkServerURL(cfg.Server); err != nil {
		return err
	}
	// Without pidfd every kill is refused, but the operator should still see the zombies, so the
	// agent keeps reporting.
	if !opts.PidfdAvailable() {
		opts.Log.Error("pidfd is not available (Linux 5.3 or later is needed, and seccomp must allow pidfd_open); every kill will be refused with pidfd_unavailable")
	}
	r := &Reporter{
		Scanner:  opts.Scanner,
		Hostname: opts.Hostname,
		Version:  opts.Version,
		Log:      opts.Log,
		Send: func(ctx context.Context, snap protocol.Snapshot) error {
			return sendSnapshot(ctx, opts.Client, cfg, snap)
		},
	}
	commands := &CommandClient{
		Server:  cfg.Server,
		Token:   cfg.Token,
		Client:  opts.StreamClient,
		Killer:  opts.Killer,
		Version: opts.Version,
		Backoff: &Backoff{Rand: opts.Rand},
		Log:     opts.Log,
		Sleep:   opts.Sleep,
	}
	done := make(chan struct{})
	go func() { commands.Run(ctx); close(done) }()
	opts.Log.Info("reporting", "server", cfg.Server)
	r.Run(ctx, opts.Ticks)
	<-done
	return nil
}

func sendSnapshot(ctx context.Context, client *http.Client, cfg Config, snap protocol.Snapshot) error {
	body, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Server+"/agent/v1/snapshots", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	res, err := noRedirects(client).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("the Killy server answered %s", res.Status)
	}
	return nil
}

// noRedirects returns a copy of client that does not follow redirects: a redirect would resend
// the Authorization header to wherever it points, including plain http on the same host.
func noRedirects(client *http.Client) *http.Client {
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &c
}
