//go:build linux

package proc

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/protocol"
)

func TestScanFindsARealZombieAndItsParent(t *testing.T) {
	// sh forks "sleep 0" and then turns into "sleep 30", which never waits for that child.
	cmd := exec.Command("sh", "-c", "sleep 0 & exec sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	scanner := &Scanner{Root: os.DirFS("/proc")}
	var found *protocol.Parent
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		parents, err := scanner.Scan()
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		for i := range parents {
			if parents[i].PID == cmd.Process.Pid && len(parents[i].Zombies) == 1 {
				found = &parents[i]
			}
		}
		if found != nil {
			break
		}
	}
	if found == nil {
		t.Fatalf("no parent with PID %d and one zombie within 5 s", cmd.Process.Pid)
	}
	if found.Cmd != "sleep 30" {
		t.Errorf("cmd = %q, want %q", found.Cmd, "sleep 30")
	}
	if found.StartTime <= 0 {
		t.Errorf("starttime = %d, want a positive clock-tick count", found.StartTime)
	}
}
