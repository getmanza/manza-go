//go:build unix

package transferauthorizer

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive lock on path, creating it, and returns the
// function that releases it (closing the file drops the lock).
func lockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil
}
