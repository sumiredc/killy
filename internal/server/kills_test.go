package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/protocol"
	"github.com/sumiredc/killy/internal/server/command"
)

const killID = "16fd2706-8baf-433b-82eb-8c7fada847da"

type killReply struct {
	Outcome string `json:"outcome"`
	Reaped  int    `json:"reaped"`
	Reason  string `json:"reason"`
	Error   string `json:"error"`
}

func killBody(id, serverID string) string {
	return `{"id":"` + id + `","server_id":"` + serverID + `","ppid":100,"starttime":5000}`
}

// kill sends a kill and waits up to 3 s for the answer, so a kill that should have been refused
// but went on to wait for the agent fails the test instead of hanging it.
func (s *testServer) kill(t *testing.T, device, body string) (int, killReply) {
	t.Helper()
	c := waitKill(t, s.killAsync(t, device, body))
	return c.status, c.reply
}

type killCall struct {
	status int
	reply  killReply
}

func (s *testServer) killAsync(t *testing.T, device, body string) <-chan killCall {
	out := make(chan killCall, 1)
	go func() {
		status, b := s.do(t, "/app/v1/kills", phoneAddr, device, body)
		var r killReply
		_ = json.Unmarshal(b, &r)
		out <- killCall{status, r}
	}()
	return out
}

func waitKill(t *testing.T, out <-chan killCall) killCall {
	t.Helper()
	select {
	case c := <-out:
		return c
	case <-time.After(3 * time.Second):
		t.Fatal("the kill request did not return within 3 s")
		return killCall{}
	}
}

func (s *testServer) postResult(t *testing.T, agentToken, body string) (int, string) {
	t.Helper()
	status, b := s.do(t, "/agent/v1/results", "192.0.2.1", agentToken, body)
	return status, string(b)
}

const killedResult = `{"id":"` + killID + `","type":"kill","outcome":"killed","reaped":2}`

// nextKill reads the next kill command from an agent's stream.
func nextKill(t *testing.T, es *eventStream) protocol.KillCommand {
	t.Helper()
	it := es.next(t)
	if it.event != "kill" {
		t.Fatalf("got %+v, want a kill event", it)
	}
	var c protocol.KillCommand
	if err := json.Unmarshal([]byte(it.data), &c); err != nil {
		t.Fatalf("kill data %q: %v", it.data, err)
	}
	return c
}

// agentWithStream links a device to an agent and opens the agent's command stream.
func (s *testServer) agentWithStream(t *testing.T) (agentToken, agentID, device string, stream *eventStream) {
	t.Helper()
	agentToken, agentID, device = s.linkedDevice(t, "")
	stream = s.openStream(t, "/agent/v1/commands", agentToken)
	s.waitAttached(t, agentID)
	return
}

func (s *testServer) waitAttached(t *testing.T, agentID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !s.Commands.Attached(agentID) {
		if time.Now().After(deadline) {
			t.Fatal("the agent's command stream never attached")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestAKillReachesTheAgentAndItsOutcomeComesBack(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, device, stream := s.agentWithStream(t)
	out := s.killAsync(t, device, killBody(killID, agentID))
	if c := nextKill(t, stream); c != (protocol.KillCommand{ID: killID, PPID: 100, StartTime: 5000}) {
		t.Fatalf("command %+v, want the requested kill", c)
	}
	if status, body := s.postResult(t, agentToken, killedResult); status != http.StatusNoContent {
		t.Fatalf("result: %d %s, want 204", status, body)
	}
	if c := waitKill(t, out); c.status != http.StatusOK || c.reply != (killReply{Outcome: "killed", Reaped: 2}) {
		t.Errorf("kill: %d %+v, want 200 killed with 2 reaped", c.status, c.reply)
	}
}

func TestAKillOfAServerNotLinkedToThePhoneIsNotFound(t *testing.T) {
	s := newTestServer(t)
	_, agentID, _, stream := s.agentWithStream(t)
	other := s.newDevice(t)
	for _, serverID := range []string{agentID, "7c9e6679-7425-40de-944b-e07fc1f90ae7"} {
		if status, r := s.kill(t, other, killBody(killID, serverID)); status != http.StatusNotFound || r.Error != "server_not_found" {
			t.Errorf("server %s: %d %+v, want 404 server_not_found", serverID, status, r)
		}
	}
	stream.quiet(t)
}

func TestAKillWhoseIDIsNotAUUIDv4IsRefused(t *testing.T) {
	s := newTestServer(t)
	_, agentID, device, stream := s.agentWithStream(t)
	for _, id := range []string{"c232ab00-9414-11ec-b3c8-9f6bdeced846", "16FD2706-8BAF-433B-82EB-8C7FADA847DA", "kill-1"} {
		if status, r := s.kill(t, device, killBody(id, agentID)); status != http.StatusBadRequest || r.Error != "invalid_id" {
			t.Errorf("id %q: %d %+v, want 400 invalid_id", id, status, r)
		}
	}
	stream.quiet(t)
}

func TestAnInvalidKillRequestIsRefused(t *testing.T) {
	s := newTestServer(t)
	_, agentID, device, stream := s.agentWithStream(t)
	for name, body := range map[string]string{
		"not JSON":     "kill",
		"no ppid":      `{"id":"` + killID + `","server_id":"` + agentID + `","starttime":5000}`,
		"ppid 0":       `{"id":"` + killID + `","server_id":"` + agentID + `","ppid":0,"starttime":5000}`,
		"no starttime": `{"id":"` + killID + `","server_id":"` + agentID + `","ppid":100}`,
		"no server":    `{"id":"` + killID + `","ppid":100,"starttime":5000}`,
	} {
		if status, r := s.kill(t, device, body); status != http.StatusBadRequest || r.Error != "invalid_request" {
			t.Errorf("%s: %d %+v, want 400 invalid_request", name, status, r)
		}
	}
	stream.quiet(t)
}

func TestAKillWithoutTheAgentsStreamIsUnknownAgentDisconnected(t *testing.T) {
	s := newTestServer(t)
	_, agentID, device := s.linkedDevice(t, "")
	status, r := s.kill(t, device, killBody(killID, agentID))
	if status != http.StatusOK || r != (killReply{Outcome: "unknown", Reason: "agent_disconnected"}) {
		t.Errorf("got %d %+v, want 200 unknown agent_disconnected", status, r)
	}
}

func TestAKillWithoutAnOutcomeIn10SecondsIsUnknownTimeout(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, device, stream := s.agentWithStream(t)
	out := s.killAsync(t, device, killBody(killID, agentID))
	nextKill(t, stream)
	s.clock.Advance(command.KillTimeout)
	s.killTimer <- s.clock.Now()
	if c := waitKill(t, out); c.status != http.StatusOK || c.reply != (killReply{Outcome: "unknown", Reason: "timeout"}) {
		t.Errorf("kill: %d %+v, want 200 unknown timeout", c.status, c.reply)
	}
	// The outcome arriving after that has nobody to go to.
	if status, body := s.postResult(t, agentToken, killedResult); status != http.StatusNotFound || !strings.Contains(body, "command_not_found") {
		t.Errorf("late result: %d %s, want 404 command_not_found", status, body)
	}
}

func TestAnOutcomeAtExactly10SecondsIsTooLate(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, device, stream := s.agentWithStream(t)
	out := s.killAsync(t, device, killBody(killID, agentID))
	nextKill(t, stream)
	s.clock.Advance(command.KillTimeout)
	if status, _ := s.postResult(t, agentToken, killedResult); status != http.StatusNotFound {
		t.Errorf("result at 10 s: %d, want 404", status)
	}
	s.killTimer <- s.clock.Now()
	if c := waitKill(t, out); c.reply.Outcome != "unknown" {
		t.Errorf("kill: %+v, want unknown", c.reply)
	}
}

func TestAnOutcomeJustBefore10SecondsIsUsed(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, device, stream := s.agentWithStream(t)
	out := s.killAsync(t, device, killBody(killID, agentID))
	nextKill(t, stream)
	s.clock.Advance(command.KillTimeout - time.Nanosecond)
	s.postResult(t, agentToken, killedResult)
	if c := waitKill(t, out); c.reply.Outcome != "killed" {
		t.Errorf("kill: %+v, want killed", c.reply)
	}
}

func TestAnOutcomeFromAnotherAgentIsNotUsed(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, device, stream := s.agentWithStream(t)
	otherToken, _, _ := s.linkedDevice(t, "")
	out := s.killAsync(t, device, killBody(killID, agentID))
	nextKill(t, stream)
	if status, _ := s.postResult(t, otherToken, killedResult); status != http.StatusNotFound {
		t.Errorf("result from another agent: %d, want 404", status)
	}
	select {
	case c := <-out:
		t.Fatalf("the kill ended with %+v from the wrong agent", c)
	case <-time.After(100 * time.Millisecond):
	}
	s.postResult(t, agentToken, `{"id":"`+killID+`","type":"kill","outcome":"already_gone"}`)
	if c := waitKill(t, out); c.reply.Outcome != "already_gone" {
		t.Errorf("kill: %+v, want already_gone from the right agent", c.reply)
	}
}

func TestTwoKillsWithTheSameIDAtOnceSendOneCommandAndShareTheOutcome(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, device, stream := s.agentWithStream(t)
	first := s.killAsync(t, device, killBody(killID, agentID))
	nextKill(t, stream)
	second := s.killAsync(t, device, killBody(killID, agentID))
	stream.quiet(t)
	s.postResult(t, agentToken, killedResult)
	a, b := waitKill(t, first), waitKill(t, second)
	if a != b || a.reply.Outcome != "killed" {
		t.Errorf("replies %+v and %+v, want killed twice", a, b)
	}
}

func TestKillsAreLimitedTo30AMinutePerDevice(t *testing.T) {
	s := newTestServer(t)
	_, agentID, device, stream := s.agentWithStream(t)
	other := s.newDevice(t)
	id := func(i int) string {
		return "16fd2706-8baf-433b-82eb-" + strings.Repeat("0", 12-len(strconv.Itoa(i))) + strconv.Itoa(i)
	}
	// Requests that fail still count: a bad body, an unlinked server, a resend of the same ID.
	s.kill(t, device, "kill")
	s.kill(t, device, killBody(id(1), "7c9e6679-7425-40de-944b-e07fc1f90ae7"))
	for i := 3; i <= KillLimit; i++ {
		out := s.killAsync(t, device, killBody(id(i%3+2), agentID))
		nextKill(t, stream)
		s.killTimer <- s.clock.Now()
		if c := waitKill(t, out); c.status != http.StatusOK {
			t.Fatalf("kill %d: %d %+v, want 200", i, c.status, c.reply)
		}
	}
	for range 3 {
		if status, r := s.kill(t, device, killBody(id(99), agentID)); status != http.StatusTooManyRequests || r.Error != "rate_limited" {
			t.Errorf("kill over the limit: %d %+v, want 429 rate_limited", status, r)
		}
	}
	stream.quiet(t)
	if status, r := s.kill(t, other, killBody(id(98), agentID)); status != http.StatusNotFound {
		t.Errorf("another device: %d %+v, want its own count (404 for an unlinked server)", status, r)
	}
	s.clock.Advance(KillPeriod)
	out := s.killAsync(t, device, killBody(id(97), agentID))
	nextKill(t, stream)
	s.killTimer <- s.clock.Now()
	if c := waitKill(t, out); c.status != http.StatusOK {
		t.Errorf("after a minute: %d %+v, want 200", c.status, c.reply)
	}
}

func TestTheCommandStreamSendsHeartbeats(t *testing.T) {
	s := newTestServer(t)
	s.HeartbeatInterval = 30 * time.Millisecond
	_, _, _, stream := s.agentWithStream(t)
	if it := stream.next(t); it.comment != "hb" {
		t.Errorf("got %+v, want : hb", it)
	}
	if h := stream.res.Header.Get("Content-Type"); h != "text/event-stream" {
		t.Errorf("Content-Type %q, want text/event-stream", h)
	}
}

func TestTheCommandStreamNeedsAValidAgentToken(t *testing.T) {
	s := newTestServer(t)
	for _, token := range []string{"", "bm90LWFuLWFnZW50"} {
		if status, b := s.doGet(t, "/agent/v1/commands", token); status != http.StatusUnauthorized || !strings.Contains(b, "unauthorized") {
			t.Errorf("token %q: %d %s, want 401 unauthorized", token, status, b)
		}
	}
}

func TestResultsNeedAValidAgentTokenAndBody(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, _, _ := s.agentWithStream(t)
	if status, _ := s.postResult(t, "bm90LWFuLWFnZW50", killedResult); status != http.StatusUnauthorized {
		t.Errorf("unknown token: %d, want 401", status)
	}
	for name, body := range map[string]string{
		"not JSON":        "result",
		"no id":           `{"type":"kill","outcome":"killed"}`,
		"unknown outcome": `{"id":"` + killID + `","type":"kill","outcome":"unknown"}`,
		"no outcome":      `{"id":"` + killID + `","type":"kill"}`,
		"unknown type":    `{"id":"` + killID + `","type":"reboot","outcome":"killed"}`,
	} {
		if status, b := s.postResult(t, agentToken, body); status != http.StatusBadRequest || !strings.Contains(b, "invalid_request") {
			t.Errorf("%s: %d %s, want 400 invalid_request", name, status, b)
		}
	}
}

func TestStopStreamsClosesCommandStreamsToo(t *testing.T) {
	s := newTestServer(t)
	_, agentID, _, stream := s.agentWithStream(t)
	s.StopStreams()
	select {
	case _, ok := <-stream.items:
		if ok {
			t.Error("got an item, want the stream to end")
		}
	case <-time.After(2 * time.Second):
		t.Error("the command stream is still open")
	}
	deadline := time.Now().Add(2 * time.Second)
	for s.Commands.Attached(agentID) {
		if time.Now().After(deadline) {
			t.Fatal("the closed stream is still attached")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestManyKillsAtOnceAreSafe(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, device, stream := s.agentWithStream(t)
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() { defer wg.Done(); s.do(t, "/app/v1/kills", phoneAddr, device, killBody(killID, agentID)) }()
	}
	nextKill(t, stream)
	s.postResult(t, agentToken, killedResult)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	for {
		select {
		case <-done:
			return
		case it := <-stream.items:
			if it.event == "kill" {
				s.postResult(t, agentToken, killedResult)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the kills did not finish")
		}
	}
}

func TestTheKillLimitIs30AMinute(t *testing.T) {
	if KillLimit != 30 || KillPeriod != time.Minute {
		t.Errorf("KillLimit %d per %v, want 30 per minute (docs/ARCHITECTURE.md, Rate limits)", KillLimit, KillPeriod)
	}
}
