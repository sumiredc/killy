// Package command sends commands to agents over their command streams and waits for the results.
package command

import (
	"context"
	"sync"
	"time"

	"github.com/sumiredc/killy/internal/clock"
	"github.com/sumiredc/killy/internal/protocol"
)

// KillTimeout is how long a kill waits for the agent's outcome.
const KillTimeout = 10 * time.Second

// Command is one event on an agent's command stream.
type Command struct {
	Event string
	Data  any
}

// Router keeps one command stream per agent and the kills waiting for their outcomes. It is safe
// for concurrent use.
type Router struct {
	clock clock.Clock
	// after starts the timer of a kill; time.After in production.
	after func(time.Duration) <-chan time.Time

	mu      sync.Mutex
	streams map[string]chan Command
	waits   map[waitKey]*wait
}

type waitKey struct{ agentID, id string }

type wait struct {
	deadline time.Time
	done     chan struct{}
	outcome  protocol.Outcome
}

func New(c clock.Clock, after func(time.Duration) <-chan time.Time) *Router {
	return &Router{clock: c, after: after, streams: map[string]chan Command{}, waits: map[waitKey]*wait{}}
}

// Attach registers the agent's command stream and returns the commands to write to it. A newer
// stream of the same agent replaces the older one, whose channel is closed. detach must be called
// when the stream ends.
func (r *Router) Attach(agentID string) (cmds <-chan Command, detach func()) {
	ch := make(chan Command, 16)
	r.mu.Lock()
	if old, ok := r.streams[agentID]; ok {
		close(old)
	}
	r.streams[agentID] = ch
	r.mu.Unlock()
	return ch, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		// Only if it is still the agent's stream: a newer one may have replaced and closed it.
		if r.streams[agentID] == ch {
			delete(r.streams, agentID)
			close(ch)
		}
	}
}

// RelayKill sends the kill to the agent and returns its outcome, or unknown with agent_disconnected
// when the agent has no stream, or with timeout when no outcome arrives within KillTimeout. Two
// kills with the same ID for the same agent at once send one command and get the same outcome.
// ctx only ends this caller's wait; the command keeps waiting for its outcome for a resend to join.
func (r *Router) RelayKill(ctx context.Context, agentID string, cmd protocol.KillCommand) protocol.Outcome {
	key := waitKey{agentID, cmd.ID}
	r.mu.Lock()
	w, joined := r.waits[key]
	if !joined {
		stream, ok := r.streams[agentID]
		if !ok {
			r.mu.Unlock()
			return protocol.Outcome{Outcome: protocol.Unknown, Reason: protocol.ReasonAgentDisconnected}
		}
		w = &wait{deadline: r.clock.Now().Add(KillTimeout), done: make(chan struct{})}
		select {
		case stream <- Command{Event: "kill", Data: cmd}:
		default:
			// The stream is not taking commands; it is as good as gone.
			r.mu.Unlock()
			return protocol.Outcome{Outcome: protocol.Unknown, Reason: protocol.ReasonAgentDisconnected}
		}
		r.waits[key] = w
		timer := r.after(KillTimeout)
		go func() {
			select {
			case <-timer:
				r.finish(key, w, protocol.Outcome{Outcome: protocol.Unknown, Reason: protocol.ReasonTimeout})
			case <-w.done:
			}
		}()
	}
	r.mu.Unlock()

	select {
	case <-w.done:
		return w.outcome
	case <-ctx.Done():
		return protocol.Outcome{Outcome: protocol.Unknown, Reason: protocol.ReasonTimeout}
	}
}

// finish ends the wait with o, unless it has already ended.
func (r *Router) finish(key waitKey, w *wait, o protocol.Outcome) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.waits[key] != w {
		return false
	}
	delete(r.waits, key)
	w.outcome = o
	close(w.done)
	return true
}

// Deliver hands a kill outcome from agentID to the kill waiting for it. It reports false when no
// kill with that ID is waiting for that agent, including one whose time is up: an outcome at
// exactly KillTimeout or later is too late.
func (r *Router) Deliver(agentID string, res protocol.CommandResult) bool {
	key := waitKey{agentID, res.ID}
	r.mu.Lock()
	w, ok := r.waits[key]
	r.mu.Unlock()
	if !ok || !r.clock.Now().Before(w.deadline) {
		return false
	}
	return r.finish(key, w, protocol.Outcome{Outcome: res.Outcome, Reason: res.Reason, Reaped: res.Reaped})
}

// Attached reports whether the agent has a command stream.
func (r *Router) Attached(agentID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.streams[agentID]
	return ok
}
