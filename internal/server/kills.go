package server

import (
	"context"
	"net/http"

	"github.com/sumiredc/killy/internal/protocol"
	"github.com/sumiredc/killy/internal/server/sse"
)

// streamCommands is the agent's GET /agent/v1/commands: kill commands as SSE events. The agent
// opens it, since agents accept no inbound connections.
func (s *Server) streamCommands(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.authAgent(w, r)
	if !ok {
		return
	}
	sw, err := sse.Start(w)
	if err != nil {
		return
	}
	cmds, detach := s.Commands.Attach(agent.ID)
	defer detach()
	ticks, stop := s.heartbeats()
	defer stop()
	stopping := s.stoppingChan()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-stopping:
			return
		case <-ticks:
			if err := sw.Heartbeat(); err != nil {
				return
			}
		case c, ok := <-cmds:
			if !ok {
				// A newer stream of the same agent took over. Two agents running with one token
				// would take turns here, about once a second.
				s.Log.Warn("command stream replaced by a newer one of the same agent", "agent", agent.ID)
				return
			}
			if err := sw.Event(c.Event, c.Data); err != nil {
				return
			}
		}
	}
}

// postResult is the agent's POST /agent/v1/results.
func (s *Server) postResult(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.authAgent(w, r)
	if !ok {
		return
	}
	var res protocol.CommandResult
	if err := decodeJSON(w, r, &res); err != nil || res.ID == "" || !validResult(res) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	// Only the agent the command was sent to can answer it, and only with a result of its kind.
	if !s.Commands.Deliver(agent.ID, res) {
		writeError(w, http.StatusNotFound, "command_not_found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validResult accepts a kill result with an outcome an agent may return, and a pong with the
// agent's version.
func validResult(res protocol.CommandResult) bool {
	switch res.Type {
	case protocol.ResultKill:
		return protocol.IsAgentOutcome(res.Outcome)
	case protocol.ResultPong:
		return res.Version != ""
	}
	return false
}

// killRequest uses pointers so a missing field can be told from a zero value.
type killRequest struct {
	ID        string `json:"id"`
	ServerID  string `json:"server_id"`
	PPID      *int   `json:"ppid"`
	StartTime *int64 `json:"starttime"`
}

// postKill is the app's POST /app/v1/kills. It answers with the agent's outcome, or unknown.
func (s *Server) postKill(w http.ResponseWriter, r *http.Request) {
	device, ok := s.authDevice(w, r)
	if !ok {
		return
	}
	// Every authenticated request counts, failed ones and resends included: the limit bounds how
	// fast anyone can grow an agent's stored outcomes, which never expire.
	if !s.Kills.Allow(device.ID) {
		writeError(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	var req killRequest
	if err := decodeJSON(w, r, &req); err != nil || req.ServerID == "" || req.PPID == nil || !validPID(*req.PPID) || req.StartTime == nil || *req.StartTime < 0 {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !protocol.IsUUIDv4(req.ID) {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	linked, err := s.linked(r.Context(), device.ID, req.ServerID)
	if err != nil {
		s.Log.Error("check link", "device", device.ID, "err", err)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if !linked {
		writeError(w, http.StatusNotFound, "server_not_found")
		return
	}
	o := s.Commands.RelayKill(r.Context(), req.ServerID, protocol.KillCommand{ID: req.ID, PPID: *req.PPID, StartTime: *req.StartTime})
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) linked(ctx context.Context, deviceID, agentID string) (bool, error) {
	links, err := s.Store.LinksOfDevice(ctx, deviceID)
	if err != nil {
		return false, err
	}
	for _, l := range links {
		if l.AgentID == agentID {
			return true, nil
		}
	}
	return false, nil
}
