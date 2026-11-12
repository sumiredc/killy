package kill

import (
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/proc"
	"github.com/sumiredc/killy/internal/protocol"
)

const (
	agentPID   = 500
	parentPID  = 100
	parentTime = 5000
)

// standardWorld has the agent (500, child of init) and one parent (100) holding two zombies.
func standardWorld(extra ...proc.Stat) *world {
	ps := []proc.Stat{
		{PID: agentPID, Comm: "killy", State: 'S', PPID: 1, StartTime: 900},
		{PID: parentPID, Comm: "parent", State: 'S', PPID: 1, StartTime: parentTime},
		{PID: 101, Comm: "child", State: 'Z', PPID: parentPID, StartTime: 5001},
		{PID: 102, Comm: "child", State: 'Z', PPID: parentPID, StartTime: 5002},
	}
	return newWorld(append(ps, extra...)...)
}

func newKiller(w *world, c *fakeClock) *Killer {
	return &Killer{Proc: &proc.Scanner{Root: w}, Pidfd: worldPidfd{w}, Clock: c, SelfPID: func() int { return agentPID }}
}

func killCmd(ppid int, starttime int64) protocol.KillCommand {
	return protocol.KillCommand{ID: "0f8fad5b-d9cb-469f-a165-70867728950e", PPID: ppid, StartTime: starttime}
}

func assertOutcome(t *testing.T, got, want protocol.Outcome) {
	t.Helper()
	if got != want {
		t.Errorf("outcome = %+v, want %+v", got, want)
	}
}

func assertNoSignal(t *testing.T, w *world) {
	t.Helper()
	if n := w.sendCount(); n != 0 {
		t.Errorf("SendKill called %d times, want 0; events %v", n, w.eventLog())
	}
}

func TestKillRefusesWhenPidfdIsUnavailable(t *testing.T) {
	w := standardWorld()
	w.openErr = ErrPidfdUnavailable
	got := newKiller(w, newFakeClock()).Handle(killCmd(parentPID, parentTime))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.Refused, Reason: protocol.ReasonPidfdUnavailable})
	assertNoSignal(t, w)
}

func TestKillOfAProcessMissingAtPidfdOpenIsAlreadyGone(t *testing.T) {
	w := standardWorld()
	got := newKiller(w, newFakeClock()).Handle(killCmd(777, parentTime))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.AlreadyGone})
	assertNoSignal(t, w)
}

func TestKillIsAlreadyGoneWhenTheStatVanishesAfterPidfdOpen(t *testing.T) {
	w := standardWorld()
	w.afterOpen = func() { w.remove(parentPID) }
	got := newKiller(w, newFakeClock()).Handle(killCmd(parentPID, parentTime))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.AlreadyGone})
	assertNoSignal(t, w)
}

func TestKillRefusesAReusedPID(t *testing.T) {
	w := standardWorld()
	got := newKiller(w, newFakeClock()).Handle(killCmd(parentPID, parentTime-1))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.Refused, Reason: protocol.ReasonPIDReused})
	assertNoSignal(t, w)
}

func TestKillOfAParentAlreadyInStateZIsAlreadyGone(t *testing.T) {
	w := standardWorld()
	w.add(proc.Stat{PID: parentPID, Comm: "parent", State: 'Z', PPID: 1, StartTime: parentTime})
	got := newKiller(w, newFakeClock()).Handle(killCmd(parentPID, parentTime))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.AlreadyGone})
	assertNoSignal(t, w)
}

func TestKillRefusesAParentWithoutZombieChildren(t *testing.T) {
	w := standardWorld(
		proc.Stat{PID: 300, Comm: "busy", State: 'S', PPID: 1, StartTime: 7000},
		proc.Stat{PID: 301, Comm: "worker", State: 'R', PPID: 300, StartTime: 7001},
	)
	got := newKiller(w, newFakeClock()).Handle(killCmd(300, 7000))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.Refused, Reason: protocol.ReasonNotAZombieParent})
	assertNoSignal(t, w)
}

func TestKillRefusesPID1(t *testing.T) {
	w := standardWorld(proc.Stat{PID: 150, Comm: "orphan", State: 'Z', PPID: 1, StartTime: 6000})
	// The agent was started from outside its PID namespace (as with docker exec), so its parent
	// shows as 0 and PID 1 is not among its ancestors: PID 1 has to be refused on its own.
	w.setPPID(agentPID, 0)
	got := newKiller(w, newFakeClock()).Handle(killCmd(1, 1))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.Refused, Reason: protocol.ReasonProtected})
	assertNoSignal(t, w)
}

func TestKillRefusesTheAgentItself(t *testing.T) {
	w := standardWorld(proc.Stat{PID: 501, Comm: "child", State: 'Z', PPID: agentPID, StartTime: 901})
	got := newKiller(w, newFakeClock()).Handle(killCmd(agentPID, 900))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.Refused, Reason: protocol.ReasonProtected})
	assertNoSignal(t, w)
}

func TestKillRefusesAnAncestorOfTheAgent(t *testing.T) {
	// init → 300 (holds a zombie) → 400 → agent
	w := standardWorld(
		proc.Stat{PID: 300, Comm: "sshd", State: 'S', PPID: 1, StartTime: 300},
		proc.Stat{PID: 301, Comm: "child", State: 'Z', PPID: 300, StartTime: 301},
		proc.Stat{PID: 400, Comm: "bash", State: 'S', PPID: 300, StartTime: 400},
	)
	w.setPPID(agentPID, 400)
	got := newKiller(w, newFakeClock()).Handle(killCmd(300, 300))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.Refused, Reason: protocol.ReasonProtected})
	assertNoSignal(t, w)
}

func TestKillReadsTheAgentsAncestorsAgainOnEveryKill(t *testing.T) {
	w := standardWorld(
		proc.Stat{PID: 300, Comm: "a", State: 'S', PPID: 1, StartTime: 7000},
		proc.Stat{PID: 301, Comm: "z", State: 'Z', PPID: 300, StartTime: 7001},
	)
	k := newKiller(w, newFakeClock())
	if got := k.Handle(killCmd(300, 7000)); got.Outcome != protocol.Killed {
		t.Fatalf("first kill = %+v, want killed", got)
	}
	// The agent's parent changes after the first kill (for example it is reparented to a subreaper).
	w.setPPID(agentPID, parentPID)
	got := k.Handle(killCmd(parentPID, parentTime))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.Refused, Reason: protocol.ReasonProtected})
	if n := w.sendCount(); n != 1 {
		t.Errorf("SendKill called %d times, want 1 (the first kill only)", n)
	}
}

func TestKillRefusesWhenTheAgentsAncestryCannotBeRead(t *testing.T) {
	w := standardWorld()
	w.remove(agentPID)
	got := newKiller(w, newFakeClock()).Handle(killCmd(parentPID, parentTime))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.Refused, Reason: protocol.ReasonProtected})
	assertNoSignal(t, w)
}

func TestKillIsAlreadyGoneWhenTheTargetExitsBeforeTheSignal(t *testing.T) {
	w := standardWorld()
	reused := proc.Stat{PID: parentPID, Comm: "newcomer", State: 'S', PPID: 1, StartTime: 8000}
	// After the checks pass, the parent exits and is reaped, and its PID goes to a new process.
	w.beforeSend = func() {
		w.removeZombiesOf(parentPID)
		w.remove(parentPID)
		w.add(reused)
	}
	got := newKiller(w, newFakeClock()).Handle(killCmd(parentPID, parentTime))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.AlreadyGone})
	if s := w.signalledProcesses(); len(s) != 0 {
		t.Errorf("signalled %v, want nobody (the new process with PID %d must not get SIGKILL)", s, parentPID)
	}
	if n := w.sendCount(); n != 1 {
		t.Errorf("SendKill called %d times, want 1", n)
	}
}

func TestKillReportsSignalFailureWithTheErrnoName(t *testing.T) {
	w := standardWorld()
	w.sendErr = syscall.EPERM
	got := newKiller(w, newFakeClock()).Handle(killCmd(parentPID, parentTime))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.SignalFailed, Reason: "EPERM"})
}

func TestKillReturnsKilledWithTheNumberOfZombiesReaped(t *testing.T) {
	w := standardWorld()
	got := newKiller(w, newFakeClock()).Handle(killCmd(parentPID, parentTime))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.Killed, Reaped: 2})
	if s, want := w.signalledProcesses(), []identity{{parentPID, parentTime}}; !slices.Equal(s, want) {
		t.Errorf("signalled %v, want %v", s, want)
	}
}

func TestKillOpensThePidfdBeforeCheckingAndSignalsThroughItAfter(t *testing.T) {
	w := standardWorld()
	newKiller(w, newFakeClock()).Handle(killCmd(parentPID, parentTime))
	events := w.eventLog()
	open := slices.Index(events, "pidfd_open 100")
	read := slices.Index(events, "read 100/stat")
	send := slices.Index(events, "send_kill 100")
	if open < 0 || read < 0 || send < 0 {
		t.Fatalf("events %v: want pidfd_open 100, read 100/stat and send_kill 100", events)
	}
	if !(open < read && read < send) {
		t.Errorf("events %v: want the first read of 100/stat after pidfd_open and before send_kill", events)
	}
}

func TestKillWaitsUpToFiveSecondsForTheZombiesToBeReaped(t *testing.T) {
	w := standardWorld()
	w.keepZombies = true
	c := newFakeClock()
	c.onSleep = func(elapsed time.Duration) {
		if elapsed >= 4900*time.Millisecond {
			w.removeZombiesOf(1)
		}
	}
	got := newKiller(w, c).Handle(killCmd(parentPID, parentTime))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.Killed, Reaped: 2})
}

func TestKillReportsZombiesRemainAfterFiveSeconds(t *testing.T) {
	w := standardWorld()
	w.keepZombies = true
	c := newFakeClock()
	got := newKiller(w, c).Handle(killCmd(parentPID, parentTime))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.ZombiesRemain})
	// Not longer either: the Killy server answers the phone with unknown after 10 s.
	if waited := c.Now().Sub(c.start); waited != 5*time.Second {
		t.Errorf("gave up after %v, want 5s", waited)
	}
}

func TestKillCountsAZombieWhosePIDWasReusedAsReaped(t *testing.T) {
	w := standardWorld()
	w.keepZombies = true
	c := newFakeClock()
	c.onSleep = func(time.Duration) {
		w.removeZombiesOf(1)
		// PID 101 now belongs to another zombie with a different start time.
		w.add(proc.Stat{PID: 101, Comm: "other", State: 'Z', PPID: 600, StartTime: 9999})
	}
	got := newKiller(w, c).Handle(killCmd(parentPID, parentTime))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.Killed, Reaped: 2})
}

func TestKillsOfTheSameParentAtOnceSignalOnce(t *testing.T) {
	w := standardWorld()
	var once sync.Once
	var arrived atomic.Int32
	// Hold the first signal until the second kill has also passed the checks and is about to
	// signal, or 300 ms have gone by. Without one-at-a-time handling the second kill gets there,
	// having checked the parent while it was still alive, and signals it too.
	w.beforeSend = func() {
		arrived.Add(1)
		once.Do(func() {
			for deadline := time.Now().Add(300 * time.Millisecond); arrived.Load() < 2 && time.Now().Before(deadline); {
				time.Sleep(time.Millisecond)
			}
		})
	}
	k := newKiller(w, newFakeClock())
	outcomes := make([]protocol.Outcome, 2)
	var wg sync.WaitGroup
	for i, id := range []string{"0f8fad5b-d9cb-469f-a165-70867728950e", "7c9e6679-7425-40de-944b-e07fc1f90ae7"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcomes[i] = k.Handle(protocol.KillCommand{ID: id, PPID: parentPID, StartTime: parentTime})
		}()
	}
	wg.Wait()
	killed := protocol.Outcome{Outcome: protocol.Killed, Reaped: 2}
	gone := protocol.Outcome{Outcome: protocol.AlreadyGone}
	if !(outcomes[0] == killed && outcomes[1] == gone) && !(outcomes[0] == gone && outcomes[1] == killed) {
		t.Errorf("outcomes = %+v, want one killed and one already_gone", outcomes)
	}
	if n := len(w.signalledProcesses()); n != 1 {
		t.Errorf("signalled %d times, want 1", n)
	}
}

func TestKillOfAnImpossiblePIDIsAlreadyGoneWithoutOpeningAPidfd(t *testing.T) {
	// pidfd_open takes a 32-bit PID, so 1<<32|100 would open PID 100.
	for _, ppid := range []int{0, -1, 1<<32 | parentPID} {
		t.Run(strconv.Itoa(ppid), func(t *testing.T) {
			w := standardWorld()
			got := newKiller(w, newFakeClock()).Handle(killCmd(ppid, parentTime))
			assertOutcome(t, got, protocol.Outcome{Outcome: protocol.AlreadyGone})
			if events := w.eventLog(); slices.ContainsFunc(events, func(e string) bool { return strings.HasPrefix(e, "pidfd_open") }) {
				t.Errorf("events %v: want no pidfd_open", events)
			}
		})
	}
}

func TestKillReportsAnUnexpectedPidfdOpenErrorAsSignalFailed(t *testing.T) {
	w := standardWorld()
	w.openErr = syscall.EMFILE
	got := newKiller(w, newFakeClock()).Handle(killCmd(parentPID, parentTime))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.SignalFailed, Reason: "EMFILE"})
	assertNoSignal(t, w)
}

func TestKillReportsATargetThatCannotBeReadAsSignalFailed(t *testing.T) {
	w := standardWorld()
	w.failStat(parentPID, syscall.EACCES)
	got := newKiller(w, newFakeClock()).Handle(killCmd(parentPID, parentTime))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.SignalFailed, Reason: "EACCES"})
	assertNoSignal(t, w)
}

func TestKillCountsAZombieThatCannotBeReadAsLeft(t *testing.T) {
	w := standardWorld()
	w.beforeSend = func() { w.failStat(101, syscall.EIO) }
	got := newKiller(w, newFakeClock()).Handle(killCmd(parentPID, parentTime))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.ZombiesRemain})
}

func TestKillClosesThePidfd(t *testing.T) {
	cases := map[string]protocol.KillCommand{
		"killed":  killCmd(parentPID, parentTime),
		"refused": killCmd(parentPID, parentTime-1),
	}
	for name, cmd := range cases {
		t.Run(name, func(t *testing.T) {
			w := standardWorld()
			newKiller(w, newFakeClock()).Handle(cmd)
			if events := w.eventLog(); !slices.Contains(events, "close 100") {
				t.Errorf("events %v: want close 100", events)
			}
		})
	}
}
