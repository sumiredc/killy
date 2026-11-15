package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/sumiredc/killy/internal/server/store"
)

// addDemoServer is the app's POST /app/v1/demo: it links this device to the public demo agent,
// as a claim does but without a code. The Killy server does not check what the demo may kill;
// the demo-mode agent does (killy agent run --demo).
func (s *Server) addDemoServer(w http.ResponseWriter, r *http.Request) {
	device, ok := s.authDevice(w, r)
	if !ok {
		return
	}
	if s.DemoAgentID == "" {
		writeError(w, http.StatusNotFound, "demo_not_configured")
		return
	}
	ctx := context.WithoutCancel(r.Context())
	agent, err := s.Store.AgentByID(ctx, s.DemoAgentID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "demo_not_configured")
		return
	}
	if err != nil {
		s.Log.Error("read demo agent", "agent", s.DemoAgentID, "err", err)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	// No name: a second call keeps the name and switch this device chose for it.
	link, err := s.Store.LinkDevice(ctx, device.ID, agent.ID, nil, agent.Hostname)
	if err != nil {
		s.Log.Error("link device to demo agent", "agent", agent.ID, "err", err)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	s.NotifyDevice(device.ID)
	writeJSON(w, http.StatusOK, serverResponse{ID: agent.ID, Name: link.Name, Hostname: agent.Hostname, Notify: link.Notify})
}
