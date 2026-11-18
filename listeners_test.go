package manza_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	manza "github.com/getmanza/manza-go/v2"
)

// A body that re-marshalling would change: key order, spacing, an escaped
// "<" and a \u escape. Stream must hand back exactly these bytes.
const listenerBody = `{"event":"transfer.executed", "b":1,"a":"<x>","c":"café"}`

func listenerClient(t *testing.T, handler http.HandlerFunc, opts ...manza.Option) (*manza.Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := manza.New(append([]manza.Option{manza.WithAPIKey("test"), manza.WithBaseURL(server.URL)}, opts...)...)
	if err != nil {
		t.Fatalf("build client: %v", err)
	}
	return client, server
}

func fastBackoff(t *testing.T) {
	t.Helper()
	t.Cleanup(manza.SetStreamBackoff(time.Millisecond, 5*time.Millisecond))
}

func streamListener() manza.WebhookListener {
	return manza.WebhookListener{ID: "wl_1", StreamURL: "/api/webhook_listeners/stream?token=signed"}
}

// webhookFrame renders one `event: webhook` SSE frame.
func webhookFrame(id, eventID string) string {
	data := fmt.Sprintf(`{"id":"del_%s","event_type":"transfer.executed","event_id":%q,"body":%q}`, id, eventID, listenerBody)
	return "id: " + id + "\nevent: webhook\ndata: " + data + "\n\n"
}

const closedFrame = "id: 99\nevent: listener_closed\ndata: {}\n\n"

func writeSSE(w http.ResponseWriter, chunks ...string) {
	for _, chunk := range chunks {
		io.WriteString(w, chunk)
	}
	w.(http.Flusher).Flush()
}

func startSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	w.(http.Flusher).Flush()
}

type streamResult struct {
	events []manza.ListenerEvent
	errs   []error
}

func collect(ctx context.Context, client *manza.Client, listener manza.WebhookListener, opts manza.WebhookListenerStreamOptions) streamResult {
	var result streamResult
	for event, err := range client.WebhookListeners.Stream(ctx, listener, opts) {
		if err != nil {
			result.errs = append(result.errs, err)
			continue
		}
		result.events = append(result.events, event)
	}
	return result
}

func collectWithin(t *testing.T, d time.Duration, client *manza.Client, opts manza.WebhookListenerStreamOptions) streamResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	result := collect(ctx, client, streamListener(), opts)
	if ctx.Err() != nil {
		t.Fatalf("stream did not end on its own within %s", d)
	}
	return result
}

func TestWebhookListenersCreate(t *testing.T) {
	var method, path, auth, body string
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		method, path, auth, body = r.Method, r.URL.Path, r.Header.Get("Authorization"), string(raw)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"id":"wl_1","events":["transfer.executed"],"authorizer":true,"expires_at":"2026-11-18T12:00:00Z","stream_url":"https://ma.manza.dev/api/webhook_listeners/stream?token=t","signing_secret":"lsec_1"}`)
	})

	resp, err := client.WebhookListeners.Create(context.Background(), manza.WebhookListenerParams{Events: []string{"transfer.executed"}, Authorizer: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if method != http.MethodPost || path != "/api/webhook_listeners" || auth != "Bearer test" {
		t.Errorf("request = %s %s (Authorization %q)", method, path, auth)
	}
	if !jsonEqual(`{"events":["transfer.executed"],"authorizer":true}`, body) {
		t.Errorf("body = %s", body)
	}
	if resp.Status != http.StatusCreated {
		t.Errorf("status = %d", resp.Status)
	}

	var listener manza.WebhookListener
	if err := resp.Decode(&listener); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	want := manza.WebhookListener{
		ID: "wl_1", Events: []string{"transfer.executed"}, Authorizer: true, ExpiresAt: "2026-11-18T12:00:00Z",
		StreamURL: "https://ma.manza.dev/api/webhook_listeners/stream?token=t", SigningSecret: "lsec_1",
	}
	if fmt.Sprint(listener) != fmt.Sprint(want) {
		t.Errorf("listener = %+v, want %+v", listener, want)
	}
}

// No events means every subscribable event; the body says so with [], never null.
func TestWebhookListenersCreateWithoutEventsSendsAnEmptyList(t *testing.T) {
	var body string
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{}`)
	})

	if _, err := client.WebhookListeners.Create(context.Background(), manza.WebhookListenerParams{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !jsonEqual(`{"events":[],"authorizer":false}`, body) {
		t.Errorf("body = %s", body)
	}
}

func TestWebhookListenersDelete(t *testing.T) {
	var method, target string
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		method, target = r.Method, r.RequestURI
		w.WriteHeader(http.StatusNoContent)
	})

	resp, err := client.WebhookListeners.Delete(context.Background(), "wl/1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if method != http.MethodDelete || target != "/api/webhook_listeners/wl%2F1" {
		t.Errorf("request = %s %s", method, target)
	}
	if resp.Status != http.StatusNoContent {
		t.Errorf("status = %d", resp.Status)
	}
}

func TestStreamYieldsWebhookFramesUntilListenerClosed(t *testing.T) {
	var header http.Header
	var target string
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		header, target = r.Header.Clone(), r.RequestURI
		startSSE(w)
		writeSSE(w,
			": heartbeat\n\n",
			"id: 0\nevent: pgbus:connected\ndata: {}\n\n",
			webhookFrame("1", "evt_1"),
			":\n\n",
			// CRLF line endings parse too.
			"id: 2\r\nevent: webhook\r\ndata: "+fmt.Sprintf(`{"id":"del_2","event_type":"payment.authorization_requested","event_id":"evt_2","body":%q}`, listenerBody)+"\r\n\r\n",
			closedFrame,
			webhookFrame("3", "evt_after_close"),
		)
	})

	result := collectWithin(t, 5*time.Second, client, manza.WebhookListenerStreamOptions{})
	if len(result.errs) != 0 {
		t.Fatalf("errors: %v", result.errs)
	}
	if len(result.events) != 2 {
		t.Fatalf("events = %+v, want 2", result.events)
	}
	first, second := result.events[0], result.events[1]
	if first.ID != "del_1" || first.EventType != "transfer.executed" || first.EventID != "evt_1" || first.StreamID != "1" {
		t.Errorf("first = %+v", first)
	}
	if second.ID != "del_2" || second.EventType != "payment.authorization_requested" || second.EventID != "evt_2" || second.StreamID != "2" {
		t.Errorf("second = %+v", second)
	}
	for _, event := range result.events {
		if string(event.Body) != listenerBody {
			t.Errorf("body = %s, want the exact bytes %s", event.Body, listenerBody)
		}
	}

	if target != "/api/webhook_listeners/stream?token=signed" {
		t.Errorf("request target = %q", target)
	}
	checks := map[string]string{
		"Authorization": "Bearer test",
		"Accept":        "text/event-stream",
		"Cache-Control": "no-cache",
		"User-Agent":    "manza-go/" + manza.Version,
	}
	for name, want := range checks {
		if got := header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if _, ok := header["Last-Event-Id"]; ok {
		t.Errorf("first connect sent Last-Event-ID %q", header.Get("Last-Event-ID"))
	}
}

func TestStreamSendsTheConfiguredAPIVersion(t *testing.T) {
	var version string
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		version = r.Header.Get("Manza-Version")
		startSSE(w)
		writeSSE(w, closedFrame)
	}, manza.WithAPIVersion("2026-11-13"))

	collectWithin(t, 5*time.Second, client, manza.WebhookListenerStreamOptions{})
	if version != "2026-11-13" {
		t.Errorf("Manza-Version = %q", version)
	}
}

// A drop mid-frame: frame 2's id line arrived but not its blank line, so the
// reconnect resumes from frame 1 and frame 2 arrives exactly once.
func TestStreamResumesWithLastEventIDAfterADrop(t *testing.T) {
	fastBackoff(t)
	var connections atomic.Int32
	var resumedFrom string
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		startSSE(w)
		if connections.Add(1) == 1 {
			writeSSE(w, webhookFrame("1", "evt_1"), "id: 2\nevent: webhook\n")
			return
		}
		resumedFrom = r.Header.Get("Last-Event-ID")
		writeSSE(w, webhookFrame("2", "evt_2"), closedFrame)
	})

	result := collectWithin(t, 5*time.Second, client, manza.WebhookListenerStreamOptions{})
	if len(result.errs) != 0 {
		t.Fatalf("errors: %v", result.errs)
	}
	if resumedFrom != "1" {
		t.Errorf("reconnect Last-Event-ID = %q, want 1", resumedFrom)
	}
	if got := eventIDs(result.events); got != "[evt_1 evt_2]" {
		t.Errorf("events = %s", got)
	}
	if connections.Load() != 2 {
		t.Errorf("connections = %d, want 2", connections.Load())
	}
}

func TestStreamResumesFromTheGivenLastEventID(t *testing.T) {
	var resumedFrom string
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		resumedFrom = r.Header.Get("Last-Event-ID")
		startSSE(w)
		writeSSE(w, closedFrame)
	})

	collectWithin(t, 5*time.Second, client, manza.WebhookListenerStreamOptions{LastEventID: "41"})
	if resumedFrom != "41" {
		t.Errorf("Last-Event-ID = %q, want 41", resumedFrom)
	}
}

func TestStreamRetriesAServerErrorAfterADrop(t *testing.T) {
	fastBackoff(t)
	var connections atomic.Int32
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch connections.Add(1) {
		case 1:
			startSSE(w)
			writeSSE(w, webhookFrame("1", "evt_1"))
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			startSSE(w)
			writeSSE(w, webhookFrame("2", "evt_2"), closedFrame)
		}
	})

	result := collectWithin(t, 5*time.Second, client, manza.WebhookListenerStreamOptions{})
	if len(result.errs) != 0 {
		t.Fatalf("errors: %v", result.errs)
	}
	if got := eventIDs(result.events); got != "[evt_1 evt_2]" {
		t.Errorf("events = %s", got)
	}
	if connections.Load() != 3 {
		t.Errorf("connections = %d, want 3", connections.Load())
	}
}

func TestStreamEndsWithAnAuthenticationErrorWithoutRetrying(t *testing.T) {
	fastBackoff(t)
	var connections atomic.Int32
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		connections.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"type":"authentication_error","message":"Invalid API key"}}`)
	})

	result := collectWithin(t, 5*time.Second, client, manza.WebhookListenerStreamOptions{})
	apiErr := onlyAPIError(t, result)
	if apiErr.Kind != manza.KindAuthentication || apiErr.Message != "Invalid API key" {
		t.Errorf("error = %+v", apiErr)
	}
	if connections.Load() != 1 {
		t.Errorf("connections = %d, want 1", connections.Load())
	}
}

// An expired session answers the reconnect with 404: the caller creates a new one.
func TestStreamEndsWithNotFoundWhenTheSessionExpired(t *testing.T) {
	fastBackoff(t)
	var connections atomic.Int32
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if connections.Add(1) == 1 {
			startSSE(w)
			writeSSE(w, webhookFrame("1", "evt_1"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	result := collectWithin(t, 5*time.Second, client, manza.WebhookListenerStreamOptions{})
	if len(result.events) != 1 {
		t.Errorf("events = %+v, want 1", result.events)
	}
	if apiErr := onlyAPIError(t, result); apiErr.Kind != manza.KindNotFound {
		t.Errorf("kind = %s, want not_found", apiErr.Kind)
	}
	if connections.Load() != 2 {
		t.Errorf("connections = %d, want 2", connections.Load())
	}
}

func TestStreamEndsWhenTheFirstConnectFails(t *testing.T) {
	fastBackoff(t)
	var connections atomic.Int32
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		connections.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	})

	result := collectWithin(t, 5*time.Second, client, manza.WebhookListenerStreamOptions{})
	if apiErr := onlyAPIError(t, result); apiErr.Kind != manza.KindServer {
		t.Errorf("kind = %s, want server", apiErr.Kind)
	}
	if connections.Load() != 1 {
		t.Errorf("connections = %d, want 1", connections.Load())
	}
}

func TestStreamEndsWithAConnectionErrorWhenTheServerIsUnreachable(t *testing.T) {
	client, err := manza.New(manza.WithAPIKey("test"), manza.WithBaseURL("http://127.0.0.1:1"))
	if err != nil {
		t.Fatalf("build client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := collect(ctx, client, streamListener(), manza.WebhookListenerStreamOptions{})
	var connErr *manza.ConnectionError
	if len(result.errs) != 1 || !errors.As(result.errs[0], &connErr) {
		t.Fatalf("errors = %v, want one *ConnectionError", result.errs)
	}
}

func TestStreamEndsOnAMalformedWebhookFrame(t *testing.T) {
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		startSSE(w)
		writeSSE(w, "id: 1\nevent: webhook\ndata: {not json\n\n", webhookFrame("2", "evt_2"))
		<-r.Context().Done()
	})

	result := collectWithin(t, 5*time.Second, client, manza.WebhookListenerStreamOptions{})
	if len(result.events) != 0 || len(result.errs) != 1 {
		t.Fatalf("result = %+v, want one error and no events", result)
	}
}

func TestStreamEndsQuietlyWhenTheContextIsCancelled(t *testing.T) {
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		startSSE(w)
		<-r.Context().Done()
	})

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	start := time.Now()
	result := collect(ctx, client, streamListener(), manza.WebhookListenerStreamOptions{})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("stream took %s to end after cancel", elapsed)
	}
	if len(result.events) != 0 || len(result.errs) != 0 {
		t.Errorf("result = %+v, want nothing", result)
	}
}

func TestStreamEndsQuietlyWhenTheContextIsCancelledDuringBackoff(t *testing.T) {
	t.Cleanup(manza.SetStreamBackoff(time.Hour, time.Hour))
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		startSSE(w)
		writeSSE(w, webhookFrame("1", "evt_1"))
	})

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)
	start := time.Now()
	result := collect(ctx, client, streamListener(), manza.WebhookListenerStreamOptions{})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("stream took %s to end after cancel", elapsed)
	}
	if len(result.events) != 1 || len(result.errs) != 0 {
		t.Errorf("result = %+v, want one event and no error", result)
	}
}

// Breaking out of the range closes the connection.
func TestStreamClosesTheConnectionWhenTheCallerStops(t *testing.T) {
	closed := make(chan struct{})
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		startSSE(w)
		writeSSE(w, webhookFrame("1", "evt_1"), webhookFrame("2", "evt_2"))
		<-r.Context().Done()
		close(closed)
	})

	for event, err := range client.WebhookListeners.Stream(context.Background(), streamListener(), manza.WebhookListenerStreamOptions{}) {
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if event.EventID != "evt_1" {
			t.Fatalf("first event = %+v", event)
		}
		break
	}

	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("connection still open after the caller stopped")
	}
}

// The client's 30s default timeout covers reading the body; a stream must
// not be cut by it.
func TestStreamIgnoresTheHTTPClientTimeout(t *testing.T) {
	fastBackoff(t)
	var connections atomic.Int32
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		connections.Add(1)
		startSSE(w)
		writeSSE(w, webhookFrame("1", "evt_1"))
		select {
		case <-time.After(400 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		writeSSE(w, webhookFrame("2", "evt_2"), closedFrame)
	}, manza.WithHTTPClient(&http.Client{Timeout: 200 * time.Millisecond}))

	result := collectWithin(t, 5*time.Second, client, manza.WebhookListenerStreamOptions{})
	if len(result.errs) != 0 {
		t.Fatalf("errors: %v", result.errs)
	}
	if got := eventIDs(result.events); got != "[evt_1 evt_2]" {
		t.Errorf("events = %s", got)
	}
	if connections.Load() != 1 {
		t.Errorf("connections = %d, want 1", connections.Load())
	}
}

func TestStreamResolvesAnAbsoluteStreamURLOnTheAPIHost(t *testing.T) {
	var target string
	client, server := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		target = r.RequestURI
		startSSE(w)
		writeSSE(w, closedFrame)
	})
	listener := manza.WebhookListener{StreamURL: server.URL + "/api/webhook_listeners/stream?token=abs"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if result := collect(ctx, client, listener, manza.WebhookListenerStreamOptions{}); len(result.errs) != 0 {
		t.Fatalf("errors: %v", result.errs)
	}
	if target != "/api/webhook_listeners/stream?token=abs" {
		t.Errorf("request target = %q", target)
	}
}

// The API key is sent only to the configured API host.
func TestStreamRefusesAStreamURLOnAnotherHost(t *testing.T) {
	var connections atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { connections.Add(1) }))
	t.Cleanup(other.Close)
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) { connections.Add(1) })

	for _, streamURL := range []string{other.URL + "/api/webhook_listeners/stream", "", "://bad"} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		result := collect(ctx, client, manza.WebhookListener{StreamURL: streamURL}, manza.WebhookListenerStreamOptions{})
		cancel()
		var argErr *manza.ArgumentError
		if len(result.errs) != 1 || !errors.As(result.errs[0], &argErr) {
			t.Errorf("stream_url %q: errors = %v, want one *ArgumentError", streamURL, result.errs)
		}
	}
	if connections.Load() != 0 {
		t.Errorf("connections = %d, want 0", connections.Load())
	}
}

func eventIDs(events []manza.ListenerEvent) string {
	ids := make([]string, len(events))
	for i, event := range events {
		ids[i] = event.EventID
	}
	return fmt.Sprint(ids)
}

func onlyAPIError(t *testing.T, result streamResult) *manza.Error {
	t.Helper()
	if len(result.errs) != 1 {
		t.Fatalf("errors = %v, want exactly one", result.errs)
	}
	var apiErr *manza.Error
	if !errors.As(result.errs[0], &apiErr) {
		t.Fatalf("error = %T %v, want *manza.Error", result.errs[0], result.errs[0])
	}
	return apiErr
}
