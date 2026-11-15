// Package kill sends SIGKILL to the parent of zombies after checking, right before sending, that it
// is still the process the operator was shown.
package kill

import (
	"errors"
	"io/fs"
	"log/slog"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/sumiredc/killy/internal/proc"
	"github.com/sumiredc/killy/internal/protocol"
)

const (
	reapTimeout  = 5 * time.Second
	reapInterval = 100 * time.Millisecond
	// maxAncestors bounds the walk up the agent's ancestry. /proc is not read atomically, so PIDs
	// reused during the walk could otherwise form a loop.
	maxAncestors = 4096
	// maxPID is PID_MAX_LIMIT on 64-bit Linux, the highest pid_max can be set to.
	maxPID = 1 << 22
)

// Clock is the time source for waiting until the zombies are reaped.
type Clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}

// SystemClock is the wall clock.
type SystemClock struct{}

func (SystemClock) Now() time.Time        { return time.Now() }
func (SystemClock) Sleep(d time.Duration) { time.Sleep(d) }

// ErrInvalidKillID is returned for a command whose ID is not a UUID v4. Such a command is ignored:
// nothing is signalled, stored or reported.
var ErrInvalidKillID = errors.New("kill ID is not a UUID v4")

// Killer runs the kill procedure of docs/ARCHITECTURE.md ("Kill procedure").
type Killer struct {
	Proc    *proc.Scanner
	Pidfd   PidfdOps
	Store   OutcomeStore
	Clock   Clock
	SelfPID func() int
	// Demo, set by killy agent run --demo, kills only parents in proc.SummonCgroup.
	Demo bool
	// Log receives the outcomes that could not be stored. Nil means slog.Default().
	Log *slog.Logger

	ids     keyedMutex[string]
	parents keyedMutex[parentKey]
}

type parentKey struct {
	pid       int
	starttime int64
}

// Handle runs a kill command and returns its outcome. A command whose ID was handled before gets
// the first outcome again without touching any process. Handle is safe to call from several
// goroutines.
func (k *Killer) Handle(cmd protocol.KillCommand) (protocol.Outcome, error) {
	if !protocol.IsUUIDv4(cmd.ID) {
		return protocol.Outcome{}, ErrInvalidKillID
	}
	// A second request with the same ID waits here for the first to store its outcome, then
	// finds it. Without the wait it would find no record yet and run the kill again.
	unlock := k.ids.lock(cmd.ID)
	defer unlock()
	if o, ok := k.Store.Lookup(cmd.ID); ok {
		return o, nil
	}
	o := k.kill(cmd)
	if err := k.Store.Save(cmd.ID, o, k.Clock.Now()); err != nil {
		k.logger().Error("could not store the kill outcome; a repeat of this ID after a restart will be handled as new",
			"id", cmd.ID, "outcome", o.Outcome, "err", err)
	}
	return o, nil
}

func (k *Killer) logger() *slog.Logger {
	if k.Log != nil {
		return k.Log
	}
	return slog.Default()
}

// kill runs steps 2 to 6. Kills of the same parent run one at a time, so the later one sees what
// the earlier one did.
func (k *Killer) kill(cmd protocol.KillCommand) protocol.Outcome {
	// No process can have such a PID. Checked before pidfd_open, which truncates the PID to 32 bits
	// and would open a different process.
	if cmd.PPID <= 0 || cmd.PPID > maxPID {
		return protocol.Outcome{Outcome: protocol.AlreadyGone}
	}
	unlock := k.parents.lock(parentKey{cmd.PPID, cmd.StartTime})
	defer unlock()

	// The pidfd is opened before the checks so that the signal goes to the process that was
	// checked: if it exits and its PID is reused after the checks, the pidfd still refers to the
	// old process and the signal fails with ESRCH instead of reaching the new one.
	fd, err := k.Pidfd.Open(cmd.PPID)
	switch {
	case errors.Is(err, ErrPidfdUnavailable):
		return refused(protocol.ReasonPidfdUnavailable)
	case errors.Is(err, syscall.ESRCH):
		return protocol.Outcome{Outcome: protocol.AlreadyGone}
	case err != nil:
		return signalFailed(err)
	}
	defer k.Pidfd.Close(fd)

	zombies, refusal := k.check(cmd)
	if refusal != nil {
		return *refusal
	}

	if err := k.Pidfd.SendKill(fd); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return protocol.Outcome{Outcome: protocol.AlreadyGone}
		}
		return signalFailed(err)
	}
	return k.waitUntilReaped(zombies)
}

// check reads the target again and returns its zombie children, or the outcome that ends the kill
// without a signal.
func (k *Killer) check(cmd protocol.KillCommand) ([]proc.Stat, *protocol.Outcome) {
	st, err := k.Proc.Stat(cmd.PPID)
	if gone(err) {
		return nil, &protocol.Outcome{Outcome: protocol.AlreadyGone}
	}
	if err != nil {
		o := signalFailed(err)
		return nil, &o
	}
	if st.StartTime != cmd.StartTime {
		o := refused(protocol.ReasonPIDReused)
		return nil, &o
	}
	// The target is itself a zombie: it was killed already and its own parent has not reaped it.
	if st.State == 'Z' {
		return nil, &protocol.Outcome{Outcome: protocol.AlreadyGone}
	}

	children, err := k.Proc.Children(cmd.PPID)
	if err != nil {
		o := signalFailed(err)
		return nil, &o
	}
	var zombies []proc.Stat
	for _, c := range children {
		if c.State == 'Z' {
			zombies = append(zombies, c)
		}
	}
	if len(zombies) == 0 {
		o := refused(protocol.ReasonNotAZombieParent)
		return nil, &o
	}
	if k.protected(cmd.PPID) {
		o := refused(protocol.ReasonProtected)
		return nil, &o
	}
	// The public demo kills only what killy summon started. Only root can move a process into
	// that cgroup, so an ordinary process cannot pass this. A parent whose cgroup cannot be read
	// is not killed: where it came from is not known.
	if k.Demo {
		in, err := k.Proc.InCgroup(cmd.PPID, proc.SummonCgroup)
		switch {
		case gone(err):
			return nil, &protocol.Outcome{Outcome: protocol.AlreadyGone}
		case err != nil || !in:
			o := refused(protocol.ReasonNotDemo)
			return nil, &o
		}
	}
	return zombies, nil
}

// protected reports whether pid is PID 1, the agent, or one of the agent's ancestors. The ancestry
// is read again on every call because the agent can be reparented while it runs. When it cannot be
// read, pid is treated as protected.
func (k *Killer) protected(pid int) bool {
	self := k.SelfPID()
	if pid == 1 || pid == self {
		return true
	}
	cur := self
	for range maxAncestors {
		st, err := k.Proc.Stat(cur)
		if err != nil {
			return true
		}
		if st.PPID == pid {
			return true
		}
		if st.PPID <= 1 {
			return false
		}
		cur = st.PPID
	}
	return true
}

// waitUntilReaped polls until none of zombies is left. A zombie counts as left only while the same
// PID with the same start time is still in state Z, so a PID reused by another process is not counted.
func (k *Killer) waitUntilReaped(zombies []proc.Stat) protocol.Outcome {
	deadline := k.Clock.Now().Add(reapTimeout)
	for {
		left := 0
		for _, z := range zombies {
			st, err := k.Proc.Stat(z.PID)
			if gone(err) {
				continue
			}
			if err != nil || (st.StartTime == z.StartTime && st.State == 'Z') {
				left++
			}
		}
		if left == 0 {
			return protocol.Outcome{Outcome: protocol.Killed, Reaped: len(zombies)}
		}
		now := k.Clock.Now()
		if !now.Before(deadline) {
			return protocol.Outcome{Outcome: protocol.ZombiesRemain}
		}
		k.Clock.Sleep(min(reapInterval, deadline.Sub(now)))
	}
}

// gone reports whether a /proc read failed because the process no longer exists. Reading the
// files of a process that is exiting can fail with ESRCH instead of ENOENT.
func gone(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ESRCH)
}

func refused(reason string) protocol.Outcome {
	return protocol.Outcome{Outcome: protocol.Refused, Reason: reason}
}

func signalFailed(err error) protocol.Outcome {
	return protocol.Outcome{Outcome: protocol.SignalFailed, Reason: errnoName(err)}
}

// errnoName gives the symbolic name the API reports, such as EPERM. The syscall package only has
// the messages ("operation not permitted"), so the errors a kill can meet are named here.
func errnoName(err error) string {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return err.Error()
	}
	switch errno {
	case syscall.EPERM:
		return "EPERM"
	case syscall.ESRCH:
		return "ESRCH"
	case syscall.EINVAL:
		return "EINVAL"
	case syscall.EBADF:
		return "EBADF"
	case syscall.EACCES:
		return "EACCES"
	case syscall.EMFILE:
		return "EMFILE"
	case syscall.ENFILE:
		return "ENFILE"
	case syscall.ENOMEM:
		return "ENOMEM"
	case syscall.ENOSYS:
		return "ENOSYS"
	case syscall.EIO:
		return "EIO"
	default:
		return "errno " + strconv.Itoa(int(errno))
	}
}

// keyedMutex is a mutex per key. A key's entry exists only while someone holds or waits for it.
type keyedMutex[K comparable] struct {
	mu      sync.Mutex
	entries map[K]*keyedEntry
}

type keyedEntry struct {
	sync.Mutex
	users int
}

func (m *keyedMutex[K]) lock(key K) (unlock func()) {
	m.mu.Lock()
	if m.entries == nil {
		m.entries = map[K]*keyedEntry{}
	}
	e := m.entries[key]
	if e == nil {
		e = &keyedEntry{}
		m.entries[key] = e
	}
	e.users++
	m.mu.Unlock()

	e.Lock()
	return func() {
		e.Unlock()
		m.mu.Lock()
		e.users--
		if e.users == 0 {
			delete(m.entries, key)
		}
		m.mu.Unlock()
	}
}
