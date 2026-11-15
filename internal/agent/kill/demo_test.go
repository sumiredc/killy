package kill

import (
	"slices"
	"syscall"
	"testing"

	"github.com/sumiredc/killy/internal/proc"
	"github.com/sumiredc/killy/internal/protocol"
)

// summonCgroup is /proc/<pid>/cgroup of a parent killy-summon.service started, on a cgroup v2 host.
const summonCgroup = "0::/system.slice/killy-summon.service\n"

func demoKiller(w *world) *Killer {
	k := newKiller(w, newFakeClock())
	k.Demo = true
	return k
}

func TestDemoModeRefusesAParentOutsideSummonsCgroup(t *testing.T) {
	for name, content := range map[string]string{
		"another system service":       "0::/system.slice/nginx.service\n",
		"a user unit of the same name": "0::/user.slice/user-1000.slice/user@1000.service/app.slice/killy-summon.service\n",
		// A user may name a slice of their own manager system.slice: the path then ends like summon's.
		"a user unit in a user slice named system.slice": "0::/user.slice/user-1000.slice/user@1000.service/system.slice/killy-summon.service\n",
		"a name with more after it":                      "0::/system.slice/killy-summon.service.evil\n",
		"a cgroup below summon's":                        "0::/system.slice/killy-summon.service/sub\n",
		"the root cgroup":                                "0::/\n",
		"no cgroup v2 line":                              "12:pids:/system.slice/killy-summon.service\n1:name=systemd:/system.slice/killy-summon.service\n",
		"an empty file":                                  "",
	} {
		t.Run(name, func(t *testing.T) {
			w := standardWorld()
			w.cgroup[parentPID] = content
			got := handle(t, demoKiller(w), killCmd(parentPID, parentTime))
			assertOutcome(t, got, protocol.Outcome{Outcome: protocol.Refused, Reason: protocol.ReasonNotDemo})
			assertNoSignal(t, w)
		})
	}
}

func TestDemoModeKillsAParentInSummonsCgroup(t *testing.T) {
	for name, content := range map[string]string{
		"cgroup v2 only":           summonCgroup,
		"with cgroup v1 lines too": "12:pids:/user.slice\n0::/system.slice/killy-summon.service\n1:name=systemd:/user.slice\n",
		"without a final new line": "0::/system.slice/killy-summon.service",
	} {
		t.Run(name, func(t *testing.T) {
			w := standardWorld()
			w.cgroup[parentPID] = content
			got := handle(t, demoKiller(w), killCmd(parentPID, parentTime))
			assertOutcome(t, got, protocol.Outcome{Outcome: protocol.Killed, Reaped: 2})
		})
	}
}

func TestDemoModeRefusesAParentWhoseCgroupCannotBeRead(t *testing.T) {
	w := standardWorld()
	w.cgroup[parentPID] = summonCgroup
	w.cgroupErr[parentPID] = syscall.EACCES
	got := handle(t, demoKiller(w), killCmd(parentPID, parentTime))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.Refused, Reason: protocol.ReasonNotDemo})
	assertNoSignal(t, w)
}

func TestDemoModeIsAlreadyGoneWhenTheParentExitsBeforeItsCgroupIsRead(t *testing.T) {
	for name, exit := range map[string]func(w *world){
		// The process is gone, and with it its /proc directory.
		"the file is gone": func(w *world) { delete(w.procs, parentPID) },
		// The process is exiting: its files are still listed but read as ESRCH.
		"the read gets ESRCH": func(w *world) { w.cgroupErr[parentPID] = syscall.ESRCH },
	} {
		t.Run(name, func(t *testing.T) {
			w := standardWorld()
			w.cgroup[parentPID] = summonCgroup
			w.beforeCgroup = func() {
				w.mu.Lock()
				defer w.mu.Unlock()
				exit(w)
			}
			got := handle(t, demoKiller(w), killCmd(parentPID, parentTime))
			assertOutcome(t, got, protocol.Outcome{Outcome: protocol.AlreadyGone})
			assertNoSignal(t, w)
		})
	}
}

func TestDemoModeReadsTheCgroupAfterOpeningThePidfdAndBeforeTheSignal(t *testing.T) {
	w := standardWorld()
	w.cgroup[parentPID] = summonCgroup
	handle(t, demoKiller(w), killCmd(parentPID, parentTime))
	events := w.eventLog()
	open := slices.Index(events, "pidfd_open 100")
	read := slices.Index(events, "read 100/cgroup")
	send := slices.Index(events, "send_kill 100")
	if open < 0 || read < 0 || send < 0 || !(open < read && read < send) {
		t.Errorf("events %v: want pidfd_open 100, then read 100/cgroup, then send_kill 100", events)
	}
}

func TestDemoModeStillRefusesWhatEveryKillRefuses(t *testing.T) {
	w := standardWorld(proc.Stat{PID: 150, Comm: "orphan", State: 'Z', PPID: 1, StartTime: 6000})
	w.cgroup[1] = summonCgroup
	w.setPPID(agentPID, 0)
	got := handle(t, demoKiller(w), killCmd(1, 1))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.Refused, Reason: protocol.ReasonProtected})
	assertNoSignal(t, w)
}

func TestWithoutDemoModeTheCgroupIsNotRead(t *testing.T) {
	w := standardWorld()
	w.cgroup[parentPID] = "0::/system.slice/nginx.service\n"
	got := handle(t, newKiller(w, newFakeClock()), killCmd(parentPID, parentTime))
	assertOutcome(t, got, protocol.Outcome{Outcome: protocol.Killed, Reaped: 2})
	if slices.Contains(w.eventLog(), "read 100/cgroup") {
		t.Errorf("events %v: a normal agent read the cgroup", w.eventLog())
	}
}
