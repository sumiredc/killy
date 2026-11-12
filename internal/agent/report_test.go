package agent

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/protocol"
)

var t0 = time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)

// fakeScanner returns parents, or err when set.
type fakeScanner struct {
	parents []protocol.Parent
	err     error
	scans   int
}

func (f *fakeScanner) Scan() ([]protocol.Parent, error) {
	f.scans++
	return append([]protocol.Parent{}, f.parents...), f.err
}

type reporterFixture struct {
	scanner *fakeScanner
	sent    []protocol.Snapshot
	sendErr error
	logs    bytes.Buffer
	r       *Reporter
}

func newReporter() *reporterFixture {
	f := &reporterFixture{scanner: &fakeScanner{parents: []protocol.Parent{}}}
	f.r = &Reporter{
		Scanner:  f.scanner,
		Hostname: "web-1",
		Version:  "dev",
		Log:      slog.New(slog.NewTextHandler(&f.logs, nil)),
		Send: func(_ context.Context, s protocol.Snapshot) error {
			f.sent = append(f.sent, s)
			return f.sendErr
		},
	}
	return f
}

func (f *reporterFixture) tickAt(d time.Duration) { f.r.Tick(context.Background(), t0.Add(d)) }

var (
	parentA = protocol.Parent{PID: 100, StartTime: 5000, Cmd: "a", Zombies: []int{101}}
	parentB = protocol.Parent{PID: 200, StartTime: 7000, Cmd: "b", Zombies: []int{201}}
)

func TestTheFirstScanIsSentAtOnceAsAFullSnapshot(t *testing.T) {
	f := newReporter()
	f.scanner.parents = []protocol.Parent{parentA}
	f.tickAt(0)
	if len(f.sent) != 1 {
		t.Fatalf("sent %d snapshots, want 1", len(f.sent))
	}
	s := f.sent[0]
	if s.Hostname != "web-1" || s.Version != "dev" || !s.ScannedAt.Equal(t0) || len(s.Parents) != 1 || s.Parents[0].PID != 100 {
		t.Errorf("snapshot %+v, want web-1, dev, scanned at %v, parent 100", s, t0)
	}
}

func TestAnEmptyScanIsSentAsAnEmptyList(t *testing.T) {
	f := newReporter()
	f.tickAt(0)
	if len(f.sent) != 1 || f.sent[0].Parents == nil {
		t.Errorf("sent %+v, want one snapshot with an empty, non-nil parent list", f.sent)
	}
}

func TestAnUnchangedSetIsSentEveryFifteenSeconds(t *testing.T) {
	f := newReporter()
	f.scanner.parents = []protocol.Parent{parentA}
	f.tickAt(0)
	f.tickAt(5 * time.Second)
	f.tickAt(10 * time.Second)
	if len(f.sent) != 1 {
		t.Fatalf("sent %d snapshots by 10 s, want 1", len(f.sent))
	}
	f.tickAt(15 * time.Second)
	if len(f.sent) != 2 || !f.sent[1].ScannedAt.Equal(t0.Add(15*time.Second)) {
		t.Errorf("sent %d snapshots by 15 s, want 2 (the second at 15 s)", len(f.sent))
	}
}

func TestAChangedSetIsSentAtOnce(t *testing.T) {
	for name, next := range map[string][]protocol.Parent{
		"a parent appears":    {parentA, parentB},
		"a parent disappears": {},
		"a PID is reused":     {{PID: 100, StartTime: 9999, Cmd: "a", Zombies: []int{101}}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newReporter()
			f.scanner.parents = []protocol.Parent{parentA}
			f.tickAt(0)
			f.scanner.parents = next
			f.tickAt(5 * time.Second)
			if len(f.sent) != 2 {
				t.Errorf("sent %d snapshots, want 2", len(f.sent))
			}
		})
	}
}

func TestNewZombiesOfTheSameParentsWaitForTheNextReport(t *testing.T) {
	f := newReporter()
	f.scanner.parents = []protocol.Parent{parentA}
	f.tickAt(0)
	f.scanner.parents = []protocol.Parent{{PID: 100, StartTime: 5000, Cmd: "a", Zombies: []int{101, 102}}}
	f.tickAt(5 * time.Second)
	if len(f.sent) != 1 {
		t.Errorf("sent %d snapshots, want 1: the set of parents did not change", len(f.sent))
	}
}

func TestAFailedSendIsRetriedOnTheNextTick(t *testing.T) {
	f := newReporter()
	f.scanner.parents = []protocol.Parent{parentA}
	f.sendErr = errors.New("connection refused")
	f.tickAt(0)
	f.sendErr = nil
	f.tickAt(5 * time.Second)
	if len(f.sent) != 2 {
		t.Errorf("sent %d snapshots, want 2 (the failed one tried again)", len(f.sent))
	}
	if !strings.Contains(f.logs.String(), "connection refused") {
		t.Errorf("log %q, want the send error", f.logs.String())
	}
}

func TestAFailedScanSendsNothingAndIsLogged(t *testing.T) {
	f := newReporter()
	f.scanner.err = errors.New("open /proc: permission denied")
	f.tickAt(0)
	if len(f.sent) != 0 {
		t.Errorf("sent %d snapshots, want 0", len(f.sent))
	}
	if !strings.Contains(f.logs.String(), "permission denied") {
		t.Errorf("log %q, want the scan error", f.logs.String())
	}
}

func TestRunScansOnEveryTickUntilCancelled(t *testing.T) {
	f := newReporter()
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { f.r.Run(ctx, ticks); close(done) }()
	for i := range 3 {
		select {
		case ticks <- t0.Add(time.Duration(i) * ScanInterval):
		case <-time.After(2 * time.Second):
			t.Fatalf("Run stopped taking ticks after %d", i)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if f.scanner.scans != 3 {
		t.Errorf("%d scans, want 3", f.scanner.scans)
	}
}

func TestAFailedPeriodicSendIsRetriedOnTheNextTick(t *testing.T) {
	f := newReporter()
	f.scanner.parents = []protocol.Parent{parentA}
	f.tickAt(0)
	f.sendErr = errors.New("timeout")
	f.tickAt(15 * time.Second)
	f.sendErr = nil
	f.tickAt(20 * time.Second)
	if len(f.sent) != 3 {
		t.Errorf("sent %d snapshots, want 3 (0 s, the failed 15 s, and again at 20 s)", len(f.sent))
	}
}

func TestTheFifteenSecondReportToleratesLateTicks(t *testing.T) {
	f := newReporter()
	f.scanner.parents = []protocol.Parent{parentA}
	// A real ticker delivers ticks a little late by varying amounts.
	f.r.Tick(context.Background(), t0.Add(30*time.Millisecond))
	f.r.Tick(context.Background(), t0.Add(5*time.Second+10*time.Millisecond))
	f.r.Tick(context.Background(), t0.Add(10*time.Second+2*time.Millisecond))
	f.r.Tick(context.Background(), t0.Add(15*time.Second+1*time.Millisecond))
	if len(f.sent) != 2 {
		t.Errorf("sent %d snapshots by the third tick after the first, want 2", len(f.sent))
	}
}
