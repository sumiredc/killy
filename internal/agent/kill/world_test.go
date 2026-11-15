package kill

import (
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing/fstest"
	"time"

	"github.com/sumiredc/killy/internal/proc"
	"github.com/sumiredc/killy/internal/protocol"
)

// world is a fake Linux: a process table that serves /proc as an fs.FS and pidfds as PidfdOps.
// The kill code under test reads and signals it exactly as it would the real kernel.
type world struct {
	mu     sync.Mutex
	procs  map[int]*proc.Stat
	events []string
	fds    map[Pidfd]identity
	nextFD Pidfd
	// signalled lists every process that received SIGKILL.
	signalled []identity

	openErr error
	sendErr error
	// statErr makes reading <pid>/stat fail with the given error.
	statErr map[int]error
	// keepZombies leaves the zombies of a killed parent in the table; by default they are reaped
	// at once, as init does after they are reparented to it.
	keepZombies bool
	// cgroup is the content of <pid>/cgroup for the processes that have one; cgroupErr makes
	// reading it fail.
	cgroup    map[int]string
	cgroupErr map[int]error
	// afterOpen, beforeSend and beforeCgroup (before each read of a cgroup file) run outside the
	// lock, so they may change the table.
	afterOpen    func()
	beforeSend   func()
	beforeCgroup func()
}

type identity struct {
	pid       int
	starttime int64
}

func newWorld(ps ...proc.Stat) *world {
	w := &world{procs: map[int]*proc.Stat{}, fds: map[Pidfd]identity{}, nextFD: 3, statErr: map[int]error{}, cgroup: map[int]string{}, cgroupErr: map[int]error{}}
	w.add(proc.Stat{PID: 1, Comm: "init", State: 'S', PPID: 0, StartTime: 1})
	for _, p := range ps {
		w.add(p)
	}
	return w
}

func (w *world) add(p proc.Stat) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.procs[p.PID] = &p
}

func (w *world) remove(pid int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.procs, pid)
}

func (w *world) failStat(pid int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.statErr[pid] = err
}

func (w *world) setPPID(pid, ppid int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.procs[pid].PPID = ppid
}

func (w *world) removeZombiesOf(ppid int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for pid, p := range w.procs {
		if p.PPID == ppid && p.State == 'Z' {
			delete(w.procs, pid)
		}
	}
}

func (w *world) eventLog() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.events)
}

func (w *world) signalledProcesses() []identity {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.signalled)
}

func (w *world) sendCount() int {
	n := 0
	for _, e := range w.eventLog() {
		if len(e) >= 9 && e[:9] == "send_kill" {
			n++
		}
	}
	return n
}

// Open serves /proc. Reads of <pid>/stat are logged so tests can check the order of the steps.
func (w *world) Open(name string) (fs.File, error) {
	if strings.HasSuffix(name, "/cgroup") && w.beforeCgroup != nil {
		w.beforeCgroup()
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	root := fstest.MapFS{}
	for _, p := range w.procs {
		root[fmt.Sprintf("%d/stat", p.PID)] = &fstest.MapFile{Data: []byte(statLine(*p))}
		root[fmt.Sprintf("%d/cmdline", p.PID)] = &fstest.MapFile{Data: []byte(p.Comm + "\x00")}
		if c, ok := w.cgroup[p.PID]; ok {
			root[fmt.Sprintf("%d/cgroup", p.PID)] = &fstest.MapFile{Data: []byte(c)}
		}
	}
	if pidText, ok := strings.CutSuffix(name, "/cgroup"); ok {
		w.events = append(w.events, "read "+name)
		if pid, err := strconv.Atoi(pidText); err == nil && w.cgroupErr[pid] != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: w.cgroupErr[pid]}
		}
	}
	if pidText, ok := strings.CutSuffix(name, "/stat"); ok {
		w.events = append(w.events, "read "+name)
		if pid, err := strconv.Atoi(pidText); err == nil && w.statErr[pid] != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: w.statErr[pid]}
		}
	}
	return root.Open(name)
}

func statLine(p proc.Stat) string {
	return fmt.Sprintf("%d (%s) %c %d 1 1 0 -1 4194304 56 0 0 0 0 0 0 0 20 0 1 0 %d 1728512 179 18446744073709551615 0 0 0 0 0 0 0 6 0 1 0 0 17 5 0 0 0 0 0 0 0 0 0 0 0 0 0\n",
		p.PID, p.Comm, p.State, p.PPID, p.StartTime)
}

type worldPidfd struct{ w *world }

func (o worldPidfd) Open(pid int) (Pidfd, error) {
	w := o.w
	w.mu.Lock()
	w.events = append(w.events, fmt.Sprintf("pidfd_open %d", pid))
	if w.openErr != nil {
		w.mu.Unlock()
		return -1, w.openErr
	}
	p, ok := w.procs[pid]
	if !ok {
		w.mu.Unlock()
		return -1, syscall.ESRCH
	}
	fd := w.nextFD
	w.nextFD++
	w.fds[fd] = identity{pid: p.PID, starttime: p.StartTime}
	w.mu.Unlock()
	if w.afterOpen != nil {
		w.afterOpen()
	}
	return fd, nil
}

func (o worldPidfd) SendKill(fd Pidfd) error {
	w := o.w
	if w.beforeSend != nil {
		w.beforeSend()
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	target := w.fds[fd]
	w.events = append(w.events, fmt.Sprintf("send_kill %d", target.pid))
	if w.sendErr != nil {
		return w.sendErr
	}
	// Like the kernel: the pidfd reaches only the process it was opened for, even if the PID now
	// belongs to another process.
	p, ok := w.procs[target.pid]
	if !ok || p.StartTime != target.starttime {
		return syscall.ESRCH
	}
	w.signalled = append(w.signalled, target)
	p.State = 'Z'
	for pid, c := range w.procs {
		if c.PPID == target.pid {
			if c.State == 'Z' && !w.keepZombies {
				delete(w.procs, pid)
			} else {
				c.PPID = 1
			}
		}
	}
	return nil
}

func (o worldPidfd) Close(fd Pidfd) error {
	o.w.mu.Lock()
	defer o.w.mu.Unlock()
	o.w.events = append(o.w.events, fmt.Sprintf("close %d", o.w.fds[fd].pid))
	return nil
}

// fakeClock moves forward only when the code under test sleeps.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	onSleep func(elapsed time.Duration)
	start   time.Time
}

func newFakeClock() *fakeClock {
	t := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	return &fakeClock{now: t, start: t}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Sleep(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	elapsed := c.now.Sub(c.start)
	c.mu.Unlock()
	if c.onSleep != nil {
		c.onSleep(elapsed)
	}
}

// memStore is an OutcomeStore that only keeps records in memory, for tests about other rules.
type memStore struct {
	mu sync.Mutex
	m  map[string]protocol.Outcome
}

func newMemStore() *memStore { return &memStore{m: map[string]protocol.Outcome{}} }

func (s *memStore) Lookup(id string) (protocol.Outcome, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.m[id]
	return o, ok
}

func (s *memStore) Save(id string, o protocol.Outcome, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[id] = o
	return nil
}
