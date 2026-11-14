package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sumiredc/killy/internal/protocol"
)

// Backoff is the wait before reconnecting the command stream: 1 s, doubling to 30 s, each with up
// to a quarter more at random, so a restart of the Killy server does not bring every agent back at
// the same instant.
type Backoff struct {
	// Rand returns a number in [0, 1); math/rand/v2.Float64 in production.
	Rand    func() float64
	attempt int
}

// Next returns the wait before the next attempt.
func (b *Backoff) Next() time.Duration {
	base := maxBackoff
	if b.attempt < 5 {
		base = time.Second << b.attempt
	}
	b.attempt++
	return base + time.Duration(b.Rand()*float64(base/4))
}

const maxBackoff = 30 * time.Second

// Reset starts again from 1 s, after a connection succeeded.
func (b *Backoff) Reset() { b.attempt = 0 }

// Killer runs a kill command; *kill.Killer in production.
type Killer interface {
	Handle(cmd protocol.KillCommand) (protocol.Outcome, error)
}

// CommandClient keeps the agent's command stream open and answers its kill commands.
type CommandClient struct {
	Server string
	Token  string
	// Client must have no overall timeout: the stream stays open for as long as it can.
	Client *http.Client
	Killer Killer
	// Version is sent with every pong: the agent's own version, set at build time.
	Version string
	Backoff *Backoff
	Log     *slog.Logger
	// Sleep waits between attempts; it returns early when ctx is done.
	Sleep func(ctx context.Context, d time.Duration)
	// IdleTimeout closes a stream that sent nothing, not even a heartbeat, for that long; zero means 60 s.
	IdleTimeout time.Duration

	kills sync.WaitGroup
}

// defaultIdleTimeout is three of the Killy server's 20 s heartbeats.
const defaultIdleTimeout = 60 * time.Second

func (c *CommandClient) idleTimeout() time.Duration {
	if c.IdleTimeout == 0 {
		return defaultIdleTimeout
	}
	return c.IdleTimeout
}

// Run connects, reads commands until the stream ends, and reconnects after Backoff, until ctx is done.
// When ctx is done it still waits for the kills already running, so their outcomes are stored and
// posted: a kill can be between its signal and its record.
func (c *CommandClient) Run(ctx context.Context) {
	defer c.kills.Wait()
	for ctx.Err() == nil {
		if err := c.stream(ctx); err != nil && ctx.Err() == nil {
			c.Log.Error("command stream", "err", err)
		}
		if ctx.Err() != nil {
			return
		}
		c.Sleep(ctx, c.Backoff.Next())
	}
}

// stream reads one connection of the command stream until it ends.
func (c *CommandClient) stream(ctx context.Context) error {
	// A half-open connection delivers nothing, heartbeats included; the idle timer cancels it.
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	idle := time.AfterFunc(c.idleTimeout(), cancel)
	defer idle.Stop()
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, c.Server+"/agent/v1/commands", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "text/event-stream")
	res, err := noRedirects(c.Client).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("the Killy server answered %s", res.Status)
	}
	c.Backoff.Reset()
	c.Log.Info("command stream open")

	sc := bufio.NewScanner(res.Body)
	defer func() {
		if errors.Is(streamCtx.Err(), context.Canceled) && ctx.Err() == nil {
			c.Log.Error("command stream silent; reconnecting", "idle", c.idleTimeout())
		}
	}()
	sc.Buffer(make([]byte, 0, 4096), 64<<10)
	var event, data string
	for sc.Scan() {
		idle.Reset(c.idleTimeout())
		line := sc.Text()
		switch {
		case line == "":
			switch event {
			case "kill":
				var cmd protocol.KillCommand
				if err := json.Unmarshal([]byte(data), &cmd); err != nil {
					c.Log.Error("kill command", "err", err)
				} else {
					// Each kill runs on its own: one can wait up to 5 s for its zombies to go.
					c.kills.Add(1)
					go func() { defer c.kills.Done(); c.kill(ctx, cmd) }()
				}
			case "ping":
				var p protocol.PingCommand
				if err := json.Unmarshal([]byte(data), &p); err != nil || p.ID == "" {
					c.Log.Error("ping without an id", "data", data)
				} else {
					// Not waited for when the agent stops: a pong from a stopping agent means nothing.
					go c.pong(ctx, p.ID)
				}
			}
			event, data = "", ""
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		}
	}
	return sc.Err()
}

func (c *CommandClient) kill(ctx context.Context, cmd protocol.KillCommand) {
	o, err := c.Killer.Handle(cmd)
	if err != nil {
		// The command is ignored: its ID is not a UUID v4. The Killy server times it out.
		c.Log.Error("kill command ignored", "id", cmd.ID, "err", err)
		return
	}
	res := protocol.CommandResult{ID: cmd.ID, Type: protocol.ResultKill, Outcome: o.Outcome, Reaped: o.Reaped, Reason: o.Reason}
	// Posted even when the agent is stopping: the phone is waiting for this outcome.
	if err := c.postResult(context.WithoutCancel(ctx), res); err != nil {
		c.Log.Error("post kill result", "id", cmd.ID, "outcome", o.Outcome, "err", err)
	}
}

// pong answers a connection check. It touches nothing on the server: no Killer, no /proc, no
// record of outcomes.
func (c *CommandClient) pong(ctx context.Context, id string) {
	if err := c.postResult(ctx, protocol.CommandResult{ID: id, Type: protocol.ResultPong, Version: c.Version}); err != nil {
		c.Log.Error("post pong", "id", id, "err", err)
	}
}

func (c *CommandClient) postResult(ctx context.Context, res protocol.CommandResult) error {
	body, err := json.Marshal(res)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Server+"/agent/v1/results", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res2, err := noRedirects(c.Client).Do(req.WithContext(ctx))
	if err != nil {
		return err
	}
	defer res2.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res2.Body, 4096))
	if res2.StatusCode != http.StatusNoContent {
		return fmt.Errorf("the Killy server answered %s", res2.Status)
	}
	return nil
}
