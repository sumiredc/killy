package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func (s *testServer) postSnapshot(t *testing.T, token, body string) (int, string) {
	t.Helper()
	r := httptest.NewRequest("POST", "/agent/v1/snapshots", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w.Code, w.Body.String()
}

func snapshotJSON(scannedAt time.Time, parents string) string {
	return `{"hostname":"web-1","version":"dev","scanned_at":"` + scannedAt.Format(time.RFC3339Nano) + `","parents":[` + parents + `]}`
}

const parent100 = `{"pid":100,"starttime":5000,"cmd":"node evil_daemon.js","zombies":[101,102]}`
const parent200 = `{"pid":200,"starttime":7000,"cmd":"python3 worker.py","zombies":[201]}`

func (s *testServer) reportedPIDs(t *testing.T, agentID string) []int {
	t.Helper()
	a, ok := s.Live.Agent(agentID)
	if !ok {
		return nil
	}
	pids := []int{}
	for _, p := range a.Parents {
		pids = append(pids, p.PID)
	}
	return pids
}

func TestASnapshotIsAcceptedAndBecomesTheAgentsState(t *testing.T) {
	s := newTestServer(t)
	agent := s.register(t)
	status, body := s.postSnapshot(t, *agent.AgentToken, snapshotJSON(t0, parent100))
	if status != http.StatusNoContent {
		t.Fatalf("status %d (%s), want 204", status, body)
	}
	a, ok := s.Live.Agent(agent.AgentID)
	if !ok || len(a.Parents) != 1 {
		t.Fatalf("state %+v, want one parent", a)
	}
	p := a.Parents[0]
	if p.PID != 100 || p.StartTime != 5000 || p.Cmd != "node evil_daemon.js" || len(p.Zombies) != 2 || !p.FirstDetectedAt.Equal(t0) {
		t.Errorf("parent %+v, want PID 100 from the snapshot, first detected at %v", p, t0)
	}
	if !s.Live.Connected(agent.AgentID) {
		t.Error("agent disconnected right after a snapshot")
	}
}

func TestAnOlderSnapshotGets204ButChangesNothing(t *testing.T) {
	s := newTestServer(t)
	agent := s.register(t)
	s.postSnapshot(t, *agent.AgentToken, snapshotJSON(t0.Add(10*time.Second), parent200))
	status, _ := s.postSnapshot(t, *agent.AgentToken, snapshotJSON(t0, parent100))
	if status != http.StatusNoContent {
		t.Errorf("status %d, want 204 so the agent does not resend it", status)
	}
	if pids := s.reportedPIDs(t, agent.AgentID); len(pids) != 1 || pids[0] != 200 {
		t.Errorf("parents %v, want [200]", pids)
	}
}

func TestASnapshotWithAnInvalidTokenIsRefusedAndChangesNothing(t *testing.T) {
	s := newTestServer(t)
	agent := s.register(t)
	s.postSnapshot(t, *agent.AgentToken, snapshotJSON(t0, parent200))
	for name, token := range map[string]string{"no token": "", "unknown token": "bm90LWEtcmVhbC10b2tlbg"} {
		t.Run(name, func(t *testing.T) {
			status, body := s.postSnapshot(t, token, snapshotJSON(t0.Add(time.Minute), parent100))
			if status != http.StatusUnauthorized || !strings.Contains(body, `"unauthorized"`) {
				t.Errorf("got %d %s, want 401 unauthorized", status, body)
			}
			if pids := s.reportedPIDs(t, agent.AgentID); len(pids) != 1 || pids[0] != 200 {
				t.Errorf("parents %v, want [200] unchanged", pids)
			}
		})
	}
}

func TestAnInvalidSnapshotIsAnInvalidRequest(t *testing.T) {
	s := newTestServer(t)
	agent := s.register(t)
	for name, body := range map[string]string{
		"not JSON":          "parents",
		"no scanned_at":     `{"hostname":"web-1","version":"dev","parents":[]}`,
		"no parents":        `{"hostname":"web-1","version":"dev","scanned_at":"2026-11-13T09:00:00Z"}`,
		"PID 0":             snapshotJSON(t0, `{"pid":0,"starttime":1,"cmd":"x","zombies":[2]}`),
		"no zombies":        snapshotJSON(t0, `{"pid":100,"starttime":1,"cmd":"x"}`),
		"empty zombies":     snapshotJSON(t0, `{"pid":100,"starttime":1,"cmd":"x","zombies":[]}`),
		"no starttime":      snapshotJSON(t0, `{"pid":100,"cmd":"x","zombies":[2]}`),
		"no cmd":            snapshotJSON(t0, `{"pid":100,"starttime":1,"zombies":[2]}`),
		"PID over the max":  snapshotJSON(t0, `{"pid":4294967396,"starttime":1,"cmd":"x","zombies":[2]}`),
		"zombie PID 0":      snapshotJSON(t0, `{"pid":100,"starttime":1,"cmd":"x","zombies":[0]}`),
		"no hostname":       `{"version":"dev","scanned_at":"2026-11-13T09:00:00Z","parents":[]}`,
		"no version":        `{"hostname":"web-1","scanned_at":"2026-11-13T09:00:00Z","parents":[]}`,
		"over the size cap": snapshotJSON(t0, parent100) + strings.Repeat(" ", maxSnapshotBytes),
	} {
		t.Run(name, func(t *testing.T) {
			status, resp := s.postSnapshot(t, *agent.AgentToken, body)
			if status != http.StatusBadRequest || !strings.Contains(resp, `"invalid_request"`) {
				t.Errorf("got %d %s, want 400 invalid_request", status, resp)
			}
			if _, ok := s.Live.Agent(agent.AgentID); ok {
				t.Error("the invalid snapshot became the agent's state")
			}
		})
	}
}

func TestASnapshotIsUnavailableWhenTheDatabaseCannotBeReached(t *testing.T) {
	s := newTestServer(t)
	s.unreachableDB()
	status, body := s.postSnapshot(t, "c29tZS10b2tlbg", snapshotJSON(t0, parent100))
	if status != http.StatusServiceUnavailable || !strings.Contains(body, `"unavailable"`) {
		t.Errorf("got %d %s, want 503 unavailable", status, body)
	}
}

func TestAStartTimeOfZeroIsAccepted(t *testing.T) {
	s := newTestServer(t)
	agent := s.register(t)
	status, body := s.postSnapshot(t, *agent.AgentToken, snapshotJSON(t0, `{"pid":100,"starttime":0,"cmd":"x","zombies":[101]}`))
	if status != http.StatusNoContent {
		t.Errorf("got %d %s, want 204", status, body)
	}
}
