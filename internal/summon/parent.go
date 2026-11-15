package summon

import (
	"context"
	"log/slog"
	"time"
)

// RoleEnv tells a killy process started by summon what to be: RoleParent or RoleZombie. It only
// routes the process in main; the agent never reads it (demo mode checks the cgroup).
const RoleEnv = "KILLY_SUMMON_ROLE"

const (
	RoleParent = "parent"
	RoleZombie = "zombie"
)

const (
	// FirstZombies is the fewest zombies a parent starts with; it starts with up to
	// FirstZombies+ExtraFirstZombies-1.
	FirstZombies      = 2
	ExtraFirstZombies = 3
	// MaxZombies is the most a parent holds: the battle screen's six zombie slots, and at most
	// MaxParents × MaxZombies zombies in the demo machine's process table.
	MaxZombies = 6
	// ZombieInterval is how often a parent adds one more zombie, so the demo shows new zombies
	// arriving during a battle.
	ZombieInterval = 30 * time.Second
)

// parent is a demo parent: it makes zombie children and never waits for them.
type parent struct {
	// Spawn starts one child that exits at once.
	Spawn func() error
	// Pick picks one of n; math/rand/v2.IntN in production.
	Pick func(n int) int
	Log  *slog.Logger
}

// Run starts the first zombies, then adds one for each value from ticks up to MaxZombies, until
// ctx is done. A child that cannot be started is logged and not counted.
func (p *parent) Run(ctx context.Context, ticks <-chan time.Time) {
	held := 0
	spawn := func() {
		if err := p.Spawn(); err != nil {
			p.Log.Error("start zombie", "err", err)
			return
		}
		held++
	}
	for range FirstZombies + p.Pick(ExtraFirstZombies) {
		spawn()
	}
	for {
		select {
		case <-ticks:
			if held < MaxZombies {
				spawn()
			}
		case <-ctx.Done():
			return
		}
	}
}
