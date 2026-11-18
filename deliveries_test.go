package manza_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	manza "github.com/getmanza/manza-go/v2"
)

// The delivery endpoints have no manza-ruby cassettes yet; these run against
// httptest until they are recorded.

const deliveryJSON = `{"id":"wd_1","endpoint_id":"we_1","event_type":"transfer.executed","event_id":"evt_1","status":"failed",` +
	`"attempts":3,"max_attempts":8,"response_code":500,"response_time_ms":120,"next_attempt_at":"2026-11-18T12:05:00Z",` +
	`"last_attempted_at":"2026-11-18T12:00:00Z","completed_at":null,"created_at":"2026-11-18T11:00:00Z"`

type recordedRequest struct {
	method, target, body string
}

func deliveryClient(t *testing.T, status int, response string) (*manza.Client, *recordedRequest) {
	t.Helper()
	recorded := &recordedRequest{}
	client, _ := listenerClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		recorded.method, recorded.target, recorded.body = r.Method, r.RequestURI, string(raw)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, response)
	})
	return client, recorded
}

func TestWebhookDeliveriesList(t *testing.T) {
	client, recorded := deliveryClient(t, http.StatusOK, `{"data":[`+deliveryJSON+`}],"has_more":true,"next_cursor":"c2"}`)

	page, err := client.WebhookDeliveries.List(context.Background(), "we_1", manza.WebhookDeliveryListParams{
		ListParams: manza.ListParams{Limit: 10, Cursor: "c1"}, Status: "failed",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if recorded.method != http.MethodGet || recorded.target != "/api/webhook_endpoints/we_1/deliveries?cursor=c1&limit=10&status=failed" {
		t.Errorf("request = %s %s", recorded.method, recorded.target)
	}
	if len(page.Data) != 1 || page.Data[0]["id"] != "wd_1" || !page.HasMore || page.NextCursor != "c2" {
		t.Errorf("page = %+v", page)
	}
}

func TestWebhookDeliveriesListWithoutStatus(t *testing.T) {
	client, recorded := deliveryClient(t, http.StatusOK, `{"data":[],"has_more":false,"next_cursor":null}`)

	if _, err := client.WebhookDeliveries.List(context.Background(), "we_1", manza.WebhookDeliveryListParams{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if recorded.target != "/api/webhook_endpoints/we_1/deliveries?limit=100" {
		t.Errorf("request target = %s", recorded.target)
	}
}

func TestWebhookDeliveriesGet(t *testing.T) {
	payload := `{"event":"transfer.executed","data":{"id":"tr_1"}}`
	client, recorded := deliveryClient(t, http.StatusOK, deliveryJSON+`,"payload":`+payload+`,"response_body":"boom"}`)

	resp, err := client.WebhookDeliveries.Get(context.Background(), "wd/1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if recorded.method != http.MethodGet || recorded.target != "/api/webhook_deliveries/wd%2F1" {
		t.Errorf("request = %s %s", recorded.method, recorded.target)
	}

	var delivery manza.WebhookDelivery
	if err := resp.Decode(&delivery); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if delivery.ID != "wd_1" || delivery.EndpointID != "we_1" || delivery.EventType != "transfer.executed" ||
		delivery.EventID != "evt_1" || delivery.Status != "failed" || delivery.Attempts != 3 || delivery.MaxAttempts != 8 ||
		delivery.CreatedAt != "2026-11-18T11:00:00Z" {
		t.Errorf("delivery = %+v", delivery)
	}
	if delivery.ResponseCode == nil || *delivery.ResponseCode != 500 || delivery.ResponseTimeMs == nil || *delivery.ResponseTimeMs != 120 {
		t.Errorf("response code/time = %v/%v", delivery.ResponseCode, delivery.ResponseTimeMs)
	}
	if delivery.NextAttemptAt == nil || *delivery.NextAttemptAt != "2026-11-18T12:05:00Z" ||
		delivery.LastAttemptedAt == nil || *delivery.LastAttemptedAt != "2026-11-18T12:00:00Z" || delivery.CompletedAt != nil {
		t.Errorf("timestamps = %v %v %v", delivery.NextAttemptAt, delivery.LastAttemptedAt, delivery.CompletedAt)
	}
	if string(delivery.Payload) != payload {
		t.Errorf("payload = %s", delivery.Payload)
	}
	if delivery.ResponseBody == nil || *delivery.ResponseBody != "boom" {
		t.Errorf("response_body = %v", delivery.ResponseBody)
	}
}

func TestWebhookDeliveriesRetry(t *testing.T) {
	client, recorded := deliveryClient(t, http.StatusAccepted, `{"delivery_id":"wd_1","status":"queued"}`)

	resp, err := client.WebhookDeliveries.Retry(context.Background(), "wd_1")
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if recorded.method != http.MethodPost || recorded.target != "/api/webhook_deliveries/wd_1/retry" {
		t.Errorf("request = %s %s", recorded.method, recorded.target)
	}
	if resp.Status != http.StatusAccepted || resp.Body["delivery_id"] != "wd_1" || resp.Body["status"] != "queued" {
		t.Errorf("response = %d %v", resp.Status, resp.Body)
	}
}

func TestWebhookDeliveriesRetryRefused(t *testing.T) {
	client, _ := deliveryClient(t, http.StatusUnprocessableEntity, `{"error":{"type":"state_error","message":"Delivery already succeeded"}}`)

	_, err := client.WebhookDeliveries.Retry(context.Background(), "wd_1")
	var apiErr *manza.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T %v, want *manza.Error", err, err)
	}
	if apiErr.Kind != manza.KindValidation || apiErr.Type != "state_error" || apiErr.Message != "Delivery already succeeded" {
		t.Errorf("error = %+v", apiErr)
	}
}

func TestWebhookEndpointsRetryFailed(t *testing.T) {
	client, recorded := deliveryClient(t, http.StatusAccepted, `{"status":"queued","count":4}`)

	resp, err := client.WebhookEndpoints.RetryFailed(context.Background(), "we_1")
	if err != nil {
		t.Fatalf("RetryFailed: %v", err)
	}
	if recorded.method != http.MethodPost || recorded.target != "/api/webhook_endpoints/we_1/retry_failed" {
		t.Errorf("request = %s %s", recorded.method, recorded.target)
	}
	if resp.Status != http.StatusAccepted || resp.Body["status"] != "queued" || resp.Body["count"] != float64(4) {
		t.Errorf("response = %d %v", resp.Status, resp.Body)
	}
}
