//go:build windows

package session

// flockExclusive is a no-op on Windows. A proper implementation would use
// LockFileEx, but clade does not target Windows today.
func flockExclusive(fd uintptr) error {
	return nil
}

// flockShared is a no-op on Windows.
func flockShared(fd uintptr) error {
	return nil
}

// flockUnlock is a no-op on Windows.
func flockUnlock(fd uintptr) error {
	return nil
}
