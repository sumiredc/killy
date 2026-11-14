package command

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/protocol"
)

// checkAsync starts a check and returns where its result will arrive.
func (f *fixture) checkAsync(ctx context.Context, agentID string) <-chan CheckResult {
	out := make(chan CheckResult, 1)
	go func() { out <- f.r.Check(ctx, agentID) }()
	return out
}

// checkNow runs a check and waits up to 2 s for it, so a check that blocks fails the test instead
// of hanging it.
func (f *fixture) checkNow(t *testing.T, agentID string) CheckResult {
	t.Helper()
	return resultOf(t, f.checkAsync(background, agentID))
}

func resultOf(t *testing.T, out <-chan CheckResult) CheckResult {
	t.Helper()
	select {
	case r := <-out:
		return r
	case <-time.After(2 * time.Second):
		t.Fatal("no check result within 2 s")
		return CheckResult{}
	}
}

func stillWaiting[T any](t *testing.T, out <-chan T) {
	t.Helper()
	select {
	case r := <-out:
		t.Fatalf("ended with %+v, want it still waiting", r)
	case <-time.After(100 * time.Millisecond):
	}
}

// nextPing reads the next command, which must be a ping, and returns its ID.
func nextPing(t *testing.T, cmds <-chan Command) string {
	t.Helper()
	c := nextCommand(t, cmds)
	p, ok := c.Data.(protocol.PingCommand)
	if c.Event != "ping" || !ok {
		t.Fatalf("command %+v, want a ping", c)
	}
	return p.ID
}

func pong(id, version string) protocol.CommandResult {
	return protocol.CommandResult{ID: id, Type: protocol.ResultPong, Version: version}
}

var (
	ok123          = CheckResult{Result: protocol.CheckOK, Version: "1.2.3"}
	noResponse     = CheckResult{Result: protocol.CheckNoResponse}
	agentGone      = CheckResult{Result: protocol.CheckAgentDisconnected}
	background     = context.Background()
	pingTimerFired = func(f *fixture) { f.timer <- f.clock.Now() }
)

func TestACheckSendsAPingAndAPongMakesItOkWithTheAgentVersion(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	out := f.checkAsync(background, agentA)
	id := nextPing(t, cmds)
	if !protocol.IsUUIDv4(id) {
		t.Errorf("ping id %q, want a UUID v4 made by the Killy server", id)
	}
	if !f.r.Deliver(agentA, pong(id, "1.2.3")) {
		t.Fatal("Deliver(pong) = false for the waiting ping")
	}
	if r := resultOf(t, out); r != ok123 {
		t.Errorf("result %+v, want ok with version 1.2.3", r)
	}
}

func TestThePingEventCarriesOnlyItsID(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	f.checkAsync(background, agentA)
	c := nextCommand(t, cmds)
	b, err := json.Marshal(c.Data)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	_ = json.Unmarshal(b, &fields)
	if _, ok := fields["id"]; !ok || len(fields) != 1 {
		t.Errorf("ping data %s, want {\"id\": …} only (PingCommand)", b)
	}
}

func TestACheckWithoutAStreamIsAgentDisconnectedAndSendsNothing(t *testing.T) {
	f := newFixture()
	if r := f.checkNow(t, agentA); r != agentGone {
		t.Errorf("result %+v, want agent_disconnected", r)
	}
	if n := f.timers.Load(); n != 0 {
		t.Errorf("%d pings started waiting, want none", n)
	}
}

func TestAPongJustBefore5SecondsIsOk(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	out := f.checkAsync(background, agentA)
	id := nextPing(t, cmds)
	f.clock.Advance(PingTimeout - time.Millisecond)
	if !f.r.Deliver(agentA, pong(id, "1.2.3")) {
		t.Error("Deliver(pong) at 4.999 s = false")
	}
	if r := resultOf(t, out); r != ok123 {
		t.Errorf("result %+v, want ok", r)
	}
}

func TestAPongAtExactly5SecondsIsTooLateAndTheCheckIsNoResponse(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	out := f.checkAsync(background, agentA)
	id := nextPing(t, cmds)
	f.clock.Advance(PingTimeout)
	if f.r.Deliver(agentA, pong(id, "1.2.3")) {
		t.Error("Deliver(pong) at exactly 5 s = true, want too late")
	}
	pingTimerFired(f)
	if r := resultOf(t, out); r != noResponse {
		t.Errorf("result %+v, want no_response", r)
	}
}

func TestAPongAfter5SecondsIsTooLate(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	out := f.checkAsync(background, agentA)
	id := nextPing(t, cmds)
	f.clock.Advance(PingTimeout + time.Second)
	if f.r.Deliver(agentA, pong(id, "1.2.3")) {
		t.Error("Deliver(pong) at 6 s = true, want too late")
	}
	pingTimerFired(f)
	if r := resultOf(t, out); r != noResponse {
		t.Errorf("result %+v, want no_response", r)
	}
}

func TestACheckWithNoPongIsNoResponseWhenItsTimeIsUp(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	out := f.checkAsync(background, agentA)
	nextPing(t, cmds)
	stillWaiting(t, out)
	f.clock.Advance(PingTimeout)
	pingTimerFired(f)
	if r := resultOf(t, out); r != noResponse {
		t.Errorf("result %+v, want no_response", r)
	}
}

func TestACheckWithin2SecondsOfARunningCheckSendsNoPingAndGetsItsResult(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	first := f.checkAsync(background, agentA)
	id := nextPing(t, cmds)
	f.clock.Advance(CheckInterval)
	second := f.checkAsync(background, agentA)
	noCommand(t, cmds)
	stillWaiting(t, second)
	f.r.Deliver(agentA, pong(id, "1.2.3"))
	if a, b := resultOf(t, first), resultOf(t, second); a != ok123 || b != ok123 {
		t.Errorf("results %+v and %+v, want ok twice", a, b)
	}
}

func TestACheckWithin2SecondsOfAFinishedCheckSendsNoPingAndGetsItsResult(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	first := f.checkAsync(background, agentA)
	f.r.Deliver(agentA, pong(nextPing(t, cmds), "1.2.3"))
	resultOf(t, first)
	for _, at := range []time.Duration{time.Second, time.Second} { // 1 s, then exactly 2 s
		f.clock.Advance(at)
		if r := f.checkNow(t, agentA); r != ok123 {
			t.Errorf("check %v later: %+v, want the last result ok", at, r)
		}
	}
	noCommand(t, cmds)
}

func TestACheckJustOver2SecondsLaterSendsANewPingEvenWhileThePreviousOneRuns(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	first := f.checkAsync(background, agentA)
	firstID := nextPing(t, cmds)
	f.clock.Advance(CheckInterval + time.Nanosecond)
	second := f.checkAsync(background, agentA)
	secondID := nextPing(t, cmds)
	if secondID == firstID {
		t.Fatalf("the new ping reused the id %s", firstID)
	}
	f.r.Deliver(agentA, pong(secondID, "2.0.0"))
	if r := resultOf(t, second); r != (CheckResult{Result: protocol.CheckOK, Version: "2.0.0"}) {
		t.Errorf("second check %+v, want ok 2.0.0 from its own ping", r)
	}
	stillWaiting(t, first)
}

func TestTheTwoSecondsCountFromTheLastCheckThatWentToTheAgent(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	first := f.checkAsync(background, agentA)
	f.r.Deliver(agentA, pong(nextPing(t, cmds), "1.2.3"))
	resultOf(t, first)
	f.clock.Advance(1500 * time.Millisecond)
	f.checkNow(t, agentA)        // answered from the first check
	f.clock.Advance(time.Second) // 2.5 s after the first, 1 s after the second
	f.checkAsync(background, agentA)
	nextPing(t, cmds)
}

func TestAnAgentDisconnectedCheckIsAlsoReusedWithin2Seconds(t *testing.T) {
	f := newFixture()
	f.checkNow(t, agentA)
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	f.clock.Advance(CheckInterval)
	if r := f.checkNow(t, agentA); r != agentGone {
		t.Errorf("result %+v, want the last result agent_disconnected", r)
	}
	noCommand(t, cmds)
	f.clock.Advance(time.Nanosecond)
	f.checkAsync(background, agentA)
	nextPing(t, cmds)
}

func TestChecksOfAnotherAgentAreCountedApart(t *testing.T) {
	f := newFixture()
	cmdsA, detachA := f.r.Attach(agentA)
	defer detachA()
	cmdsB, detachB := f.r.Attach(agentB)
	defer detachB()
	f.checkAsync(background, agentA)
	nextPing(t, cmdsA)
	f.checkAsync(background, agentB)
	nextPing(t, cmdsB)
}

func TestACallerThatHangsUpLeavesThePingWaitingForTheNextCheck(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	ctx, cancel := context.WithCancel(background)
	gaveUp := f.checkAsync(ctx, agentA)
	id := nextPing(t, cmds)
	cancel()
	resultOf(t, gaveUp)
	next := f.checkAsync(background, agentA)
	noCommand(t, cmds)
	f.r.Deliver(agentA, pong(id, "1.2.3"))
	if r := resultOf(t, next); r != ok123 {
		t.Errorf("the next check got %+v, want the pong of the first ping", r)
	}
}

func TestAPongWithAWaitingKillsIDIsRefusedAndTheKillKeepsWaiting(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	out := f.killAsync(agentA)
	nextCommand(t, cmds)
	if f.r.Deliver(agentA, pong(killID, "1.2.3")) {
		t.Error("Deliver(pong) with the kill's id = true")
	}
	stillWaiting(t, out)
	f.r.Deliver(agentA, killed)
	if o := outcomeOf(t, out); o.Outcome != protocol.Killed {
		t.Errorf("outcome %+v, want the agent's killed", o)
	}
}

func TestAKillResultWithAWaitingPingsIDIsRefusedAndThePingKeepsWaiting(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	out := f.checkAsync(background, agentA)
	id := nextPing(t, cmds)
	if f.r.Deliver(agentA, protocol.CommandResult{ID: id, Type: protocol.ResultKill, Outcome: protocol.Killed}) {
		t.Error("Deliver(kill) with the ping's id = true")
	}
	stillWaiting(t, out)
	f.r.Deliver(agentA, pong(id, "1.2.3"))
	if r := resultOf(t, out); r != ok123 {
		t.Errorf("result %+v, want ok", r)
	}
}

func TestAPongFromAnotherAgentIsRefused(t *testing.T) {
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	_, detachB := f.r.Attach(agentB)
	defer detachB()
	out := f.checkAsync(background, agentA)
	id := nextPing(t, cmds)
	if f.r.Deliver(agentB, pong(id, "1.2.3")) {
		t.Error("Deliver(pong) from agent B = true for agent A's ping")
	}
	stillWaiting(t, out)
}

func TestACheckWhoseStreamTakesNoCommandsIsNoResponse(t *testing.T) {
	// The agent is connected but commands are not getting through: UC4's "Commands are not
	// reaching the agent. Restart the agent.", not "check that killy agent is running".
	f := newFixture()
	cmds, detach := f.r.Attach(agentA)
	defer detach()
	for i := range cap(cmds) {
		go f.r.RelayKill(background, agentA, protocol.KillCommand{ID: fmt.Sprintf("kill-%d", i), PPID: 100, StartTime: 5000})
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(cmds) < cap(cmds) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if r := f.checkNow(t, agentA); r != noResponse {
		t.Errorf("result %+v, want no_response", r)
	}
}

func TestThePingTimeoutIs5SecondsAndChecksAre2SecondsApart(t *testing.T) {
	if PingTimeout != 5*time.Second || CheckInterval != 2*time.Second {
		t.Errorf("PingTimeout = %v, CheckInterval = %v; want 5s and 2s (docs/ARCHITECTURE.md, Commands)", PingTimeout, CheckInterval)
	}
}
