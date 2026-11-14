// Package command sends commands to agents over their command streams and waits for the results.
package command

import (
	"context"
	"sync"
	"time"
	"uuid"

	"github.com/sumiredc/killy/internal/clock"
	"github.com/sumiredc/killy/internal/protocol"
)

// KillTimeout is how long a kill waits for the agent's outcome.
const KillTimeout = 10 * time.Second

// PingTimeout is how long a connection check waits for the agent's pong.
const PingTimeout = 5 * time.Second

// CheckInterval is how close together two checks of one agent can be before the later one reuses
// the earlier one's result instead of sending a ping.
const CheckInterval = 2 * time.Second

// CheckResult is the result of a connection check.
type CheckResult struct {
	Result  string // protocol.CheckOK, CheckAgentDisconnected or CheckNoResponse
	Version string // the agent's version, for CheckOK
}

// check is the latest check of one agent: the one a check within CheckInterval reuses.
type check struct {
	started time.Time
	done    chan struct{}
	result  CheckResult
}

// Check sends a ping to the agent and returns ok with the agent's version, agent_disconnected when
// the agent has no stream, or no_response when no pong arrives within PingTimeout. Within
// CheckInterval of the last check of the same agent that was not itself a reuse, it sends nothing
// and returns that check's result, waiting for it when it is still running. ctx only ends this
// caller's wait; the ping keeps waiting for its pong, for a later check to use.
func (r *Router) Check(ctx context.Context, agentID string) CheckResult {
	r.mu.Lock()
	now := r.clock.Now()
	c, recent := r.checks[agentID]
	if !recent || now.After(c.started.Add(CheckInterval)) {
		c = r.startCheck(agentID, now)
	}
	r.mu.Unlock()
	select {
	case <-c.done:
		return c.result
	case <-ctx.Done():
		return CheckResult{Result: protocol.CheckNoResponse}
	}
}

// startCheck records a new check of the agent and sends its ping. r.mu must be held.
func (r *Router) startCheck(agentID string, now time.Time) *check {
	c := &check{started: now, done: make(chan struct{})}
	r.checks[agentID] = c
	end := func(result string) *check {
		c.result = CheckResult{Result: result}
		close(c.done)
		return c
	}
	stream, ok := r.streams[agentID]
	if !ok {
		return end(protocol.CheckAgentDisconnected)
	}
	id := uuid.NewV4().String()
	select {
	case stream <- Command{Event: "ping", Data: protocol.PingCommand{ID: id}}:
	default:
		// The agent is connected but its stream is not taking commands: they are not reaching it.
		return end(protocol.CheckNoResponse)
	}
	key := waitKey{agentID, id}
	w := &wait{kind: protocol.ResultPong, deadline: now.Add(PingTimeout), done: make(chan struct{})}
	r.waits[key] = w
	timer := r.after(PingTimeout)
	go func() {
		select {
		case <-timer:
			r.finish(key, w, func(*wait) {})
		case <-w.done:
		}
		<-w.done
		if w.version == "" {
			c.result = CheckResult{Result: protocol.CheckNoResponse}
		} else {
			c.result = CheckResult{Result: protocol.CheckOK, Version: w.version}
		}
		close(c.done)
	}()
	return c
}

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
	checks  map[string]*check
}

type waitKey struct{ agentID, id string }

// wait is a command waiting for its result. A result is handed only to a wait of its own kind, so
// a pong carrying a kill's ID cannot end the kill without an outcome, nor a kill result a ping.
type wait struct {
	kind     string // protocol.ResultKill or protocol.ResultPong
	deadline time.Time
	done     chan struct{}
	outcome  protocol.Outcome // for a kill
	version  string           // for a ping; empty when no pong came
}

func New(c clock.Clock, after func(time.Duration) <-chan time.Time) *Router {
	return &Router{clock: c, after: after, streams: map[string]chan Command{}, waits: map[waitKey]*wait{}, checks: map[string]*check{}}
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
		w = &wait{kind: protocol.ResultKill, deadline: r.clock.Now().Add(KillTimeout), done: make(chan struct{})}
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
				r.finish(key, w, func(w *wait) { w.outcome = protocol.Outcome{Outcome: protocol.Unknown, Reason: protocol.ReasonTimeout} })
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

// finish ends the wait, setting its result with set, unless it has already ended.
func (r *Router) finish(key waitKey, w *wait, set func(*wait)) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.waits[key] != w {
		return false
	}
	delete(r.waits, key)
	set(w)
	close(w.done)
	return true
}

// Deliver hands a result from agentID to the command of the same kind waiting for it: a kill
// outcome to a kill, a pong to a ping. It reports false when no command of that kind with that ID
// is waiting for that agent, including one whose time is up: a result at exactly the timeout or
// later is too late.
func (r *Router) Deliver(agentID string, res protocol.CommandResult) bool {
	key := waitKey{agentID, res.ID}
	r.mu.Lock()
	w, ok := r.waits[key]
	r.mu.Unlock()
	if !ok || w.kind != res.Type || !r.clock.Now().Before(w.deadline) {
		return false
	}
	return r.finish(key, w, func(w *wait) {
		if w.kind == protocol.ResultPong {
			w.version = res.Version
		} else {
			w.outcome = protocol.Outcome{Outcome: res.Outcome, Reason: res.Reason, Reaped: res.Reaped}
		}
	})
}

// Attached reports whether the agent has a command stream.
func (r *Router) Attached(agentID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.streams[agentID]
	return ok
}
