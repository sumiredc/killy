//go:build linux

package kill

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/proc"
	"github.com/sumiredc/killy/internal/protocol"
)

func TestPidfdIsAvailableOnThisKernel(t *testing.T) {
	if !Available(NewPidfdOps(), os.Getpid()) {
		t.Fatal("pidfd is not available; the agent needs Linux 5.3 or later without a seccomp filter blocking pidfd_open")
	}
}

// This test needs something to reap the zombie once its parent dies: a normal init, or
// "docker run --init" in a container.
func TestKillKillsARealZombieParent(t *testing.T) {
	// sh forks "sleep 0" and then turns into "sleep 30", which never waits for that child.
	cmd := exec.Command("sh", "-c", "sleep 0 & exec sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill() // cleanup only, if the kill under test did not happen
		<-done
	})

	scanner := &proc.Scanner{Root: os.DirFS("/proc")}
	var parent protocol.Parent
	for deadline := time.Now().Add(5 * time.Second); parent.PID == 0; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the zombie did not appear within 5 s")
		}
		parents, err := scanner.Scan()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range parents {
			if p.PID == cmd.Process.Pid {
				parent = p
			}
		}
	}

	k := &Killer{Proc: scanner, Pidfd: NewPidfdOps(), Store: newMemStore(), Clock: SystemClock{}, SelfPID: os.Getpid}
	got := handle(t, k, protocol.KillCommand{ID: "0f8fad5b-d9cb-469f-a165-70867728950e", PPID: parent.PID, StartTime: parent.StartTime})
	if want := (protocol.Outcome{Outcome: protocol.Killed, Reaped: 1}); got != want {
		t.Fatalf("outcome = %+v, want %+v", got, want)
	}

	select {
	case err := <-done:
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.String() != "signal: killed" {
			t.Errorf("parent ended with %v, want signal: killed", err)
		}
		done <- err
	case <-time.After(5 * time.Second):
		t.Error("the parent did not exit within 5 s")
	}
	for _, z := range parent.Zombies {
		if st, err := scanner.Stat(z); err == nil && st.State == 'Z' && st.PPID == parent.PID {
			t.Errorf("zombie %d is still in the process table", z)
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat(%d): %v", z, err)
		}
	}
}
