package manza

import (
	"fmt"
	"net/http"
	"strconv"
)

// Error kinds, mirroring the shared Manza SDK error hierarchy. Together with
// ConfigurationError, ConnectionError and ArgumentError they make up the ten
// classes every SDK exposes.
const (
	KindAuthentication = "authentication" // 401
	KindForbidden      = "forbidden"      // 403
	KindNotFound       = "not_found"      // 404
	KindValidation     = "validation"     // 400, 422
	KindConflict       = "conflict"       // 409
	KindRateLimit      = "rate_limit"     // 429
	KindServer         = "server"         // 5xx
	KindAPI            = "api"            // any other non-2xx
)

// Error is the API error envelope, mirroring the other Manza SDKs' hierarchy:
// { "error": { "type": ..., "message": ..., "param": ..., "payment_id": ... } }.
// Discriminate on Kind (errors.As to *Error, then compare against the Kind*
// constants) instead of matching status codes.
type Error struct {
	Status     int
	Kind       string // one of the Kind* constants
	Type       string // the API's error.type field
	Message    string
	Param      string
	PaymentID  string // only set for conflict: the draft already holding a duplicate client_reference
	RequestID  string
	RetryAfter int // seconds; only set for rate_limit
	Body       map[string]any
}

func (e *Error) Error() string {
	if e.Param != "" {
		return fmt.Sprintf("manza: %s (%d %s, param %s)", e.Message, e.Status, e.Kind, e.Param)
	}
	return fmt.Sprintf("manza: %s (%d %s)", e.Message, e.Status, e.Kind)
}

// ConfigurationError is returned by New when the client can't be built.
type ConfigurationError struct{ Message string }

func (e *ConfigurationError) Error() string { return "manza: " + e.Message }

// ArgumentError is returned when the caller passes a value the SDK refuses to
// send (e.g. a blank signature). Distinct from *Error with KindValidation,
// which is the server rejecting a request. No HTTP call was made.
type ArgumentError struct{ Message string }

func (e *ArgumentError) Error() string { return "manza: " + e.Message }

// ConnectionError wraps transport-level failures (timeouts, DNS, refused).
type ConnectionError struct{ Message string }

func (e *ConnectionError) Error() string { return "manza: connection error: " + e.Message }

func newError(status int, header http.Header, body map[string]any) *Error {
	e := &Error{
		Status:    status,
		Body:      body,
		RequestID: header.Get("X-Request-Id"),
	}

	if payload, ok := body["error"].(map[string]any); ok {
		e.Type, _ = payload["type"].(string)
		e.Message, _ = payload["message"].(string)
		e.Param, _ = payload["param"].(string)
		if status == 409 {
			e.PaymentID, _ = payload["payment_id"].(string)
		}
	}

	switch {
	case status == 401:
		e.Kind = KindAuthentication
	case status == 403:
		e.Kind = KindForbidden
	case status == 404:
		e.Kind = KindNotFound
	case status == 400, status == 422:
		e.Kind = KindValidation
	case status == 409:
		e.Kind = KindConflict
	case status == 429:
		e.Kind = KindRateLimit
		if retry := header.Get("Retry-After"); retry != "" {
			e.RetryAfter, _ = strconv.Atoi(retry)
		}
	case status >= 500:
		e.Kind = KindServer
	default:
		e.Kind = KindAPI
	}

	if e.Message == "" {
		e.Message = http.StatusText(status)
	}
	return e
}
