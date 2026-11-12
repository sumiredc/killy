package agent

import (
	"context"
	"log/slog"
	"maps"
	"time"

	"github.com/sumiredc/killy/internal/protocol"
)

const (
	// ScanInterval is how often the agent reads /proc.
	ScanInterval = 5 * time.Second
	// ReportInterval is how often a snapshot is sent when the set of parents does not change.
	ReportInterval = 15 * time.Second
)

// Scanner reads the parents of zombies; *proc.Scanner in production.
type Scanner interface {
	Scan() ([]protocol.Parent, error)
}

// Reporter scans on every tick and sends a full snapshot when the set of parents has changed
// since the last snapshot that was sent, when ReportInterval has passed since then, or when the
// last send failed. It is not safe for concurrent use.
type Reporter struct {
	Scanner  Scanner
	Send     func(ctx context.Context, snap protocol.Snapshot) error
	Hostname string
	Version  string
	Log      *slog.Logger

	sent     bool
	sentAt   time.Time
	sentKeys map[parentKey]bool
}

type parentKey struct {
	pid       int
	starttime int64
}

// Run calls Tick for every value of ticks until ctx is done.
func (r *Reporter) Run(ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticks:
			r.Tick(ctx, now)
		}
	}
}

// Tick scans once and sends a snapshot taken at now if one is due.
func (r *Reporter) Tick(ctx context.Context, now time.Time) {
	parents, err := r.Scanner.Scan()
	if err != nil {
		r.Log.Error("scan /proc", "err", err)
		return
	}
	keys := make(map[parentKey]bool, len(parents))
	for _, p := range parents {
		keys[parentKey{p.PID, p.StartTime}] = true
	}
	// Ticks of a real ticker arrive a little late by varying amounts, so three ticks can span a
	// little under 15 s. Half a scan interval of slack keeps the periodic send on the third tick
	// instead of the fourth.
	due := !r.sent || !now.Before(r.sentAt.Add(ReportInterval-ScanInterval/2)) || !maps.Equal(keys, r.sentKeys)
	if !due {
		return
	}
	snap := protocol.Snapshot{Hostname: r.Hostname, Version: r.Version, ScannedAt: now.UTC(), Parents: parents}
	if err := r.Send(ctx, snap); err != nil {
		// Nothing is recorded as sent, so the snapshot is still due on the next tick.
		r.Log.Error("send snapshot", "err", err)
		return
	}
	r.sent, r.sentAt, r.sentKeys = true, now, keys
}
