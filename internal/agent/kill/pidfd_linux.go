package kill

import "syscall"

// The syscall package predates pidfd and has no constants for it. Since Linux 5.1 new system calls
// have the same number on every architecture Killy ships for (amd64 and arm64).
const (
	sysPidfdSendSignal = 424
	sysPidfdOpen       = 434
)

func rawPidfdOpen(pid int) (uintptr, syscall.Errno) {
	fd, _, errno := syscall.Syscall(sysPidfdOpen, uintptr(pid), 0, 0)
	return fd, errno
}

func rawPidfdSendKill(fd uintptr) syscall.Errno {
	_, _, errno := syscall.Syscall6(sysPidfdSendSignal, fd, uintptr(syscall.SIGKILL), 0, 0, 0, 0)
	return errno
}
