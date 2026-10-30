package manza

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DefaultWebhookTolerance is how far a delivery's X-Manza-Timestamp may be
// from now before VerifyWebhook and ConstructWebhookEvent refuse it.
const DefaultWebhookTolerance = 300 * time.Second

// WebhookOption configures VerifyWebhook and ConstructWebhookEvent.
type WebhookOption func(*webhookConfig)

type webhookConfig struct {
	tolerance      time.Duration
	checkTimestamp bool
	now            time.Time
}

// WithWebhookTolerance sets how far the timestamp may be from now (default
// DefaultWebhookTolerance).
func WithWebhookTolerance(d time.Duration) WebhookOption {
	return func(c *webhookConfig) { c.tolerance = d; c.checkTimestamp = true }
}

// WithoutWebhookTolerance skips the timestamp check. Use it only to replay a
// captured delivery in a local tool or test, never on a live endpoint:
// without the check, anyone holding a captured delivery can replay it.
func WithoutWebhookTolerance() WebhookOption {
	return func(c *webhookConfig) { c.checkTimestamp = false }
}

// WithWebhookNow sets the time the timestamp is compared against (default
// time.Now()).
func WithWebhookNow(now time.Time) WebhookOption {
	return func(c *webhookConfig) { c.now = now }
}

func newWebhookConfig(opts []WebhookOption) webhookConfig {
	c := webhookConfig{tolerance: DefaultWebhookTolerance, checkTimestamp: true}
	for _, opt := range opts {
		opt(&c)
	}
	if c.now.IsZero() {
		c.now = time.Now()
	}
	return c
}

// SignWebhook returns the lowercase hex HMAC-SHA256 of "<timestamp>.<payload>"
// under secret: the X-Manza-Signature of a delivery. For tests and local tools.
func SignWebhook(payload []byte, timestamp, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyWebhook reports whether signature matches payload and timestamp
// under secret (compared in constant time) and timestamp is within the
// tolerance of now. An empty secret, or a timestamp that is not an integer,
// never verifies.
func VerifyWebhook(payload []byte, signature, timestamp, secret string, opts ...WebhookOption) bool {
	c := newWebhookConfig(opts)
	return webhookSignatureMatches(payload, signature, timestamp, secret) && c.timestampFresh(timestamp)
}

// ConstructWebhookEvent reads X-Manza-Signature and X-Manza-Timestamp from
// header (names in any case, the CGI HTTP_X_MANZA_* form, or the legacy
// X-Zazu-* names; X-Manza-* wins), verifies them, and returns the parsed
// event. It returns *ArgumentError when a header is missing, the signature
// does not match, the timestamp is outside the tolerance, or the body is not
// a JSON object.
//
// Every delivery carries X-Manza-Signature (lowercase hex HMAC-SHA256 of
// "<timestamp>.<raw body>" under the endpoint's signing secret) and
// X-Manza-Timestamp (Unix seconds, fresh on every attempt). Pass the raw
// request body, never re-serialized JSON:
//
//	body, err := io.ReadAll(r.Body)
//	...
//	event, err := manza.ConstructWebhookEvent(body, r.Header, signingSecret)
//	if err != nil { // *manza.ArgumentError
//		w.WriteHeader(http.StatusUnauthorized)
//		return
//	}
//	event["event"] // "payment.authorization_requested"
func ConstructWebhookEvent(payload []byte, header http.Header, secret string, opts ...WebhookOption) (map[string]any, error) {
	c := newWebhookConfig(opts)
	normalized := normalizeWebhookHeader(header)
	signature, err := webhookHeader(normalized, "signature")
	if err != nil {
		return nil, err
	}
	timestamp, err := webhookHeader(normalized, "timestamp")
	if err != nil {
		return nil, err
	}

	if !webhookSignatureMatches(payload, signature, timestamp, secret) {
		return nil, &ArgumentError{Message: "invalid webhook signature"}
	}
	if !c.timestampFresh(timestamp) {
		return nil, &ArgumentError{Message: "webhook timestamp outside tolerance"}
	}

	var parsed any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil, &ArgumentError{Message: "webhook payload is not JSON"}
	}
	event, ok := parsed.(map[string]any)
	if !ok {
		return nil, &ArgumentError{Message: "webhook payload is not a JSON object"}
	}
	return event, nil
}

func webhookSignatureMatches(payload []byte, signature, timestamp, secret string) bool {
	if secret == "" {
		return false
	}
	if _, ok := parseWebhookTimestamp(timestamp); !ok {
		return false
	}
	expected := SignWebhook(payload, timestamp, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

func (c webhookConfig) timestampFresh(timestamp string) bool {
	if !c.checkTimestamp {
		return true
	}
	seconds, ok := parseWebhookTimestamp(timestamp)
	if !ok {
		return false
	}
	age := c.now.Sub(time.Unix(seconds, 0))
	if age < 0 {
		age = -age
	}
	return age <= c.tolerance
}

func parseWebhookTimestamp(timestamp string) (int64, bool) {
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	return seconds, err == nil
}

// normalizeWebhookHeader lowercases names, strips an HTTP_ prefix and turns
// underscores into dashes, so "HTTP_X_MANZA_SIGNATURE", "x-MANZA-signature"
// and the canonical "X-Manza-Signature" all read the same. Names that
// normalize to the same key resolve in sorted order, first wins, so the
// result never depends on map iteration order.
func normalizeWebhookHeader(header http.Header) map[string]string {
	names := make([]string, 0, len(header))
	for name := range header {
		names = append(names, name)
	}
	sort.Strings(names)

	normalized := make(map[string]string, len(header))
	for _, name := range names {
		values := header[name]
		if len(values) == 0 {
			continue
		}
		key := strings.ToLower(name)
		key = strings.ReplaceAll(strings.TrimPrefix(key, "http_"), "_", "-")
		if _, seen := normalized[key]; !seen {
			normalized[key] = values[0]
		}
	}
	return normalized
}

func webhookHeader(normalized map[string]string, suffix string) (string, error) {
	value, ok := normalized["x-manza-"+suffix]
	if !ok {
		value = normalized["x-zazu-"+suffix]
	}
	if value != "" {
		return value, nil
	}
	return "", &ArgumentError{Message: "missing X-Manza-" + strings.ToUpper(suffix[:1]) + suffix[1:] + " header"}
}
