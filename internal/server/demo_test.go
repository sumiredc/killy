package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

func (s *testServer) addDemo(t *testing.T, device string) (int, serverReply) {
	t.Helper()
	status, b := s.do(t, "/app/v1/demo", phoneAddr, device, "")
	var r serverReply
	_ = json.Unmarshal(b, &r)
	return status, r
}

// demoServer registers an agent and makes it the public demo.
func demoServer(t *testing.T) (*testServer, string) {
	t.Helper()
	s := newTestServer(t)
	a := s.register(t)
	s.DemoAgentID = a.AgentID
	return s, a.AgentID
}

func TestTheDemoServerIsLinkedWithItsHostname(t *testing.T) {
	s, agentID := demoServer(t)
	device := s.newDevice(t)
	status, r := s.addDemo(t, device)
	if status != http.StatusOK {
		t.Fatalf("status %d, want 200", status)
	}
	if r.ID != agentID || r.Name != r.Hostname || r.Hostname == "" || r.Notify == nil || !*r.Notify {
		t.Errorf("server %+v, want the demo agent named by its hostname, notifications on", r)
	}
	if links := s.mem.Links(); len(links) != 1 || links[0].AgentID != agentID {
		t.Errorf("links %+v, want one to the demo agent", links)
	}
}

func TestAddingTheDemoServerAgainKeepsOneLinkAndItsSettings(t *testing.T) {
	s, agentID := demoServer(t)
	device := s.newDevice(t)
	s.addDemo(t, device)
	if status, body := s.patchServer(t, device, agentID, `{"name":"My demo","notify":false}`); status != http.StatusNoContent {
		t.Fatalf("patch: %d %s", status, body)
	}
	status, r := s.addDemo(t, device)
	if status != http.StatusOK {
		t.Fatalf("status %d, want 200", status)
	}
	if r.Name != "My demo" || r.Notify == nil || *r.Notify {
		t.Errorf("server %+v, want the name and switch set before kept", r)
	}
	if links := s.mem.Links(); len(links) != 1 || links[0].Name != "My demo" || links[0].Notify {
		t.Errorf("links %+v, want one, unchanged", links)
	}
}

func TestAddingTheDemoServerSendsTheNewStateToThatDeviceOnly(t *testing.T) {
	s, agentID := demoServer(t)
	device, other := s.newDevice(t), s.newDevice(t)
	es, otherES := s.openEvents(t, device), s.openEvents(t, other)
	es.nextState(t)
	otherES.nextState(t)
	s.addDemo(t, device)
	st := es.nextState(t)
	if len(st.Servers) != 1 || st.Servers[0].ID != agentID {
		t.Errorf("state %+v, want the demo server", st)
	}
	otherES.quiet(t)
}

func TestWithoutADemoServerNothingIsLinked(t *testing.T) {
	for name, demoID := range map[string]string{
		"not set":       "",
		"unknown agent": "00000000-0000-4000-8000-000000000999",
	} {
		t.Run(name, func(t *testing.T) {
			s := newTestServer(t)
			s.register(t)
			s.DemoAgentID = demoID
			device := s.newDevice(t)
			status, r := s.addDemo(t, device)
			if status != http.StatusNotFound || r.Error != "demo_not_configured" {
				t.Errorf("%d %+v, want 404 demo_not_configured", status, r)
			}
			if n := len(s.mem.Links()); n != 0 {
				t.Errorf("%d links, want 0", n)
			}
		})
	}
}

func TestAnInvalidDeviceTokenCannotAddTheDemoServer(t *testing.T) {
	s, _ := demoServer(t)
	for _, token := range []string{"", "not-a-device-token"} {
		status, r := s.addDemo(t, token)
		if status != http.StatusUnauthorized || r.Error != "unauthorized" {
			t.Errorf("token %q: %d %+v, want 401 unauthorized", token, status, r)
		}
	}
	if n := len(s.mem.Links()); n != 0 {
		t.Errorf("%d links, want 0", n)
	}
}

func TestTheDemoServerIsUnavailableWhenTheDatabaseIsNot(t *testing.T) {
	s, _ := demoServer(t)
	device := s.newDevice(t)
	s.mem.FailLink = true
	status, r := s.addDemo(t, device)
	if status != http.StatusServiceUnavailable || r.Error != "unavailable" {
		t.Errorf("%d %+v, want 503 unavailable", status, r)
	}
}

func TestWithoutADemoServerTheDatabaseIsNotAsked(t *testing.T) {
	s := newTestServer(t)
	device := s.newDevice(t)
	s.DemoAgentID = ""
	// Every store call fails from here; an unset demo server must still be demo_not_configured.
	// (PostgreSQL answers an error, not "not found", for an empty uuid.)
	s.mem.FailLink = true
	s.mem.FailAgentByID = true
	status, r := s.addDemo(t, device)
	if status != http.StatusNotFound || r.Error != "demo_not_configured" {
		t.Errorf("%d %+v, want 404 demo_not_configured", status, r)
	}
}

func TestAnInvalidDeviceTokenIsRefusedBeforeTheDemoServerIsLookedAt(t *testing.T) {
	s := newTestServer(t)
	s.DemoAgentID = ""
	status, r := s.addDemo(t, "not-a-device-token")
	if status != http.StatusUnauthorized || r.Error != "unauthorized" {
		t.Errorf("%d %+v, want 401 unauthorized even with no demo server", status, r)
	}
}
