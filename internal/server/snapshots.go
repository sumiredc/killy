package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/sumiredc/killy/internal/protocol"
	"github.com/sumiredc/killy/internal/server/store"
)

const (
	// maxSnapshotBytes bounds one snapshot: about a million zombie PIDs, with each command line
	// already cut to proc.MaxCmdBytes by the agent.
	maxSnapshotBytes = 8 << 20
	// maxPID is PID_MAX_LIMIT on 64-bit Linux, the highest pid_max can be set to.
	maxPID = 1 << 22
)

// The wire form uses pointers so a missing field can be told from a zero value: openapi.yaml
// requires every one of them, and a start time of 0 is a valid value.
type snapshotWire struct {
	Hostname  *string       `json:"hostname"`
	Version   *string       `json:"version"`
	ScannedAt *time.Time    `json:"scanned_at"`
	Parents   *[]parentWire `json:"parents"`
}

type parentWire struct {
	PID       *int    `json:"pid"`
	StartTime *int64  `json:"starttime"`
	Cmd       *string `json:"cmd"`
	Zombies   *[]int  `json:"zombies"`
}

func (s *Server) postSnapshot(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.authAgent(w, r)
	if !ok {
		return
	}
	var wire snapshotWire
	if err := decodeJSONLimit(w, r, &wire, maxSnapshotBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	snap, ok := wire.snapshot()
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	applied, err := s.Live.Report(r.Context(), agent.ID, snap)
	if err != nil {
		s.Log.Error("record snapshot", "agent", agent.ID, "err", err)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if applied {
		// Not cancelled when the agent hangs up: the snapshot is applied, so the phones must hear of it.
		_ = s.notifyAgent(context.WithoutCancel(r.Context()), agent.ID)
		// Without waiting: Expo being slow must not slow the agent's report.
		if s.Pushes != nil {
			s.Pushes.Wake(agent.ID)
		}
	}
	// A snapshot older than the last one is discarded inside Report and still answered 204, so
	// the agent does not resend it.
	w.WriteHeader(http.StatusNoContent)
}

func (w snapshotWire) snapshot() (protocol.Snapshot, bool) {
	if w.Hostname == nil || *w.Hostname == "" || w.Version == nil || w.ScannedAt == nil || w.ScannedAt.IsZero() || w.Parents == nil {
		return protocol.Snapshot{}, false
	}
	snap := protocol.Snapshot{Hostname: *w.Hostname, Version: *w.Version, ScannedAt: *w.ScannedAt, Parents: make([]protocol.Parent, 0, len(*w.Parents))}
	for _, p := range *w.Parents {
		if p.PID == nil || !validPID(*p.PID) || p.StartTime == nil || *p.StartTime < 0 || p.Cmd == nil || p.Zombies == nil || len(*p.Zombies) == 0 {
			return protocol.Snapshot{}, false
		}
		for _, z := range *p.Zombies {
			if !validPID(z) {
				return protocol.Snapshot{}, false
			}
		}
		snap.Parents = append(snap.Parents, protocol.Parent{PID: *p.PID, StartTime: *p.StartTime, Cmd: *p.Cmd, Zombies: *p.Zombies})
	}
	return snap, true
}

func validPID(pid int) bool { return 0 < pid && pid <= maxPID }

// authAgent finds the agent of the request's bearer token. When it returns false it has already
// written the error response.
func (s *Server) authAgent(w http.ResponseWriter, r *http.Request) (store.Agent, bool) {
	token, ok := bearerToken(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return store.Agent{}, false
	}
	agent, err := s.Store.AgentByToken(r.Context(), hashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return store.Agent{}, false
	}
	if err != nil {
		s.Log.Error("find agent by token", "err", err)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return store.Agent{}, false
	}
	return agent, true
}
