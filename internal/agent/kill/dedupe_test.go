package kill

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/protocol"
)

func newKillerWithStore(w *world, store OutcomeStore) *Killer {
	k := newKiller(w, newFakeClock())
	k.Store = store
	return k
}

// assertTouchedNothing fails when anything reached a process or /proc after the events in before.
func assertTouchedNothing(t *testing.T, w *world, before int) {
	t.Helper()
	if after := w.eventLog()[before:]; len(after) != 0 {
		t.Errorf("the repeated kill reached processes: %v; want no pidfd_open and no /proc read", after)
	}
}

func TestTheSameIDAgainReturnsTheFirstOutcomeWithoutTouchingAnyProcess(t *testing.T) {
	w := standardWorld()
	k := newKillerWithStore(w, openStore(t, storePath(t)))
	cmd := killCmd(parentPID, parentTime)
	first := handle(t, k, cmd)
	before := len(w.eventLog())

	again := handle(t, k, cmd)
	assertOutcome(t, again, first)
	assertTouchedNothing(t, w, before)
}

func TestTheSameIDAfterARestartReturnsTheFirstOutcomeWithoutTouchingAnyProcess(t *testing.T) {
	w := standardWorld()
	path := storePath(t)
	store := openStore(t, path)
	cmd := killCmd(parentPID, parentTime)
	first := handle(t, newKillerWithStore(w, store), cmd)
	_ = store.Close()
	before := len(w.eventLog())

	restarted := newKillerWithStore(w, openStore(t, path))
	again := handle(t, restarted, cmd)
	assertOutcome(t, again, first)
	assertTouchedNothing(t, w, before)
}

// countingStore counts lookups, so a test can tell when a kill has got past waiting for its ID.
type countingStore struct {
	OutcomeStore
	lookups atomic.Int32
}

func (s *countingStore) Lookup(id string) (protocol.Outcome, bool) {
	s.lookups.Add(1)
	return s.OutcomeStore.Lookup(id)
}

func TestTheSameIDTwiceAtOnceIsCheckedAndSignalledOnce(t *testing.T) {
	w := standardWorld()
	store := &countingStore{OutcomeStore: openStore(t, storePath(t))}
	k := newKillerWithStore(w, store)
	var once sync.Once
	// Hold the first signal until the second request has looked its ID up, or 300 ms have gone by.
	// Without waiting on the ID, the second finds no record yet, runs its own checks after the
	// first kill and ends already_gone instead of sharing the first outcome.
	w.beforeSend = func() {
		once.Do(func() {
			for deadline := time.Now().Add(300 * time.Millisecond); store.lookups.Load() < 2 && time.Now().Before(deadline); {
				time.Sleep(time.Millisecond)
			}
		})
	}
	cmd := killCmd(parentPID, parentTime)
	outcomes := make([]protocol.Outcome, 2)
	var wg sync.WaitGroup
	for i := range outcomes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcomes[i] = handle(t, k, cmd)
		}()
	}
	wg.Wait()

	if outcomes[0] != outcomes[1] {
		t.Errorf("outcomes = %+v, want the same outcome twice", outcomes)
	}
	events := w.eventLog()
	if n := countPrefix(events, "pidfd_open"); n != 1 {
		t.Errorf("pidfd_open called %d times, want 1; events %v", n, events)
	}
	if n := countPrefix(events, "send_kill"); n != 1 {
		t.Errorf("SendKill called %d times, want 1", n)
	}
}

func countPrefix(events []string, prefix string) int {
	n := 0
	for _, e := range events {
		if strings.HasPrefix(e, prefix) {
			n++
		}
	}
	return n
}

func TestAKillWhoseIDIsNotAUUIDv4IsIgnored(t *testing.T) {
	for _, id := range []string{
		"c232ab00-9414-11ec-b3c8-9f6bdeced846", // v1
		"0f8fad5b-d9cb-469f-c165-70867728950e", // variant 110
		"0F8FAD5B-D9CB-469F-A165-70867728950E", // upper case
		"kill-100",
	} {
		t.Run(id, func(t *testing.T) {
			w := standardWorld()
			store := newMemStore()
			k := newKillerWithStore(w, store)
			_, err := k.Handle(protocol.KillCommand{ID: id, PPID: parentPID, StartTime: parentTime})
			if !errors.Is(err, ErrInvalidKillID) {
				t.Errorf("err = %v, want ErrInvalidKillID", err)
			}
			if events := w.eventLog(); len(events) != 0 {
				t.Errorf("events %v, want none", events)
			}
			assertNoRecord(t, store, id)
		})
	}
}

func TestAnOutcomeThatCannotBeStoredIsReturnedLoggedAndNotStoredAgain(t *testing.T) {
	w := standardWorld()
	store := openStore(t, storePath(t))
	file := &failingFile{}
	store.file = file
	var logs bytes.Buffer
	k := newKillerWithStore(w, store)
	k.Log = slog.New(slog.NewTextHandler(&logs, nil))
	cmd := killCmd(parentPID, parentTime)

	first := handle(t, k, cmd)
	assertOutcome(t, first, protocol.Outcome{Outcome: protocol.Killed, Reaped: 2})
	if !strings.Contains(logs.String(), cmd.ID) {
		t.Errorf("log %q does not mention the kill ID %s", logs.String(), cmd.ID)
	}

	before := len(w.eventLog())
	again := handle(t, k, cmd)
	assertOutcome(t, again, first)
	assertTouchedNothing(t, w, before)
	if file.writes != 1 {
		t.Errorf("tried to write %d times, want 1", file.writes)
	}
}

func TestDifferentIDsForTheSameParentAreDifferentKills(t *testing.T) {
	w := standardWorld()
	k := newKillerWithStore(w, openStore(t, storePath(t)))
	first := handle(t, k, killCmd(parentPID, parentTime))
	second := handle(t, k, killCmd(parentPID, parentTime))
	assertOutcome(t, first, protocol.Outcome{Outcome: protocol.Killed, Reaped: 2})
	assertOutcome(t, second, protocol.Outcome{Outcome: protocol.AlreadyGone})
	if n := countPrefix(w.eventLog(), "pidfd_open"); n != 2 {
		t.Errorf("pidfd_open called %d times, want 2", n)
	}
}
