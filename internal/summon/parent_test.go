package summon

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestAParentStartsWithTwoToFourZombiesAndAddsOneEachTickUpToSix(t *testing.T) {
	for pick, first := range map[int]int{0: 2, 1: 3, 2: 4} {
		spawned := 0
		var asked []int
		p := &parent{
			Spawn: func() error { spawned++; return nil },
			Pick:  func(n int) int { asked = append(asked, n); return pick },
			Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		ticks := make(chan time.Time)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { p.Run(ctx, ticks); close(done) }()
		// More ticks than needed: each unbuffered send waits until Run took the one before.
		for range MaxZombies + 2 {
			ticks <- time.Now()
		}
		cancel()
		<-done
		if len(asked) != 1 || asked[0] != ExtraFirstZombies {
			t.Errorf("picked among %v, want once among %d", asked, ExtraFirstZombies)
		}
		if spawned != MaxZombies {
			t.Errorf("starting with %d: %d zombies after many ticks, want %d", first, spawned, MaxZombies)
		}
	}
}

func TestAParentCountsOnlyTheZombiesItCouldStart(t *testing.T) {
	spawned, calls := 0, 0
	p := &parent{
		Spawn: func() error {
			calls++
			if calls == 1 {
				return errors.New("fork failed")
			}
			spawned++
			return nil
		},
		Pick: func(int) int { return 0 },
		Log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx, ticks); close(done) }()
	for range 10 {
		ticks <- time.Now()
	}
	cancel()
	<-done
	if spawned != MaxZombies || calls != MaxZombies+1 {
		t.Errorf("%d zombies from %d tries, want %d from %d", spawned, calls, MaxZombies, MaxZombies+1)
	}
}

func TestAParentStartsWithThePickedNumberOfZombiesBeforeAnyTick(t *testing.T) {
	for pick, first := range map[int]int{0: 2, 1: 3, 2: 4} {
		spawned := make(chan struct{}, 10)
		p := &parent{
			Spawn: func() error { spawned <- struct{}{}; return nil },
			Pick:  func(int) int { return pick },
			Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		ctx, cancel := context.WithCancel(context.Background())
		go p.Run(ctx, make(chan time.Time))
		for i := range first {
			select {
			case <-spawned:
			case <-time.After(2 * time.Second):
				t.Fatalf("pick %d: %d zombies without a tick, want %d", pick, i, first)
			}
		}
		select {
		case <-spawned:
			t.Errorf("pick %d: more than %d zombies without a tick", pick, first)
		case <-time.After(50 * time.Millisecond):
		}
		cancel()
	}
}
