//go:build !unix

package transferauthorizer

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// lockFile takes the lock by creating path exclusively, waiting up to 10 s
// for another writer to release it, and returns the function that releases
// it (removing the file, retried briefly while another process holds it
// open). A crash leaves the file behind: delete it.
func lockFile(path string) (func() error, error) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			f.Close()
			return func() error { return removeWithRetry(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("ledger is locked by %s: delete it if no creator is running", path)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func removeWithRetry(path string) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if err = os.Remove(path); err == nil || errors.Is(err, os.ErrNotExist) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("release the ledger lock %s: %w (delete it by hand)", path, err)
}
