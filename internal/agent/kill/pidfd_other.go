//go:build !linux

package kill

import "syscall"

// Only Linux has pidfd. Elsewhere the agent behaves as on a kernel without it and refuses every kill.
func rawPidfdOpen(int) (uintptr, syscall.Errno) { return 0, syscall.ENOSYS }

func rawPidfdSendKill(uintptr) syscall.Errno { return syscall.ENOSYS }
