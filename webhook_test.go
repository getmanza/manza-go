package manza_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	manza "github.com/getmanza/manza-go"
)

// Fixed test vector, shared by every SDK in the family (see
// spec/manza/webhook_spec.rb in manza-ruby). Each verifier must accept exactly
// this delivery. The signature was computed independently with:
//
//	printf '%s' '1767225600.<payload>' | openssl dgst -sha256 -hmac 'whsec_test_vector_secret'
const (
	webhookSecret    = "whsec_test_vector_secret"
	webhookTimestamp = "1767225600"
	webhookPayload   = `{"event":"test.ping","event_id":"0199a1b2-0000-7000-8000-000000000010"}`
	webhookSignature = "970fdd13680015f73dbd172884498c6fd1e07d5b5c41aae081272598607fea75"
)

var webhookNow = time.Unix(1767225600, 0)

func TestSignWebhookSignsTheSharedVector(t *testing.T) {
	if got := manza.SignWebhook([]byte(webhookPayload), webhookTimestamp, webhookSecret); got != webhookSignature {
		t.Fatalf("signature %s", got)
	}
}

func TestVerifyWebhook(t *testing.T) {
	type args struct {
		payload, signature, timestamp, secret string
		opts                                  []manza.WebhookOption
	}
	base := func() args {
		return args{webhookPayload, webhookSignature, webhookTimestamp, webhookSecret, []manza.WebhookOption{manza.WithWebhookNow(webhookNow)}}
	}
	forgedEmpty := manza.SignWebhook([]byte(webhookPayload), webhookTimestamp, "")

	cases := []struct {
		name   string
		mutate func(*args)
		want   bool
	}{
		{"accepts the shared vector", func(*args) {}, true},
		{"rejects a wrong secret", func(a *args) { a.secret = "whsec_other" }, false},
		{"rejects a tampered payload", func(a *args) { a.payload = strings.Replace(a.payload, "ping", "pong", 1) }, false},
		{"rejects a signature of the wrong length", func(a *args) { a.signature = "abc" }, false},
		{"rejects an uppercase signature", func(a *args) { a.signature = strings.ToUpper(a.signature) }, false},
		{"rejects a timestamp 301 seconds old", func(a *args) {
			a.opts = []manza.WebhookOption{manza.WithWebhookNow(webhookNow.Add(301 * time.Second))}
		}, false},
		{"rejects a timestamp 301 seconds in the future", func(a *args) {
			a.opts = []manza.WebhookOption{manza.WithWebhookNow(webhookNow.Add(-301 * time.Second))}
		}, false},
		{"accepts a timestamp exactly at the tolerance", func(a *args) {
			a.opts = []manza.WebhookOption{manza.WithWebhookNow(webhookNow.Add(300 * time.Second))}
		}, true},
		{"honours a custom tolerance", func(a *args) {
			a.opts = []manza.WebhookOption{manza.WithWebhookNow(webhookNow.Add(61 * time.Second)), manza.WithWebhookTolerance(time.Minute)}
		}, false},
		{"skips the timestamp check without a tolerance", func(a *args) {
			a.opts = []manza.WebhookOption{manza.WithWebhookNow(webhookNow.Add(24 * time.Hour)), manza.WithoutWebhookTolerance()}
		}, true},
		{"rejects a non-numeric timestamp", func(a *args) {
			a.timestamp = "yesterday"
			a.opts = []manza.WebhookOption{manza.WithoutWebhookTolerance()}
		}, false},
		{"rejects an empty signature", func(a *args) { a.signature = "" }, false},
		{"rejects an empty timestamp", func(a *args) { a.timestamp = "" }, false},
		{"rejects an empty secret, even with a signature made under it", func(a *args) {
			a.secret = ""
			a.signature = forgedEmpty
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := base()
			tc.mutate(&a)
			if got := manza.VerifyWebhook([]byte(a.payload), a.signature, a.timestamp, a.secret, a.opts...); got != tc.want {
				t.Fatalf("VerifyWebhook = %v, want %v", got, tc.want)
			}
		})
	}
}

func constructEvent(t *testing.T, payload string, header http.Header, secret string, opts ...manza.WebhookOption) (map[string]any, error) {
	t.Helper()
	opts = append([]manza.WebhookOption{manza.WithWebhookNow(webhookNow)}, opts...)
	return manza.ConstructWebhookEvent([]byte(payload), header, secret, opts...)
}

func TestConstructWebhookEventReadsHeaders(t *testing.T) {
	cases := map[string]http.Header{
		"X-Manza-* headers": {"X-Manza-Signature": {webhookSignature}, "X-Manza-Timestamp": {webhookTimestamp}},
		"set through Header.Set": func() http.Header {
			h := http.Header{}
			h.Set("x-manza-signature", webhookSignature)
			h.Set("X-MANZA-TIMESTAMP", webhookTimestamp)
			return h
		}(),
		"mixed-case header names": {"x-MANZA-signature": {webhookSignature}, "X-manza-Timestamp": {webhookTimestamp}},
		"the CGI env form":        {"HTTP_X_MANZA_SIGNATURE": {webhookSignature}, "HTTP_X_MANZA_TIMESTAMP": {webhookTimestamp}},
		"X-Zazu-* fallback":       {"X-Zazu-Signature": {webhookSignature}, "X-Zazu-Timestamp": {webhookTimestamp}},
		"X-Manza-* over X-Zazu-*": {
			"X-Manza-Signature": {webhookSignature}, "X-Manza-Timestamp": {webhookTimestamp},
			"X-Zazu-Signature": {strings.Repeat("0", 64)}, "X-Zazu-Timestamp": {"1"},
		},
	}
	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			event, err := constructEvent(t, webhookPayload, header, webhookSecret)
			if err != nil {
				t.Fatalf("construct: %v", err)
			}
			if event["event"] != "test.ping" || event["event_id"] != "0199a1b2-0000-7000-8000-000000000010" {
				t.Fatalf("unexpected event %#v", event)
			}
		})
	}
}

func TestConstructWebhookEventAcceptsAStaleTimestampWithoutTolerance(t *testing.T) {
	header := http.Header{"X-Manza-Signature": {webhookSignature}, "X-Manza-Timestamp": {webhookTimestamp}}
	event, err := constructEvent(t, webhookPayload, header, webhookSecret,
		manza.WithWebhookNow(webhookNow.Add(301*time.Second)), manza.WithoutWebhookTolerance())
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if event["event"] != "test.ping" {
		t.Fatalf("unexpected event %#v", event)
	}
}

func TestConstructWebhookEventFailures(t *testing.T) {
	signed := func(body, secret string) http.Header {
		return http.Header{
			"X-Manza-Signature": {manza.SignWebhook([]byte(body), webhookTimestamp, secret)},
			"X-Manza-Timestamp": {webhookTimestamp},
		}
	}
	cases := []struct {
		name    string
		payload string
		header  http.Header
		secret  string
		opts    []manza.WebhookOption
		message string
	}{
		{"missing signature header", webhookPayload, http.Header{"X-Manza-Timestamp": {webhookTimestamp}}, webhookSecret, nil,
			"missing X-Manza-Signature header"},
		{"missing timestamp header", webhookPayload, http.Header{"X-Manza-Signature": {webhookSignature}}, webhookSecret, nil,
			"missing X-Manza-Timestamp header"},
		{"empty signature header", webhookPayload, http.Header{"X-Manza-Signature": {""}, "X-Manza-Timestamp": {webhookTimestamp}}, webhookSecret, nil,
			"missing X-Manza-Signature header"},
		{"bad signature", webhookPayload, http.Header{"X-Manza-Signature": {strings.Repeat("0", 64)}, "X-Manza-Timestamp": {webhookTimestamp}}, webhookSecret, nil,
			"invalid webhook signature"},
		{"stale timestamp", webhookPayload, signed(webhookPayload, webhookSecret), webhookSecret,
			[]manza.WebhookOption{manza.WithWebhookNow(webhookNow.Add(301 * time.Second))}, "webhook timestamp outside tolerance"},
		{"body that is not JSON", "not json", signed("not json", webhookSecret), webhookSecret, nil,
			"webhook payload is not JSON"},
		{"empty secret", webhookPayload, signed(webhookPayload, ""), "", nil,
			"invalid webhook signature"},
		{"JSON array", "[]", signed("[]", webhookSecret), webhookSecret, nil, "webhook payload is not a JSON object"},
		{"JSON null", "null", signed("null", webhookSecret), webhookSecret, nil, "webhook payload is not a JSON object"},
		{"JSON number", "42", signed("42", webhookSecret), webhookSecret, nil, "webhook payload is not a JSON object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event, err := constructEvent(t, tc.payload, tc.header, tc.secret, tc.opts...)
			var argErr *manza.ArgumentError
			if !errors.As(err, &argErr) {
				t.Fatalf("expected *manza.ArgumentError, got %T (%v), event %#v", err, err, event)
			}
			if argErr.Message != tc.message {
				t.Fatalf("message %q, want %q", argErr.Message, tc.message)
			}
			if event != nil {
				t.Fatalf("expected no event, got %#v", event)
			}
		})
	}
}
