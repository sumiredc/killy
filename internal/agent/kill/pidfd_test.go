package kill

import (
	"errors"
	"syscall"
	"testing"

	"github.com/sumiredc/killy/internal/proc"
	"github.com/sumiredc/killy/internal/protocol"
)

// fakeSys stands in for the raw system calls under sysOps.
type fakeSys struct {
	openErrno syscall.Errno
	sendErrno syscall.Errno
	sends     int
	closes    int
}

func (f *fakeSys) ops() sysOps {
	return sysOps{
		open: func(int) (uintptr, syscall.Errno) {
			if f.openErrno != 0 {
				return ^uintptr(0), f.openErrno
			}
			return 42, 0
		},
		sendKill: func(uintptr) syscall.Errno { f.sends++; return f.sendErrno },
		close:    func(uintptr) error { f.closes++; return nil },
	}
}

var unavailableErrnos = []syscall.Errno{syscall.ENOSYS, syscall.EPERM}

func TestOpenReportsPidfdUnavailableForENOSYSAndEPERM(t *testing.T) {
	for _, errno := range unavailableErrnos {
		t.Run(errno.Error(), func(t *testing.T) {
			_, err := (&fakeSys{openErrno: errno}).ops().Open(100)
			if !errors.Is(err, ErrPidfdUnavailable) {
				t.Errorf("err = %v, want ErrPidfdUnavailable", err)
			}
		})
	}
}

func TestOpenReturnsESRCHForAMissingProcess(t *testing.T) {
	_, err := (&fakeSys{openErrno: syscall.ESRCH}).ops().Open(100)
	if !errors.Is(err, syscall.ESRCH) || errors.Is(err, ErrPidfdUnavailable) {
		t.Errorf("err = %v, want ESRCH", err)
	}
}

func TestOpenReturnsTheDescriptor(t *testing.T) {
	fd, err := (&fakeSys{}).ops().Open(100)
	if err != nil || fd != 42 {
		t.Errorf("Open = %d, %v, want 42, nil", fd, err)
	}
}

func TestSendKillReturnsTheErrno(t *testing.T) {
	err := (&fakeSys{sendErrno: syscall.ESRCH}).ops().SendKill(42)
	if !errors.Is(err, syscall.ESRCH) {
		t.Errorf("err = %v, want ESRCH", err)
	}
}

func TestAvailableIsFalseWhenPidfdOpenFailsWithENOSYSOrEPERM(t *testing.T) {
	for _, errno := range unavailableErrnos {
		t.Run(errno.Error(), func(t *testing.T) {
			if Available((&fakeSys{openErrno: errno}).ops(), 1) {
				t.Error("Available = true, want false")
			}
		})
	}
}

func TestAvailableIsTrueAndClosesTheProbeWhenPidfdOpenWorks(t *testing.T) {
	sys := &fakeSys{}
	if !Available(sys.ops(), 1) {
		t.Error("Available = false, want true")
	}
	if sys.closes != 1 {
		t.Errorf("closed %d descriptors, want 1", sys.closes)
	}
}

func TestKillWithoutPidfdIsRefusedAndSendsNoSignal(t *testing.T) {
	for _, errno := range unavailableErrnos {
		t.Run(errno.Error(), func(t *testing.T) {
			w := standardWorld()
			sys := &fakeSys{openErrno: errno}
			k := &Killer{Proc: &proc.Scanner{Root: w}, Pidfd: sys.ops(), Clock: newFakeClock(), SelfPID: func() int { return agentPID }}
			got := k.Handle(killCmd(parentPID, parentTime))
			assertOutcome(t, got, protocol.Outcome{Outcome: protocol.Refused, Reason: protocol.ReasonPidfdUnavailable})
			if sys.sends != 0 {
				t.Errorf("pidfd_send_signal called %d times, want 0", sys.sends)
			}
		})
	}
}
