//go:build unix

package transferauthorizer

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// acquireLock takes an exclusive lock on path, creating it, and returns the
// function that releases it (closing the file drops the lock).
func acquireLock(path string) (func() error, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	// Poll without blocking, like the non-unix lock: give up after 10 s.
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) || time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("lock the ledger with %s: %w (is another creator stuck?)", path, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return f.Close, nil
}
