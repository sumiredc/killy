package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/sumiredc/killy/internal/server/memstore"
	"github.com/sumiredc/killy/internal/server/store"
)

// sendServer sends PATCH or DELETE to /app/v1/servers/{id}; body is ignored for DELETE.
func (s *testServer) sendServer(t *testing.T, method, deviceToken, serverID, body string) (int, string) {
	t.Helper()
	var r *http.Request
	if method == http.MethodDelete {
		r = httptest.NewRequest(method, "/app/v1/servers/"+serverID, nil)
	} else {
		r = httptest.NewRequest(method, "/app/v1/servers/"+serverID, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	r.RemoteAddr = phoneAddr + ":40000"
	if deviceToken != "" {
		r.Header.Set("Authorization", "Bearer "+deviceToken)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w.Code, w.Body.String()
}

func (s *testServer) patchServer(t *testing.T, deviceToken, serverID, body string) (int, string) {
	t.Helper()
	return s.sendServer(t, http.MethodPatch, deviceToken, serverID, body)
}

func (s *testServer) deleteServer(t *testing.T, deviceToken, serverID string) (int, string) {
	t.Helper()
	return s.sendServer(t, http.MethodDelete, deviceToken, serverID, "")
}

func errorCode(t *testing.T, body string) string {
	t.Helper()
	var e struct{ Error string }
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatalf("body %q is not JSON: %v", body, err)
	}
	return e.Error
}

// twoPhones links two devices to one agent: phone A names it "Prod web", phone B "My box".
func (s *testServer) twoPhones(t *testing.T) (agentToken, agentID, phoneA, phoneB string) {
	t.Helper()
	a := s.register(t)
	phoneA, phoneB = s.newDevice(t), s.newDevice(t)
	if status, resp := s.claim(t, phoneA, phoneAddr, claimBody(a.Code, "Prod web")); status != http.StatusOK {
		t.Fatalf("claim A: %d %+v", status, resp)
	}
	_, again := s.pair(t, "192.0.2.1", *a.AgentToken, hostBody)
	if status, resp := s.claim(t, phoneB, phoneAddr, claimBody(again.Code, "My box")); status != http.StatusOK {
		t.Fatalf("claim B: %d %+v", status, resp)
	}
	return *a.AgentToken, a.AgentID, phoneA, phoneB
}

// serverIn returns the server with the ID in the device's current state.
func (s *testServer) serverIn(t *testing.T, deviceToken, serverID string) (sv struct {
	Name    string
	Notify  bool
	Parents int
}, found bool) {
	t.Helper()
	m := s.openEvents(t, deviceToken).nextState(t)
	for _, x := range m.Servers {
		if x.ID == serverID {
			sv.Name, sv.Notify, sv.Parents = x.Name, x.Notify, len(x.Parents)
			return sv, true
		}
	}
	return sv, false
}

// Phone B acts in the tests below because its link is the second one stored: a store that matched
// the link by agent alone would change phone A's.

func TestRenamingChangesTheNameOnThisPhoneOnly(t *testing.T) {
	s := newTestServer(t)
	_, agentID, phoneA, phoneB := s.twoPhones(t)
	if status, body := s.patchServer(t, phoneB, agentID, `{"name":"Renamed"}`); status != http.StatusNoContent {
		t.Fatalf("PATCH: %d %s, want 204", status, body)
	}
	if sv, _ := s.serverIn(t, phoneB, agentID); sv.Name != "Renamed" {
		t.Errorf("phone B sees %q, want Renamed", sv.Name)
	}
	if sv, _ := s.serverIn(t, phoneA, agentID); sv.Name != "Prod web" {
		t.Errorf("phone A sees %q, want its own name Prod web", sv.Name)
	}
}

func TestANameIsSavedWithoutLeadingAndTrailingSpaces(t *testing.T) {
	s := newTestServer(t)
	_, agentID, phoneA, _ := s.twoPhones(t)
	if status, body := s.patchServer(t, phoneA, agentID, `{"name":"  Renamed \t"}`); status != http.StatusNoContent {
		t.Fatalf("PATCH: %d %s, want 204", status, body)
	}
	if sv, _ := s.serverIn(t, phoneA, agentID); sv.Name != "Renamed" {
		t.Errorf("phone A sees %q, want Renamed", sv.Name)
	}
}

func TestAMissingEmptyOrBlankNameKeepsThePreviousName(t *testing.T) {
	for _, body := range []string{`{}`, `{"name":null}`, `{"name":""}`, `{"name":"   "}`, `{"name":" \t\n"}`} {
		t.Run(body, func(t *testing.T) {
			s := newTestServer(t)
			_, agentID, phoneA, _ := s.twoPhones(t)
			if status, resp := s.patchServer(t, phoneA, agentID, body); status != http.StatusNoContent {
				t.Fatalf("PATCH: %d %s, want 204", status, resp)
			}
			if sv, _ := s.serverIn(t, phoneA, agentID); sv.Name != "Prod web" {
				t.Errorf("phone A sees %q, want the previous name Prod web", sv.Name)
			}
		})
	}
}

func TestANameOf100CharactersIsAccepted(t *testing.T) {
	for _, name := range []string{strings.Repeat("a", 100), strings.Repeat("あ", 100)} {
		s := newTestServer(t)
		_, agentID, phoneA, _ := s.twoPhones(t)
		if status, body := s.patchServer(t, phoneA, agentID, `{"name":"`+name+`"}`); status != http.StatusNoContent {
			t.Fatalf("%d runes of %q: %d %s, want 204", 100, name[:1], status, body)
		}
		if sv, _ := s.serverIn(t, phoneA, agentID); sv.Name != name {
			t.Errorf("phone A sees %q, want the 100-character name", sv.Name)
		}
	}
}

func TestANameOver100CharactersIsRefusedAndChangesNothing(t *testing.T) {
	// The length is counted before the spaces are trimmed, as in the claim.
	for _, name := range []string{strings.Repeat("a", 101), strings.Repeat("あ", 101), " " + strings.Repeat("a", 100)} {
		s := newTestServer(t)
		_, agentID, phoneA, _ := s.twoPhones(t)
		before := s.mem.Links()
		status, body := s.patchServer(t, phoneA, agentID, `{"name":"`+name+`","notify":false}`)
		if status != http.StatusBadRequest || errorCode(t, body) != "invalid_request" {
			t.Errorf("name of %d bytes: %d %s, want 400 invalid_request", len(name), status, body)
		}
		if after := s.mem.Links(); !slices.Equal(before, after) {
			t.Errorf("links changed from %+v to %+v, want neither the name nor the switch changed", before, after)
		}
	}
}

func TestABodyThatIsNotAServerPatchIsRefused(t *testing.T) {
	for _, body := range []string{`not json`, `{"notify":"no"}`, `{"name":5}`, `{"name":"x"} {}`} {
		s := newTestServer(t)
		_, agentID, phoneA, _ := s.twoPhones(t)
		before := s.mem.Links()
		if status, resp := s.patchServer(t, phoneA, agentID, body); status != http.StatusBadRequest || errorCode(t, resp) != "invalid_request" {
			t.Errorf("body %s: %d %s, want 400 invalid_request", body, status, resp)
		}
		if after := s.mem.Links(); !slices.Equal(before, after) {
			t.Errorf("body %s: links changed from %+v to %+v", body, before, after)
		}
	}
}

func TestMutingKeepsTheServerAndItsParentsAndLeavesOtherPhonesOn(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, phoneA, phoneB := s.twoPhones(t)
	s.postSnapshot(t, agentToken, snapshotJSON(t0, parent100+","+parent200))
	if status, body := s.patchServer(t, phoneB, agentID, `{"notify":false}`); status != http.StatusNoContent {
		t.Fatalf("PATCH: %d %s, want 204", status, body)
	}
	sv, found := s.serverIn(t, phoneB, agentID)
	if !found || sv.Notify || sv.Parents != 2 || sv.Name != "My box" {
		t.Errorf("phone B sees %+v (found %v), want My box muted with its 2 parents", sv, found)
	}
	if sv, _ := s.serverIn(t, phoneA, agentID); !sv.Notify {
		t.Errorf("phone A sees %+v, want notifications still on", sv)
	}
}

func TestNotificationsCanBeTurnedBackOn(t *testing.T) {
	s := newTestServer(t)
	_, agentID, phoneA, _ := s.twoPhones(t)
	s.patchServer(t, phoneA, agentID, `{"notify":false}`)
	if status, body := s.patchServer(t, phoneA, agentID, `{"notify":true}`); status != http.StatusNoContent {
		t.Fatalf("PATCH: %d %s, want 204", status, body)
	}
	if sv, _ := s.serverIn(t, phoneA, agentID); !sv.Notify {
		t.Errorf("phone A sees %+v, want notifications on again", sv)
	}
}

func TestOnlyTheSwitchChangesWhenNoNameIsSent(t *testing.T) {
	s := newTestServer(t)
	_, agentID, phoneA, _ := s.twoPhones(t)
	s.patchServer(t, phoneA, agentID, `{"notify":false}`)
	s.patchServer(t, phoneA, agentID, `{"name":"Renamed"}`)
	if sv, _ := s.serverIn(t, phoneA, agentID); sv.Name != "Renamed" || sv.Notify {
		t.Errorf("phone A sees %+v, want Renamed and still muted", sv)
	}
}

func TestRemovingUnlinksThisPhoneOnly(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, phoneA, phoneB := s.twoPhones(t)
	s.postSnapshot(t, agentToken, snapshotJSON(t0, parent100))
	commands := s.openStream(t, "/agent/v1/commands", agentToken)
	s.waitAttached(t, agentID)

	if status, body := s.deleteServer(t, phoneB, agentID); status != http.StatusNoContent {
		t.Fatalf("DELETE: %d %s, want 204", status, body)
	}
	if _, found := s.serverIn(t, phoneB, agentID); found {
		t.Error("phone B still sees the server, want it gone with its parents")
	}
	if sv, found := s.serverIn(t, phoneA, agentID); !found || sv.Parents != 1 || sv.Name != "Prod web" {
		t.Errorf("phone A sees %+v (found %v), want Prod web with its parent", sv, found)
	}
	if len(s.mem.Agents()) != 1 {
		t.Errorf("%d agents stored, want the agent kept", len(s.mem.Agents()))
	}
	if _, ok := s.Live.Agent(agentID); !ok {
		t.Error("the agent's snapshot is gone from the live state, want it kept")
	}
	commands.quiet(t)
}

func TestARemovedServerCanBeRemovedOnlyOnce(t *testing.T) {
	s := newTestServer(t)
	_, agentID, phoneA, _ := s.twoPhones(t)
	s.deleteServer(t, phoneA, agentID)
	if status, body := s.deleteServer(t, phoneA, agentID); status != http.StatusNotFound || errorCode(t, body) != "server_not_found" {
		t.Errorf("second DELETE: %d %s, want 404 server_not_found", status, body)
	}
}

func TestAServerNotLinkedToThePhoneIsNotFoundAndNothingChanges(t *testing.T) {
	s := newTestServer(t)
	_, agentID, _ := s.linkedDevice(t, "Theirs")
	phone := s.newDevice(t)
	for _, id := range []string{agentID, "7c9e6679-7425-40de-944b-e07fc1f90ae7", "not-a-uuid"} {
		before := s.mem.Links()
		if status, body := s.patchServer(t, phone, id, `{"name":"Mine","notify":false}`); status != http.StatusNotFound || errorCode(t, body) != "server_not_found" {
			t.Errorf("PATCH %s: %d %s, want 404 server_not_found", id, status, body)
		}
		if status, body := s.deleteServer(t, phone, id); status != http.StatusNotFound || errorCode(t, body) != "server_not_found" {
			t.Errorf("DELETE %s: %d %s, want 404 server_not_found", id, status, body)
		}
		if after := s.mem.Links(); !slices.Equal(before, after) {
			t.Errorf("%s: links changed from %+v to %+v", id, before, after)
		}
	}
}

func TestAnInvalidDeviceTokenCannotChangeOrRemoveAServer(t *testing.T) {
	s := newTestServer(t)
	_, agentID, _, _ := s.twoPhones(t)
	before := s.mem.Links()
	for _, token := range []string{"", "bm90LWEtZGV2aWNl"} {
		if status, body := s.patchServer(t, token, agentID, `{"name":"Mine","notify":false}`); status != http.StatusUnauthorized || errorCode(t, body) != "unauthorized" {
			t.Errorf("PATCH with token %q: %d %s, want 401 unauthorized", token, status, body)
		}
		if status, body := s.deleteServer(t, token, agentID); status != http.StatusUnauthorized || errorCode(t, body) != "unauthorized" {
			t.Errorf("DELETE with token %q: %d %s, want 401 unauthorized", token, status, body)
		}
	}
	if after := s.mem.Links(); !slices.Equal(before, after) {
		t.Errorf("links changed from %+v to %+v", before, after)
	}
}

func TestChangesAreUnavailableWhenTheDatabaseCannotBeReached(t *testing.T) {
	cases := []struct {
		name string
		fail func(*testServer)
	}{
		{"every call fails", (*testServer).unreachableDB},
		{"the link lookup fails", func(s *testServer) { s.mem.FailLinksOfDevice = true }},
		{"the change itself fails", func(s *testServer) { s.mem.FailLinkChanges = true }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newTestServer(t)
			_, agentID, phoneA, _ := s.twoPhones(t)
			before := s.mem.Links()
			c.fail(s)
			if status, body := s.patchServer(t, phoneA, agentID, `{"name":"Renamed"}`); status != http.StatusServiceUnavailable || errorCode(t, body) != "unavailable" {
				t.Errorf("PATCH: %d %s, want 503 unavailable", status, body)
			}
			if status, body := s.deleteServer(t, phoneA, agentID); status != http.StatusServiceUnavailable || errorCode(t, body) != "unavailable" {
				t.Errorf("DELETE: %d %s, want 503 unavailable", status, body)
			}
			s.mem.Err, s.mem.FailLinksOfDevice, s.mem.FailLinkChanges = nil, false, false
			if after := s.mem.Links(); !slices.Equal(before, after) {
				t.Errorf("links changed from %+v to %+v", before, after)
			}
		})
	}
}

func TestAChangeSendsStateOnlyToThatPhone(t *testing.T) {
	s := newTestServer(t)
	_, agentID, phoneA, phoneB := s.twoPhones(t)
	a, b := s.openEvents(t, phoneA), s.openEvents(t, phoneB)
	a.nextState(t)
	b.nextState(t)

	s.patchServer(t, phoneA, agentID, `{"name":"Renamed"}`)
	if m := a.nextState(t); len(m.Servers) != 1 || m.Servers[0].Name != "Renamed" {
		t.Errorf("after PATCH phone A got %+v, want Renamed", m.Servers)
	}
	b.quiet(t)

	s.deleteServer(t, phoneA, agentID)
	if m := a.nextState(t); len(m.Servers) != 0 {
		t.Errorf("after DELETE phone A got %+v, want no servers", m.Servers)
	}
	b.quiet(t)
}

func TestARefusedChangeSendsNoState(t *testing.T) {
	s := newTestServer(t)
	_, agentID, phoneA, _ := s.twoPhones(t)
	a := s.openEvents(t, phoneA)
	a.nextState(t)
	s.patchServer(t, phoneA, agentID, `{"name":"`+strings.Repeat("a", 101)+`"}`)
	s.patchServer(t, phoneA, "7c9e6679-7425-40de-944b-e07fc1f90ae7", `{"name":"x"}`)
	s.deleteServer(t, phoneA, "7c9e6679-7425-40de-944b-e07fc1f90ae7")
	a.quiet(t)
}

// staleLinks reports the links of before the test removed them, as when another request of the
// same device removes the link after the handler checked it.
type staleLinks struct {
	*memstore.Memory
	links []store.LinkedServer
}

func (s staleLinks) LinksOfDevice(context.Context, string) ([]store.LinkedServer, error) {
	return s.links, nil
}

func TestALinkRemovedAfterTheCheckIsNotFound(t *testing.T) {
	s := newTestServer(t)
	_, agentID, phoneA, _ := s.twoPhones(t)
	deviceA := s.mem.Devices()[0].ID
	links, err := s.mem.LinksOfDevice(t.Context(), deviceA)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.mem.UnlinkDevice(t.Context(), deviceA, agentID); err != nil {
		t.Fatal(err)
	}
	s.Store = staleLinks{Memory: s.mem, links: links}
	if status, body := s.patchServer(t, phoneA, agentID, `{"name":"Renamed"}`); status != http.StatusNotFound || errorCode(t, body) != "server_not_found" {
		t.Errorf("PATCH: %d %s, want 404 server_not_found", status, body)
	}
	if status, body := s.deleteServer(t, phoneA, agentID); status != http.StatusNotFound || errorCode(t, body) != "server_not_found" {
		t.Errorf("DELETE: %d %s, want 404 server_not_found", status, body)
	}
}
