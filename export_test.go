package manza

import (
	"io"
	"time"
)

// SetWarnWriter redirects deprecation warnings and re-arms the once-only
// guard. It returns a restore func.
func SetWarnWriter(w io.Writer) func() {
	deprecationMu.Lock()
	defer deprecationMu.Unlock()
	prev := warnWriter
	warnWriter = w
	warned = map[string]bool{}
	return func() {
		deprecationMu.Lock()
		defer deprecationMu.Unlock()
		warnWriter = prev
		warned = map[string]bool{}
	}
}

func (c *Client) APIKeyForTest() string     { return c.apiKey }
func (c *Client) BaseURLForTest() string    { return c.baseURL }
func (c *Client) APIVersionForTest() string { return c.apiVersion }

// SetStreamBackoff shortens Stream's reconnect backoff. It returns a restore func.
func SetStreamBackoff(initial, max time.Duration) func() {
	prevInitial, prevMax := streamBackoffInitial, streamBackoffMax
	streamBackoffInitial, streamBackoffMax = initial, max
	return func() { streamBackoffInitial, streamBackoffMax = prevInitial, prevMax }
}

// StreamBackoffDefaults returns Stream's reconnect backoff bounds.
func StreamBackoffDefaults() (initial, max time.Duration) {
	return streamBackoffInitial, streamBackoffMax
}
