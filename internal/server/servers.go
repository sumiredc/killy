package server

import (
	"errors"
	"net/http"

	"github.com/sumiredc/killy/internal/server/store"
)

// serverPatch is ServerPatch in docs/openapi.yaml; a missing field leaves that setting as it is.
type serverPatch struct {
	Name   *string `json:"name"`
	Notify *bool   `json:"notify"`
}

// patchServer is the app's PATCH /app/v1/servers/{id}: it renames or mutes the server for this
// device's link only.
func (s *Server) patchServer(w http.ResponseWriter, r *http.Request) {
	device, ok := s.authDevice(w, r)
	if !ok {
		return
	}
	var req serverPatch
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	name, ok := displayName(req.Name)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	agentID, ok := s.linkedServer(w, r, device.ID)
	if !ok {
		return
	}
	if _, err := s.Store.UpdateLink(r.Context(), device.ID, agentID, name, req.Notify); err != nil {
		s.writeLinkChangeError(w, "update link", agentID, err)
		return
	}
	s.NotifyDevice(device.ID)
	w.WriteHeader(http.StatusNoContent)
}

// deleteServer is the app's DELETE /app/v1/servers/{id}: it unlinks this device from the agent.
// The agent and the other devices' links are left as they are.
func (s *Server) deleteServer(w http.ResponseWriter, r *http.Request) {
	device, ok := s.authDevice(w, r)
	if !ok {
		return
	}
	agentID, ok := s.linkedServer(w, r, device.ID)
	if !ok {
		return
	}
	if err := s.Store.UnlinkDevice(r.Context(), device.ID, agentID); err != nil {
		s.writeLinkChangeError(w, "unlink device", agentID, err)
		return
	}
	s.NotifyDevice(device.ID)
	w.WriteHeader(http.StatusNoContent)
}

// linkedServer returns the {id} of the path when the device is linked to it. The check is made in
// Go before any query takes the id, so an id that is not a UUID is not found instead of failing in
// PostgreSQL's uuid cast. When it returns false it has already written the error response.
func (s *Server) linkedServer(w http.ResponseWriter, r *http.Request, deviceID string) (string, bool) {
	agentID := r.PathValue("id")
	linked, err := s.linked(r.Context(), deviceID, agentID)
	if err != nil {
		s.Log.Error("check link", "device", deviceID, "err", err)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return "", false
	}
	if !linked {
		writeError(w, http.StatusNotFound, "server_not_found")
		return "", false
	}
	return agentID, true
}

// writeLinkChangeError answers 404 when the link went away after linkedServer, as when another
// request of the same device removed it, and 503 otherwise.
func (s *Server) writeLinkChangeError(w http.ResponseWriter, what, agentID string, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "server_not_found")
		return
	}
	s.Log.Error(what, "agent", agentID, "err", err)
	writeError(w, http.StatusServiceUnavailable, "unavailable")
}

// checkResponse is CheckResponse in docs/openapi.yaml: the result and, for ok, the agent's version.
// Nothing about timing is measured or returned.
type checkResponse struct {
	Result  string `json:"result"`
	Version string `json:"version,omitempty"`
}

// checkServer is the app's POST /app/v1/servers/{id}/check: a ping over the agent's command
// stream, so a passing check means a kill can get through too.
func (s *Server) checkServer(w http.ResponseWriter, r *http.Request) {
	device, ok := s.authDevice(w, r)
	if !ok {
		return
	}
	agentID, ok := s.linkedServer(w, r, device.ID)
	if !ok {
		return
	}
	res := s.Commands.Check(r.Context(), agentID)
	writeJSON(w, http.StatusOK, checkResponse{Result: res.Result, Version: res.Version})
}
