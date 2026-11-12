package kill

import (
	"errors"
	"syscall"
)

// Pidfd is a file descriptor that refers to one process. It keeps referring to that process even
// after the process exits and its PID is given to another one.
type Pidfd int

// ErrPidfdUnavailable means the kernel has no pidfd (Linux older than 5.3) or a seccomp filter blocks it.
var ErrPidfdUnavailable = errors.New("pidfd is not available")

// PidfdOps is the only way the kill code reaches a process.
type PidfdOps interface {
	// Open returns ErrPidfdUnavailable when pidfd cannot be used and syscall.ESRCH when pid does not exist.
	Open(pid int) (Pidfd, error)
	// SendKill sends SIGKILL, the only signal Killy sends. It returns syscall.ESRCH when the process has exited.
	SendKill(fd Pidfd) error
	Close(fd Pidfd) error
}

// sysOps turns the raw pidfd system calls into PidfdOps. The raw calls are fields so tests can make
// pidfd_open fail the way an old kernel or a seccomp filter does.
type sysOps struct {
	open     func(pid int) (uintptr, syscall.Errno)
	sendKill func(fd uintptr) syscall.Errno
	close    func(fd uintptr) error
}

func (o sysOps) Open(pid int) (Pidfd, error) {
	fd, errno := o.open(pid)
	switch errno {
	case 0:
		return Pidfd(fd), nil
	case syscall.ENOSYS, syscall.EPERM:
		return -1, ErrPidfdUnavailable
	default:
		return -1, errno
	}
}

func (o sysOps) SendKill(fd Pidfd) error {
	if errno := o.sendKill(uintptr(fd)); errno != 0 {
		return errno
	}
	return nil
}

func (o sysOps) Close(fd Pidfd) error {
	return o.close(uintptr(fd))
}

// NewPidfdOps returns the PidfdOps backed by the kernel.
func NewPidfdOps() PidfdOps {
	return sysOps{open: rawPidfdOpen, sendKill: rawPidfdSendKill, close: func(fd uintptr) error { return syscall.Close(int(fd)) }}
}

// Available reports whether ops can open a pidfd, by opening one for pid (normally the agent itself).
func Available(ops PidfdOps, pid int) bool {
	fd, err := ops.Open(pid)
	if errors.Is(err, ErrPidfdUnavailable) {
		return false
	}
	if err == nil {
		_ = ops.Close(fd)
	}
	return true
}
