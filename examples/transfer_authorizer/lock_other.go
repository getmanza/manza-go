//go:build !unix

package transferauthorizer

// lockFile is a no-op where flock is unavailable: run one creator at a time.
func lockFile(string) (func(), error) { return func() {}, nil }
