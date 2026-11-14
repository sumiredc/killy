package command

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/clock"
	"github.com/sumiredc/killy/internal/protocol"
)

var t0 = time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)

const (
	agentA = "0f8fad5b-d9cb-469f-a165-70867728950e"
	agentB = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	killID = "16fd2706-8baf-433b-82eb-8c7fada847da"
)

type fixture struct {
	clock *clock.Fake
	timer chan time.Time
	// timers counts the timers the router started, one per command sent.
	timers atomic.Int32
	r      *Router
}

func newFixture() *fixture {
	f := &fixture{clock: clock.NewFake(t0), timer: make(chan time.Time, 8)}
	f.r = New(f.clock, func(time.Duration) <-chan time.Time { f.timers.Add(1); return f.timer })
	return f
}

var cmd = protocol.KillCommand{ID: killID, PPID: 100, StartTime: 5000}

// killAsync starts a kill and returns where its outcome will arrive.
func (f *fixture) killAsync(agentID string) <-chan protocol.Outcome {
	out := make(chan protocol.Outcome, 1)
	go func() { out <- f.r.RelayKill(context.Background(), agentID, cmd) }()
	return out
}

func nextCommand(t *testing.T, cmds <-chan Command) Command {
	t.Helper()
	select {
	case c, ok := <-cmds:
		if !ok {
			t.Fatal("the command stream was closed")
		}
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("no command within 2 s")
		return Command{}
	}
}

func noCommand(t *testing.T, cmds <-chan Command) {
	t.Helper()
	select {
	case c := <-cmds:
		t.Errorf("got command %+v, want none", c)
	case <-time.After(100 * time.Millisecond):
	}
}

func outcomeOf(t *testing.T, out <-chan protocol.Outcome) protocol.Outcome {
	t.Helper()
	select {
	case o := <-out:
		return o
	case <-time.After(2 * time.Second):
		t.Fatal("no outcome within 2 s")
		return protocol.Outcome{}
	}
}

var killed = protocol.CommandResult{ID: killID, Type: protocol.ResultKill, Outcome: protocol.Killed, Reaped: 2}

func TestAKillIsSentToTheAgentAndReturnsItsOutcome(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	out := f.killAsync(agentA)
	c := nextCommand(t, cmds)
	if c.Event != "kill" || c.Data != cmd {
		t.Fatalf("command %+v, want kill %+v", c, cmd)
	}
	if !f.r.Deliver(agentA, killed) {
		t.Fatal("Deliver = false for the waiting kill")
	}
	if o := outcomeOf(t, out); o != (protocol.Outcome{Outcome: protocol.Killed, Reaped: 2}) {
		t.Errorf("outcome %+v, want killed with 2 reaped", o)
	}
}

func TestTheAgentsOutcomeIsPassedOnUnchanged(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	out := f.killAsync(agentA)
	nextCommand(t, cmds)
	f.r.Deliver(agentA, protocol.CommandResult{ID: killID, Type: protocol.ResultKill, Outcome: protocol.SignalFailed, Reason: "EPERM"})
	if o := outcomeOf(t, out); o != (protocol.Outcome{Outcome: protocol.SignalFailed, Reason: "EPERM"}) {
		t.Errorf("outcome %+v, want signal_failed EPERM", o)
	}
}

func TestAKillWithoutAStreamIsUnknownAgentDisconnected(t *testing.T) {
	f := newFixture()
	o := f.r.RelayKill(context.Background(), agentA, cmd)
	if o != (protocol.Outcome{Outcome: protocol.Unknown, Reason: protocol.ReasonAgentDisconnected}) {
		t.Errorf("outcome %+v, want unknown agent_disconnected", o)
	}
}

func TestAKillWithoutAnOutcomeIn10SecondsIsUnknownTimeout(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	out := f.killAsync(agentA)
	nextCommand(t, cmds)
	f.clock.Advance(KillTimeout)
	f.timer <- f.clock.Now()
	if o := outcomeOf(t, out); o != (protocol.Outcome{Outcome: protocol.Unknown, Reason: protocol.ReasonTimeout}) {
		t.Errorf("outcome %+v, want unknown timeout", o)
	}
}

func TestAnOutcomeJustBefore10SecondsIsUsed(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	out := f.killAsync(agentA)
	nextCommand(t, cmds)
	f.clock.Advance(KillTimeout - time.Nanosecond)
	if !f.r.Deliver(agentA, killed) {
		t.Error("Deliver just before 10 s = false")
	}
	if o := outcomeOf(t, out); o.Outcome != protocol.Killed {
		t.Errorf("outcome %+v, want killed", o)
	}
}

func TestAnOutcomeAtExactly10SecondsIsTooLate(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	out := f.killAsync(agentA)
	nextCommand(t, cmds)
	f.clock.Advance(KillTimeout)
	if f.r.Deliver(agentA, killed) {
		t.Error("Deliver at exactly 10 s = true, want too late")
	}
	f.timer <- f.clock.Now()
	if o := outcomeOf(t, out); o.Outcome != protocol.Unknown || o.Reason != protocol.ReasonTimeout {
		t.Errorf("outcome %+v, want unknown timeout", o)
	}
}

func TestAnOutcomeFromAnotherAgentIsNotUsed(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	_, detachB := f.r.Attach(agentB)
	defer detachB()
	out := f.killAsync(agentA)
	nextCommand(t, cmds)
	if f.r.Deliver(agentB, killed) {
		t.Error("Deliver from agent B = true for agent A's kill")
	}
	select {
	case o := <-out:
		t.Fatalf("the kill ended with %+v from the wrong agent", o)
	case <-time.After(100 * time.Millisecond):
	}
	f.r.Deliver(agentA, protocol.CommandResult{ID: killID, Type: protocol.ResultKill, Outcome: protocol.AlreadyGone})
	if o := outcomeOf(t, out); o.Outcome != protocol.AlreadyGone {
		t.Errorf("outcome %+v, want already_gone from agent A", o)
	}
}

func TestAnOutcomeNobodyWaitsForIsRefused(t *testing.T) {
	f := newFixture()
	if f.r.Deliver(agentA, killed) {
		t.Error("Deliver with nothing waiting = true")
	}
}

func TestTwoKillsWithTheSameIDAtOnceSendOneCommand(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	first := f.killAsync(agentA)
	nextCommand(t, cmds)
	second := f.killAsync(agentA)
	noCommand(t, cmds)
	f.r.Deliver(agentA, killed)
	a, b := outcomeOf(t, first), outcomeOf(t, second)
	if a != b || a.Outcome != protocol.Killed {
		t.Errorf("outcomes %+v and %+v, want killed twice", a, b)
	}
}

func TestTheSameIDForAnotherAgentIsAnotherCommand(t *testing.T) {
	f := newFixture()
	cmdsA, detachA := f.r.Attach(agentA)
	defer detachA()
	cmdsB, detachB := f.r.Attach(agentB)
	defer detachB()
	f.killAsync(agentA)
	f.killAsync(agentB)
	nextCommand(t, cmdsA)
	nextCommand(t, cmdsB)
}

func TestTheSameIDAfterTheFirstEndedIsSentAgain(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	out := f.killAsync(agentA)
	nextCommand(t, cmds)
	f.r.Deliver(agentA, killed)
	outcomeOf(t, out)
	// The resend goes to the agent, which answers from its own record of the ID.
	f.killAsync(agentA)
	nextCommand(t, cmds)
}

func TestACallerThatGivesUpLeavesTheKillWaitingForAResend(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	ctx, cancel := context.WithCancel(context.Background())
	gaveUp := make(chan protocol.Outcome, 1)
	go func() { gaveUp <- f.r.RelayKill(ctx, agentA, cmd) }()
	nextCommand(t, cmds)
	cancel()
	outcomeOf(t, gaveUp)
	resend := f.killAsync(agentA)
	noCommand(t, cmds)
	f.r.Deliver(agentA, killed)
	if o := outcomeOf(t, resend); o.Outcome != protocol.Killed {
		t.Errorf("resend got %+v, want the first command's outcome", o)
	}
}

func TestANewerStreamReplacesTheOlderOne(t *testing.T) {
	f := newFixture()
	old, detachOld := f.r.Attach(agentA)
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	select {
	case _, ok := <-old:
		if ok {
			t.Error("the old stream got a command, want it closed")
		}
	case <-time.After(time.Second):
		t.Error("the old stream was not closed")
	}
	detachOld() // must not detach the newer stream
	f.killAsync(agentA)
	nextCommand(t, cmds)
}

func TestADetachedAgentIsDisconnected(t *testing.T) {
	f := newFixture()
	_, detach := f.r.Attach(agentA)
	detach()
	if o := f.r.RelayKill(context.Background(), agentA, cmd); o.Reason != protocol.ReasonAgentDisconnected {
		t.Errorf("outcome %+v, want unknown agent_disconnected", o)
	}
}

func TestManyKillsAtOnceDoNotRace(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); f.r.RelayKill(context.Background(), agentA, cmd) }()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	// A kill that starts after the previous one ended sends its own command; answer each.
	for {
		select {
		case <-done:
			return
		case <-cmds:
			f.r.Deliver(agentA, killed)
		case <-time.After(5 * time.Second):
			t.Fatal("the kills did not finish")
		}
	}
}

func TestTheKillTimeoutIs10Seconds(t *testing.T) {
	if KillTimeout != 10*time.Second {
		t.Errorf("KillTimeout = %v, want 10s (docs/ARCHITECTURE.md, Timing)", KillTimeout)
	}
}

func TestAResendWhileTheStreamReconnectsJoinsTheWaitingKill(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	first := f.killAsync(agentA)
	nextCommand(t, cmds)
	// The agent's stream drops after it received the kill and before it answered.
	detach()
	resend := f.killAsync(agentA)
	time.Sleep(50 * time.Millisecond)
	f.r.Deliver(agentA, killed)
	if o := outcomeOf(t, resend); o.Outcome != protocol.Killed {
		t.Errorf("resend got %+v, want the agent's killed (not agent_disconnected)", o)
	}
	outcomeOf(t, first)
}
