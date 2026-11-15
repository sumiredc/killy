// Package summon keeps demo parents alive on the demo machine: processes that leave zombie
// children and never collect them, for the public demo to kill.
package summon

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	// MaxParents is how many demo parents are kept alive at once.
	MaxParents = 3
	// Interval is how often a new parent is started when there are fewer than MaxParents.
	Interval = 2 * time.Minute
)

// Commands are the command lines a demo parent shows, so the battle screen reads like a real server.
var Commands = [][]string{
	{"node", "evil_daemon.js"},
	{"python3", "worker.py"},
	{"ruby", "cron_runner.rb"},
	{"php", "queue_worker.php"},
}

// Process is a started parent; Wait returns once it has exited and been collected.
type Process interface{ Wait() error }

// Summoner starts demo parents and collects them when they exit.
type Summoner struct {
	// Start starts one parent showing argv; ExecStart in production.
	Start func(argv []string) (Process, error)
	// Pick picks one of n candidates; math/rand/v2.IntN in production.
	Pick func(n int) int
	Log  *slog.Logger

	mu    sync.Mutex
	alive int
}

// Tick starts one parent when fewer than MaxParents are alive. Each parent is waited for at once,
// so it is collected as soon as it exits: otherwise summon itself would hold a zombie, inside its
// own cgroup, and become a demo enemy that stops the demo when killed.
func (s *Summoner) Tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.alive >= MaxParents {
		return
	}
	argv := Commands[s.Pick(len(Commands))]
	p, err := s.Start(argv)
	if err != nil {
		s.Log.Error("start demo parent", "cmd", strings.Join(argv, " "), "err", err)
		return
	}
	s.alive++
	go func() {
		err := p.Wait()
		s.mu.Lock()
		s.alive--
		s.mu.Unlock()
		s.Log.Info("demo parent exited", "cmd", strings.Join(argv, " "), "err", err)
	}()
}

// Alive is how many parents are alive.
func (s *Summoner) Alive() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.alive
}

// Run calls Tick for each value from ticks until ctx is done.
func (s *Summoner) Run(ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ticks:
			s.Tick()
		case <-ctx.Done():
			return
		}
	}
}
