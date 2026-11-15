package summon

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"
)

// fakeParent is a started parent that exits when the test ends it.
type fakeParent struct {
	argv []string
	done chan struct{}
}

func (p *fakeParent) Wait() error { <-p.done; return nil }

func (p *fakeParent) exit() { close(p.done) }

type fakeStarts struct {
	mu      sync.Mutex
	started []*fakeParent
	fail    error
}

func (f *fakeStarts) start(argv []string) (Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return nil, f.fail
	}
	p := &fakeParent{argv: argv, done: make(chan struct{})}
	f.started = append(f.started, p)
	return p, nil
}

func (f *fakeStarts) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.started)
}

func (f *fakeStarts) get(i int) *fakeParent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.started[i]
}

func newSummoner(f *fakeStarts, pick func(int) int) *Summoner {
	return &Summoner{Start: f.start, Pick: pick, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

// waitAlive waits until the summoner has collected exited parents down to n alive.
func waitAlive(t *testing.T, s *Summoner, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for s.Alive() != n {
		if time.Now().After(deadline) {
			t.Fatalf("%d parents alive, want %d", s.Alive(), n)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestEachTickStartsAParentUntilThreeAreAlive(t *testing.T) {
	f := &fakeStarts{}
	s := newSummoner(f, func(int) int { return 0 })
	for i := 1; i <= MaxParents; i++ {
		s.Tick()
		if f.count() != i || s.Alive() != i {
			t.Fatalf("after %d ticks: %d started, %d alive; want %d", i, f.count(), s.Alive(), i)
		}
	}
	s.Tick()
	s.Tick()
	if f.count() != MaxParents {
		t.Errorf("%d started with %d alive, want no more than %d", f.count(), MaxParents, MaxParents)
	}
}

func TestAParentThatExitsIsReplacedOnTheNextTick(t *testing.T) {
	f := &fakeStarts{}
	s := newSummoner(f, func(int) int { return 0 })
	for range MaxParents {
		s.Tick()
	}
	f.get(1).exit()
	waitAlive(t, s, MaxParents-1)
	if f.count() != MaxParents {
		t.Fatalf("%d started before the next tick, want %d: a new parent waits for the interval", f.count(), MaxParents)
	}
	s.Tick()
	if f.count() != MaxParents+1 || s.Alive() != MaxParents {
		t.Errorf("%d started, %d alive; want %d and %d", f.count(), s.Alive(), MaxParents+1, MaxParents)
	}
}

func TestTheCommandLineIsPickedFromTheCandidates(t *testing.T) {
	if len(Commands) < 2 {
		t.Fatalf("%d candidates, want at least 2", len(Commands))
	}
	f := &fakeStarts{}
	var ns []int
	picks := []int{1, 0, len(Commands) - 1}
	s := newSummoner(f, func(n int) int {
		ns = append(ns, n)
		p := picks[0]
		picks = picks[1:]
		return p
	})
	for range 3 {
		s.Tick()
	}
	want := [][]string{Commands[1], Commands[0], Commands[len(Commands)-1]}
	for i, w := range want {
		if got := f.get(i).argv; !reflect.DeepEqual(got, w) {
			t.Errorf("parent %d shows %q, want %q", i, got, w)
		}
	}
	for _, n := range ns {
		if n != len(Commands) {
			t.Errorf("picked among %d, want all %d candidates", n, len(Commands))
		}
	}
}

func TestAParentThatCannotBeStartedIsTriedAgainOnTheNextTick(t *testing.T) {
	f := &fakeStarts{fail: errors.New("fork: resource temporarily unavailable")}
	s := newSummoner(f, func(int) int { return 0 })
	s.Tick()
	if s.Alive() != 0 {
		t.Fatalf("%d alive after a failed start, want 0", s.Alive())
	}
	f.mu.Lock()
	f.fail = nil
	f.mu.Unlock()
	s.Tick()
	if f.count() != 1 || s.Alive() != 1 {
		t.Errorf("%d started, %d alive; want 1 and 1", f.count(), s.Alive())
	}
}

func TestRunTicksForEachValueUntilCancelled(t *testing.T) {
	f := &fakeStarts{}
	s := newSummoner(f, func(int) int { return 0 })
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx, ticks); close(done) }()
	ticks <- time.Now()
	ticks <- time.Now()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if f.count() != 2 {
		t.Errorf("%d started for 2 ticks, want 2", f.count())
	}
}
