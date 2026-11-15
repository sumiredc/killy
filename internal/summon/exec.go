package summon

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/exec"
	"time"
)

// self is this killy binary. Starting it again keeps the demo machine free of node, python and the
// like, while /proc/<pid>/cmdline still reads as the command line given.
const self = "/proc/self/exe"

// ExecStart starts a demo parent: this same killy binary, showing argv as its command line. It
// inherits summon's cgroup, which is what demo mode checks before a kill.
func ExecStart(argv []string) (Process, error) {
	cmd := &exec.Cmd{Path: self, Args: argv, Env: withRole(RoleParent), Stderr: os.Stderr}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// RunParent is what a demo parent does: make zombies and never collect them. It does not return;
// the parent ends when it is killed or the summon service is stopped.
func RunParent() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t := time.NewTicker(ZombieInterval)
	defer t.Stop()
	p := &Parent{Spawn: spawnZombie, Pick: rand.IntN, Log: log}
	p.Run(context.Background(), t.C)
}

// spawnZombie starts a child that exits at once and is never waited for, so it stays a zombie of
// this parent. Go cannot fork without exec, so the child is this binary again.
func spawnZombie() error {
	cmd := &exec.Cmd{Path: self, Args: os.Args, Env: withRole(RoleZombie)}
	return cmd.Start()
}

// withRole is this process's environment with RoleEnv set to role; os/exec keeps the last value
// of a repeated name.
func withRole(role string) []string {
	return append(os.Environ(), RoleEnv+"="+role)
}
