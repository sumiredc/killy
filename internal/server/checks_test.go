package server

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/protocol"
	"github.com/sumiredc/killy/internal/server/command"
)

type checkCall struct {
	status int
	body   map[string]any
}

// checkAsync sends POST /app/v1/servers/{id}/check and returns where its answer will arrive.
func (s *testServer) checkAsync(t *testing.T, device, serverID string) <-chan checkCall {
	out := make(chan checkCall, 1)
	go func() {
		status, b := s.do(t, "/app/v1/servers/"+serverID+"/check", phoneAddr, device, "")
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		out <- checkCall{status, body}
	}()
	return out
}

func waitCheck(t *testing.T, out <-chan checkCall) checkCall {
	t.Helper()
	select {
	case c := <-out:
		return c
	case <-time.After(3 * time.Second):
		t.Fatal("the check did not return within 3 s")
		return checkCall{}
	}
}

func (s *testServer) check(t *testing.T, device, serverID string) checkCall {
	t.Helper()
	return waitCheck(t, s.checkAsync(t, device, serverID))
}

// nextPingID reads the next command from an agent's stream, which must be a ping.
func nextPingID(t *testing.T, es *eventStream) string {
	t.Helper()
	it := es.next(t)
	var p protocol.PingCommand
	if it.event != "ping" || json.Unmarshal([]byte(it.data), &p) != nil || p.ID == "" {
		t.Fatalf("got %+v, want a ping event with an id", it)
	}
	return p.ID
}

func pongBody(id, version string) string {
	return `{"id":"` + id + `","type":"pong","version":"` + version + `"}`
}

func keys(m map[string]any) []string { return slices.Sorted(maps.Keys(m)) }

func TestACheckOfAConnectedAgentIsOkWithItsVersionAndNothingElse(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, device, stream := s.agentWithStream(t)
	out := s.checkAsync(t, device, agentID)
	id := nextPingID(t, stream)
	if status, body := s.postResult(t, agentToken, pongBody(id, "1.2.3")); status != http.StatusNoContent {
		t.Fatalf("pong: %d %s, want 204", status, body)
	}
	c := waitCheck(t, out)
	if c.status != http.StatusOK || c.body["result"] != "ok" || c.body["version"] != "1.2.3" {
		t.Errorf("check: %d %v, want 200 ok with version 1.2.3", c.status, c.body)
	}
	if k := keys(c.body); !slices.Equal(k, []string{"result", "version"}) {
		t.Errorf("check fields %v, want result and version only (no round trip, no last-heard time)", k)
	}
}

func TestACheckOfAnAgentWithoutAStreamIsAgentDisconnected(t *testing.T) {
	s := newTestServer(t)
	_, agentID, device := s.linkedDevice(t, "")
	c := s.check(t, device, agentID)
	if c.status != http.StatusOK || c.body["result"] != "agent_disconnected" || !slices.Equal(keys(c.body), []string{"result"}) {
		t.Errorf("check: %d %v, want 200 {result: agent_disconnected} only", c.status, c.body)
	}
}

func TestACheckWithoutAPongIn5SecondsIsNoResponse(t *testing.T) {
	s := newTestServer(t)
	_, agentID, device, stream := s.agentWithStream(t)
	out := s.checkAsync(t, device, agentID)
	nextPingID(t, stream)
	s.clock.Advance(command.PingTimeout)
	s.killTimer <- s.clock.Now()
	c := waitCheck(t, out)
	if c.status != http.StatusOK || c.body["result"] != "no_response" || !slices.Equal(keys(c.body), []string{"result"}) {
		t.Errorf("check: %d %v, want 200 {result: no_response} only", c.status, c.body)
	}
}

func TestACheckOfAServerNotLinkedToThePhoneIsNotFoundAndSendsNoPing(t *testing.T) {
	s := newTestServer(t)
	_, agentID, _, stream := s.agentWithStream(t)
	other := s.newDevice(t)
	for _, id := range []string{agentID, "7c9e6679-7425-40de-944b-e07fc1f90ae7", "not-a-uuid"} {
		if c := s.check(t, other, id); c.status != http.StatusNotFound || c.body["error"] != "server_not_found" {
			t.Errorf("check %s: %d %v, want 404 server_not_found", id, c.status, c.body)
		}
	}
	stream.quiet(t)
}

func TestAnInvalidDeviceTokenCannotCheck(t *testing.T) {
	s := newTestServer(t)
	_, agentID, _, stream := s.agentWithStream(t)
	for _, token := range []string{"", "bm90LWEtZGV2aWNl"} {
		if c := s.check(t, token, agentID); c.status != http.StatusUnauthorized || c.body["error"] != "unauthorized" {
			t.Errorf("token %q: %d %v, want 401 unauthorized", token, c.status, c.body)
		}
	}
	stream.quiet(t)
}

func TestACheckIsUnavailableWhenTheDatabaseCannotBeReached(t *testing.T) {
	for name, fail := range map[string]func(*testServer){
		"every call fails":      (*testServer).unreachableDB,
		"the link lookup fails": func(s *testServer) { s.mem.FailLinksOfDevice = true },
	} {
		t.Run(name, func(t *testing.T) {
			s := newTestServer(t)
			_, agentID, device, stream := s.agentWithStream(t)
			fail(s)
			if c := s.check(t, device, agentID); c.status != http.StatusServiceUnavailable || c.body["error"] != "unavailable" {
				t.Errorf("check: %d %v, want 503 unavailable", c.status, c.body)
			}
			stream.quiet(t)
		})
	}
}

func TestAPongWithoutAVersionIsAnInvalidRequest(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, device, stream := s.agentWithStream(t)
	out := s.checkAsync(t, device, agentID)
	id := nextPingID(t, stream)
	for _, body := range []string{`{"id":"` + id + `","type":"pong"}`, `{"id":"` + id + `","type":"pong","version":""}`} {
		if status, resp := s.postResult(t, agentToken, body); status != http.StatusBadRequest {
			t.Errorf("result %s: %d %s, want 400", body, status, resp)
		}
	}
	select {
	case c := <-out:
		t.Fatalf("the check ended with %+v, want it still waiting", c)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestAResultOfAnotherTypeIsAnInvalidRequest(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, _, _ := s.agentWithStream(t)
	if status, body := s.postResult(t, agentToken, `{"id":"`+killID+`","type":"ping","version":"1.2.3"}`); status != http.StatusBadRequest {
		t.Errorf("result: %d %s, want 400", status, body)
	}
}

func TestAPongWithAWaitingKillsIDIsNotFoundAndTheKillKeepsWaiting(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, device, stream := s.agentWithStream(t)
	out := s.killAsync(t, device, killBody(killID, agentID))
	nextKill(t, stream)
	if status, body := s.postResult(t, agentToken, pongBody(killID, "1.2.3")); status != http.StatusNotFound || errorCode(t, body) != "command_not_found" {
		t.Errorf("pong: %d %s, want 404 command_not_found", status, body)
	}
	select {
	case c := <-out:
		t.Fatalf("the kill ended with %+v, want it still waiting", c)
	case <-time.After(100 * time.Millisecond):
	}
	s.postResult(t, agentToken, killedResult)
	if c := waitKill(t, out); c.reply.Outcome != "killed" {
		t.Errorf("kill: %+v, want killed", c.reply)
	}
}

func TestAKillResultWithAWaitingPingsIDIsNotFoundAndTheCheckKeepsWaiting(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, device, stream := s.agentWithStream(t)
	out := s.checkAsync(t, device, agentID)
	id := nextPingID(t, stream)
	if status, body := s.postResult(t, agentToken, `{"id":"`+id+`","type":"kill","outcome":"killed","reaped":1}`); status != http.StatusNotFound || errorCode(t, body) != "command_not_found" {
		t.Errorf("kill result: %d %s, want 404 command_not_found", status, body)
	}
	select {
	case c := <-out:
		t.Fatalf("the check ended with %+v, want it still waiting", c)
	case <-time.After(100 * time.Millisecond):
	}
	s.postResult(t, agentToken, pongBody(id, "1.2.3"))
	if c := waitCheck(t, out); c.body["result"] != "ok" {
		t.Errorf("check: %v, want ok", c.body)
	}
}

func TestAPongFromAnotherAgentIsNotFound(t *testing.T) {
	s := newTestServer(t)
	_, agentID, device, stream := s.agentWithStream(t)
	otherToken, _, _ := s.linkedDevice(t, "")
	s.checkAsync(t, device, agentID)
	id := nextPingID(t, stream)
	if status, body := s.postResult(t, otherToken, pongBody(id, "1.2.3")); status != http.StatusNotFound || errorCode(t, body) != "command_not_found" {
		t.Errorf("pong from another agent: %d %s, want 404 command_not_found", status, body)
	}
}

func TestTwoChecksWithin2SecondsSendOnePing(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, device, stream := s.agentWithStream(t)
	first := s.checkAsync(t, device, agentID)
	id := nextPingID(t, stream)
	second := s.checkAsync(t, device, agentID)
	stream.quiet(t)
	s.postResult(t, agentToken, pongBody(id, "1.2.3"))
	if a, b := waitCheck(t, first), waitCheck(t, second); a.body["result"] != "ok" || b.body["result"] != "ok" {
		t.Errorf("checks: %v and %v, want ok twice", a.body, b.body)
	}
}
