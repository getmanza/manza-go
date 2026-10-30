package transferauthorizer

import "errors"

// FailLockRelease makes every ledger lock release fail until restore is
// called.
func FailLockRelease() (restore func()) {
	lockFile = func(path string) (func() error, error) {
		release, err := acquireLock(path)
		if err != nil {
			return nil, err
		}
		return func() error {
			if err := release(); err != nil {
				return err
			}
			return errors.New("lock file stuck")
		}, nil
	}
	return func() { lockFile = acquireLock }
}
