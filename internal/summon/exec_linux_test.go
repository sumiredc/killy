//go:build linux

package summon

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestMain plays the roles summon gives this binary: ExecStart runs the test binary itself.
func TestMain(m *testing.M) {
	switch os.Getenv(RoleEnv) {
	case RoleParent:
		RunParent()
		os.Exit(0)
	case RoleZombie:
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// stat reads the state and parent PID of pid from /proc; ok is false once the process is gone.
func stat(pid int) (state byte, ppid int, ok bool) {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, 0, false
	}
	rest := string(b[bytes.LastIndexByte(b, ')')+2:])
	f := strings.Fields(rest)
	ppid, _ = strconv.Atoi(f[1])
	return f[0][0], ppid, true
}

// zombiesOf counts the processes in state Z whose parent is pid.
func zombiesOf(pid int) int {
	entries, _ := os.ReadDir("/proc")
	n := 0
	for _, e := range entries {
		child, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if s, pp, ok := stat(child); ok && pp == pid && s == 'Z' {
			n++
		}
	}
	return n
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestARealParentShowsItsCommandLineHoldsZombiesAndIsCollectedWhenKilled(t *testing.T) {
	var pid int
	s := &Summoner{
		Start: func(argv []string) (Process, error) {
			p, err := ExecStart(argv)
			if err == nil {
				pid = p.(*exec.Cmd).Process.Pid
			}
			return p, err
		},
		Pick: func(int) int { return 0 },
		Log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	s.Tick()
	if pid == 0 {
		t.Fatal("no parent was started")
	}
	cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil || string(cmdline) != "node\x00evil_daemon.js\x00" {
		t.Errorf("cmdline %q (%v), want node evil_daemon.js", cmdline, err)
	}
	eventually(t, "the parent holds its first zombies", func() bool { return zombiesOf(pid) >= FirstZombies })
	if st, pp, _ := stat(pid); st == 'Z' || pp != os.Getpid() {
		t.Fatalf("parent state %c, parent %d; want a live child of this test (%d)", st, pp, os.Getpid())
	}

	// The test kills it as the agent would; summon itself never sends a signal.
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the parent is collected", func() bool { return s.Alive() == 0 })
	eventually(t, "the parent is gone from /proc", func() bool {
		st, pp, ok := stat(pid)
		// Collected means no longer a zombie child of the summoner (here, this test).
		return !ok || !(st == 'Z' && pp == os.Getpid())
	})
}
