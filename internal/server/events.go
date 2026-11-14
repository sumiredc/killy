package server

import (
	"context"
	"net/http"
	"time"

	"github.com/sumiredc/killy/internal/protocol"
	"github.com/sumiredc/killy/internal/server/sse"
)

// state is the data of the state event (State in docs/openapi.yaml).
type state struct {
	Servers []serverState `json:"servers"`
}

type serverState struct {
	serverResponse
	Disconnected bool          `json:"disconnected"`
	Parents      []parentState `json:"parents"`
}

type parentState struct {
	protocol.Parent
	FirstDetectedAt time.Time `json:"first_detected_at"`
}

func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	device, ok := s.authDevice(w, r)
	if !ok {
		return
	}
	// Subscribed before the first state is built, so a change in between is not missed.
	changed, cancel := s.Broker.Subscribe(device.ID)
	defer cancel()
	ticks, stop := s.heartbeats()
	defer stop()

	ctx := r.Context()
	stopping := s.stoppingChan()
	st, err := s.stateFor(ctx, device.ID)
	if err != nil {
		s.Log.Error("build state", "device", device.ID, "err", err)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	sw, err := sse.Start(w)
	if err != nil {
		return
	}
	if err := sw.Event("state", st); err != nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-stopping:
			return
		case <-ticks:
			if err := sw.Heartbeat(); err != nil {
				return
			}
		case <-changed:
			st, err := s.stateFor(ctx, device.ID)
			if err != nil {
				// The phone reconnects and gets the full state then.
				s.Log.Error("build state", "device", device.ID, "err", err)
				return
			}
			if err := sw.Event("state", st); err != nil {
				return
			}
		}
	}
}

func (s *Server) heartbeats() (<-chan time.Time, func()) {
	t := time.NewTicker(s.heartbeatInterval())
	return t.C, t.Stop
}

func (s *Server) heartbeatInterval() time.Duration {
	if s.HeartbeatInterval == 0 {
		return sse.HeartbeatInterval
	}
	return s.HeartbeatInterval
}

// StopStreams ends every open event stream. http.Server.Shutdown waits for handlers to return, and
// a stream returns only when its client hangs up, so killy server calls this when it starts to
// shut down. Phones reconnect to the next server.
func (s *Server) StopStreams() {
	stopping := s.stoppingChan()
	s.stopMu.Lock()
	defer s.stopMu.Unlock()
	select {
	case <-stopping:
	default:
		close(stopping)
	}
}

func (s *Server) stoppingChan() chan struct{} {
	s.stopOnce.Do(func() { s.stopping = make(chan struct{}) })
	return s.stopping
}

// ConnectionCheckInterval is how often WatchConnections looks for agents that went quiet.
const ConnectionCheckInterval = time.Second

// WatchConnections calls CheckConnections on every tick until ctx is done.
func (s *Server) WatchConnections(ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			s.CheckConnections(ctx)
		}
	}
}

// stateFor is everything the device may see: its linked servers, each with the device's own name
// and switch, and the latest parents of each. Servers it is not linked to never appear.
func (s *Server) stateFor(ctx context.Context, deviceID string) (state, error) {
	links, err := s.Store.LinksOfDevice(ctx, deviceID)
	if err != nil {
		return state{}, err
	}
	st := state{Servers: make([]serverState, 0, len(links))}
	for _, l := range links {
		sv := serverState{
			serverResponse: serverResponse{ID: l.AgentID, Name: l.Name, Hostname: l.Hostname, Notify: l.Notify},
			Disconnected:   !s.Live.Connected(l.AgentID),
			Parents:        []parentState{},
		}
		if a, ok := s.Live.Agent(l.AgentID); ok {
			for _, p := range a.Parents {
				sv.Parents = append(sv.Parents, parentState{Parent: p.Parent, FirstDetectedAt: p.FirstDetectedAt})
			}
		}
		st.Servers = append(st.Servers, sv)
	}
	return st, nil
}

// NotifyDevice makes the device's open streams send their state again. Claims, renaming, muting
// and removing a server call it.
func (s *Server) NotifyDevice(deviceID string) {
	s.Broker.Notify(deviceID)
}

// notifyAgent makes every device linked to the agent send its state again.
func (s *Server) notifyAgent(ctx context.Context, agentID string) error {
	devices, err := s.Store.DevicesOfAgent(ctx, agentID)
	if err != nil {
		s.Log.Error("list devices of agent", "agent", agentID, "err", err)
		return err
	}
	s.Broker.Notify(devices...)
	return nil
}

// CheckConnections tells the devices of every agent that went quiet. A change whose devices could
// not be listed is tried again on the next check.
func (s *Server) CheckConnections(ctx context.Context) {
	s.Live.ConnectionChanges(func(agentID string) error { return s.notifyAgent(ctx, agentID) })
}
