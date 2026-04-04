//go:build !windows

package session

import "syscall"

// flockExclusive acquires an exclusive advisory lock on the given file descriptor.
func flockExclusive(fd uintptr) error {
	return syscall.Flock(int(fd), syscall.LOCK_EX)
}

// flockShared acquires a shared (read) advisory lock on the given file descriptor.
func flockShared(fd uintptr) error {
	return syscall.Flock(int(fd), syscall.LOCK_SH)
}

// flockUnlock releases the advisory lock on the given file descriptor.
func flockUnlock(fd uintptr) error {
	return syscall.Flock(int(fd), syscall.LOCK_UN)
}
