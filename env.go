package manza

import (
	"fmt"
	"io"
	"os"
	"sync"
)

var (
	deprecationMu sync.Mutex
	warnWriter    io.Writer = os.Stderr
	warned                  = map[string]bool{}
)

// getenv reads the MANZA_<name> variable and falls back to the legacy
// ZAZU_<name> one. The fallback prints a one-time deprecation warning per
// variable and stays supported for all of 1.x.
func getenv(name string) string {
	if v := os.Getenv("MANZA_" + name); v != "" {
		return v
	}
	legacy := "ZAZU_" + name
	v := os.Getenv(legacy)
	if v == "" {
		return ""
	}

	deprecationMu.Lock()
	defer deprecationMu.Unlock()
	if !warned[legacy] {
		warned[legacy] = true
		fmt.Fprintf(warnWriter, "manza: %s is deprecated, use MANZA_%s instead (the ZAZU_* fallback stays for all of 1.x)\n", legacy, name)
	}
	return v
}
