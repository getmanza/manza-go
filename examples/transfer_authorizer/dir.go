package transferauthorizer

import (
	"path/filepath"
	"runtime"
)

// Dir is the example's directory, where .env and ledger.json live, whatever
// the working directory `go run` is called from. It comes from this file's
// compile-time path, so it holds for `go run` and `go build` without
// -trimpath.
func Dir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}
