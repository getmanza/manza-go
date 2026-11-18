package manza

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/url"
	"strings"
	"time"
)

// Reconnect backoff for WebhookListenersService.Stream: it doubles from
// streamBackoffInitial up to streamBackoffMax, and resets once a connection
// opens.
var (
	streamBackoffInitial = 500 * time.Millisecond
	streamBackoffMax     = 10 * time.Second
)

// WebhookListenersService opens and closes listen sessions and reads their
// event stream: a copy of every subscribed webhook (and, with Authorizer,
// every transfer-authorization challenge) delivered to a local tool such as
// `manza listen`. The server sends bodies unsigned; sign each one at forward
// time with the session's SigningSecret (WebhookHeaders).
type WebhookListenersService struct{ client *Client }

// WebhookListenerParams is the body of POST /api/webhook_listeners. No
// Events means every subscribable event. Authorizer also mirrors each
// payment.authorization_requested challenge, and needs transfers:authorize
// on the same key.
type WebhookListenerParams struct {
	Events     []string `json:"events"`
	Authorizer bool     `json:"authorizer"`
}

// Create calls POST /api/webhook_listeners. Decode the response onto a
// WebhookListener and pass it to Stream.
func (s *WebhookListenersService) Create(ctx context.Context, params WebhookListenerParams) (*Response, error) {
	if params.Events == nil {
		params.Events = []string{}
	}
	return s.client.post(ctx, "api/webhook_listeners", params)
}

// Delete calls DELETE /api/webhook_listeners/:id, closing the session.
func (s *WebhookListenersService) Delete(ctx context.Context, id string) (*Response, error) {
	return s.client.delete(ctx, encodePath("api/webhook_listeners", id))
}

// WebhookListenerStreamOptions configures Stream. LastEventID resumes after
// a ListenerEvent.StreamID seen earlier (the server keeps about 5 minutes).
type WebhookListenerStreamOptions struct {
	LastEventID string
}

// Stream reads the listener's server-sent event stream and yields one
// ListenerEvent per webhook frame:
//
//	for event, err := range client.WebhookListeners.Stream(ctx, listener, manza.WebhookListenerStreamOptions{}) {
//		if err != nil {
//			return err // the stream has ended
//		}
//		header := manza.WebhookHeaders(listener.SigningSecret, event.EventType, event.EventID, event.ID, event.Body, time.Now())
//		// POST event.Body with header to the local server
//	}
//
// A dropped connection is reopened with Last-Event-ID, backing off from
// 500ms to 10s, so no frame is lost or repeated. The stream ends on a
// listener_closed frame, when ctx is done (without an error), or with one
// yielded error:
//   - *Error for a 4xx other than 429; an expired session answers the
//     reconnect with KindNotFound, so create a new one;
//   - the first connect failing in any way, before any stream opened;
//   - *ArgumentError when listener.StreamURL is not on the client's API host
//     (the API key is never sent elsewhere);
//   - a webhook frame whose data is not the expected JSON.
//
// A 429, a 5xx or a transport error after the stream first opened is retried.
func (s *WebhookListenersService) Stream(ctx context.Context, listener WebhookListener, opts WebhookListenerStreamOptions) iter.Seq2[ListenerEvent, error] {
	return func(yield func(ListenerEvent, error) bool) {
		target, err := s.client.streamTarget(listener.StreamURL)
		if err != nil {
			yield(ListenerEvent{}, err)
			return
		}

		lastID := opts.LastEventID
		backoff := streamBackoffInitial
		opened := false
		for {
			resp, err := s.client.openStream(ctx, target, lastID)
			if ctx.Err() != nil {
				if resp != nil {
					resp.Body.Close()
				}
				return
			}
			if err != nil {
				if !opened || !retryableStreamError(err) {
					yield(ListenerEvent{}, err)
					return
				}
			} else {
				opened = true
				backoff = streamBackoffInitial
				stop := readListenerFrames(bufio.NewReader(resp.Body), &lastID, yield)
				resp.Body.Close()
				if stop || ctx.Err() != nil {
					return
				}
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, streamBackoffMax)
		}
	}
}

// streamTarget resolves a listener's stream_url, relative or absolute,
// against the base URL, refusing any other scheme or host.
func (c *Client) streamTarget(streamURL string) (string, error) {
	if streamURL == "" {
		return "", &ArgumentError{Message: "webhook listener has no stream_url"}
	}
	ref, err := url.Parse(streamURL)
	if err != nil {
		return "", &ArgumentError{Message: "webhook listener stream_url is not a URL: " + err.Error()}
	}
	if !ref.IsAbs() && ref.Host == "" {
		return c.baseURL + "/" + strings.TrimLeft(streamURL, "/"), nil
	}
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", &ArgumentError{Message: "base URL is not a URL: " + err.Error()}
	}
	if !strings.EqualFold(ref.Scheme, base.Scheme) || !strings.EqualFold(ref.Host, base.Host) {
		return "", &ArgumentError{Message: fmt.Sprintf("webhook listener stream_url %s is not on the API host %s", ref.Redacted(), base.Host)}
	}
	return streamURL, nil
}

func retryableStreamError(err error) bool {
	var connErr *ConnectionError
	if errors.As(err, &connErr) {
		return true
	}
	var apiErr *Error
	return errors.As(err, &apiErr) && (apiErr.Kind == KindRateLimit || apiErr.Kind == KindServer)
}

// listenerFrame is the data of a webhook frame. Body is the JSON string of
// the exact bytes to forward.
type listenerFrame struct {
	ID        string `json:"id"`
	EventType string `json:"event_type"`
	EventID   string `json:"event_id"`
	Body      string `json:"body"`
}

// readListenerFrames parses server-sent events from r and yields each
// webhook frame. *lastID advances only once a frame is complete, so a drop
// mid-frame re-delivers it. It reports true when the stream must end (the
// listener closed, the caller stopped, or a malformed frame was yielded)
// and false when the connection dropped and should be reopened.
func readListenerFrames(r *bufio.Reader, lastID *string, yield func(ListenerEvent, error) bool) bool {
	var event, id string
	var data []string
	hasID := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return false // a partial line is dropped with its frame
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")

		if line != "" {
			if strings.HasPrefix(line, ":") {
				continue // a comment: heartbeat
			}
			field, value, _ := strings.Cut(line, ":")
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "event":
				event = value
			case "data":
				data = append(data, value)
			case "id":
				if !strings.ContainsRune(value, 0) {
					id, hasID = value, true
				}
			}
			continue
		}

		// A blank line dispatches the frame. Without its own id line it
		// carries the last one (the SSE last-event-ID rule).
		streamID := *lastID
		if hasID {
			streamID = id
		}
		switch event {
		case "webhook":
			var frame listenerFrame
			if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &frame); err != nil {
				yield(ListenerEvent{}, fmt.Errorf("manza: malformed webhook frame %q: %w", streamID, err))
				return true
			}
			if !yield(ListenerEvent{ID: frame.ID, EventType: frame.EventType, EventID: frame.EventID, Body: []byte(frame.Body), StreamID: streamID}, nil) {
				return true
			}
		case "listener_closed":
			return true
		}
		// Anything else (pgbus:* control frames, unnamed events) is skipped.
		*lastID = streamID
		event, data, hasID = "", nil, false
	}
}
