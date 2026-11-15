package proc

import (
	"errors"
	"io/fs"
	"reflect"
	"slices"
	"sync"
	"testing"
	"testing/fstest"
)

// openLog records the files read from a /proc.
type openLog struct {
	fs.FS
	mu     sync.Mutex
	opened []string
}

func (o *openLog) Open(name string) (fs.File, error) {
	o.mu.Lock()
	o.opened = append(o.opened, name)
	o.mu.Unlock()
	return o.FS.Open(name)
}

// demoRoot has one zombie parent in summon's cgroup (100) and parents that are not (200 to 600).
func demoRoot() fstest.MapFS {
	parent := func(pid int, cgroup string) []fakeProc {
		return []fakeProc{
			{pid: pid, comm: "p", state: 'S', ppid: 1, starttime: int64(pid) * 10, cmdline: "secret-cmd\x00", cgroup: cgroup},
			{pid: pid + 1, comm: "z", state: 'Z', ppid: pid, starttime: int64(pid)*10 + 1},
		}
	}
	var ps []fakeProc
	ps = append(ps, parent(100, "0::/system.slice/killy-summon.service\n")...)
	ps = append(ps, parent(200, "0::/system.slice/nginx.service\n")...)
	ps = append(ps, parent(300, "0::/user.slice/user-1000.slice/user@1000.service/app.slice/killy-summon.service\n")...)
	ps = append(ps, parent(400, "0::/system.slice/killy-summon.service.evil\n")...)
	ps = append(ps, parent(500, "1:name=systemd:/system.slice/killy-summon.service\n")...)
	ps = append(ps, parent(600, "")...) // no cgroup file: it cannot be read
	ps = append(ps, parent(700, "0::/user.slice/user-1000.slice/user@1000.service/system.slice/killy-summon.service\n")...)
	return fakeRoot(ps...)
}

func TestScanInDemoModeReportsOnlyParentsInSummonsCgroup(t *testing.T) {
	root := &openLog{FS: demoRoot()}
	parents, err := (&Scanner{Root: root, OnlyCgroup: SummonCgroup}).Scan()
	if err != nil {
		t.Fatal(err)
	}
	var got []int
	for _, p := range parents {
		got = append(got, p.PID)
	}
	if !reflect.DeepEqual(got, []int{100}) {
		t.Errorf("parents %v, want only 100", got)
	}
	// The command lines of other processes on the demo machine are not read, so they cannot leak.
	for _, pid := range []string{"200", "300", "400", "500", "600", "700"} {
		if slices.Contains(root.opened, pid+"/cmdline") {
			t.Errorf("read %s/cmdline of a parent outside summon's cgroup", pid)
		}
	}
}

func TestScanWithoutDemoModeReportsEveryParent(t *testing.T) {
	parents := scan(t, demoRoot())
	var got []int
	for _, p := range parents {
		got = append(got, p.PID)
	}
	if want := []int{100, 200, 300, 400, 500, 600, 700}; !reflect.DeepEqual(got, want) {
		t.Errorf("parents %v, want %v", got, want)
	}
}

func TestInCgroupComparesTheCgroupV2PathExactly(t *testing.T) {
	s := &Scanner{Root: demoRoot()}
	for pid, want := range map[int]bool{100: true, 200: false, 300: false, 400: false, 700: false} {
		in, err := s.InCgroup(pid, SummonCgroup)
		if err != nil || in != want {
			t.Errorf("InCgroup(%d) = %v, %v; want %v", pid, in, err, want)
		}
	}
}

func TestInCgroupTellsAMissingV2LineAndAMissingProcessApart(t *testing.T) {
	s := &Scanner{Root: demoRoot()}
	if _, err := s.InCgroup(500, SummonCgroup); !errors.Is(err, ErrNoUnifiedCgroup) {
		t.Errorf("no 0:: line: err %v, want ErrNoUnifiedCgroup", err)
	}
	if _, err := s.InCgroup(999, SummonCgroup); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("no process: err %v, want fs.ErrNotExist", err)
	}
}

// reusedAfterCmdline is a /proc where process 100 exits after its cgroup is read, and PID 100 is
// reused by a process outside summon's cgroup before its command line is read.
type reusedAfterCmdline struct {
	// Not embedded: fstest.MapFS has ReadFile, which fs.ReadFile would call instead of Open.
	files      fstest.MapFS
	cgroupRead bool
}

func (r *reusedAfterCmdline) Open(name string) (fs.File, error) {
	switch {
	case name == "100/cgroup":
		r.cgroupRead = true
	case r.cgroupRead && name == "100/cmdline":
		return fstest.MapFS{name: {Data: []byte("someone-elses-secret\x00")}}.Open(name)
	case r.cgroupRead && name == "100/stat":
		return fstest.MapFS{name: {Data: []byte(statLine(fakeProc{pid: 100, comm: "other", state: 'S', ppid: 1, starttime: 777}))}}.Open(name)
	}
	return r.files.Open(name)
}

func TestScanInDemoModeDropsAParentWhosePIDWasReusedWhileItWasRead(t *testing.T) {
	root := &reusedAfterCmdline{files: demoRoot()}
	parents, err := (&Scanner{Root: root, OnlyCgroup: SummonCgroup}).Scan()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range parents {
		if p.PID == 100 {
			t.Errorf("reported %+v: the command line read belongs to another process", p)
		}
	}
}
