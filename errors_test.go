package manza_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	manza "github.com/getmanza/manza-go/v2"
)

func TestErrorKindByStatus(t *testing.T) {
	cases := map[int]string{
		400: manza.KindValidation,
		401: manza.KindAuthentication,
		403: manza.KindForbidden,
		404: manza.KindNotFound,
		409: manza.KindConflict,
		422: manza.KindValidation,
		429: manza.KindRateLimit,
		500: manza.KindServer,
		418: manza.KindAPI,
	}
	for status, kind := range cases {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":{"type":"boom","message":"nope","payment_id":"pay_1"}}`))
			}))
			t.Cleanup(server.Close)

			_, err := replayClient(t, server).Entity.Get(context.Background())
			var apiErr *manza.Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected *manza.Error, got %T", err)
			}
			if apiErr.Kind != kind {
				t.Fatalf("status %d: expected kind %q, got %q", status, kind, apiErr.Kind)
			}
			wantPaymentID := ""
			if status == 409 {
				wantPaymentID = "pay_1"
			}
			if apiErr.PaymentID != wantPaymentID {
				t.Fatalf("status %d: expected payment_id %q, got %q", status, wantPaymentID, apiErr.PaymentID)
			}
		})
	}
}

func errorFrom(t *testing.T, status int, body string) *manza.Error {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	_, err := replayClient(t, server).Entity.Get(context.Background())
	var apiErr *manza.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *manza.Error, got %T (%v)", err, err)
	}
	return apiErr
}

// The API's body verbatim (getmanza/app spec/requests/api/invoices_spec.rb).
func TestErrorPlanLimitReachedCarriesKindCurrentAndMax(t *testing.T) {
	apiErr := errorFrom(t, 422, `{"error":{"type":"plan_limit_reached","message":"Plan limit reached for invoices: 10 of 10",`+
		`"kind":"invoices","current":10,"max":10}}`)

	if apiErr.Kind != manza.KindPlanLimitReached || apiErr.Status != 422 || apiErr.Type != "plan_limit_reached" {
		t.Fatalf("expected plan_limit_reached/422/plan_limit_reached, got %s/%d/%s", apiErr.Kind, apiErr.Status, apiErr.Type)
	}
	if apiErr.Message != "Plan limit reached for invoices: 10 of 10" {
		t.Fatalf("unexpected message %q", apiErr.Message)
	}
	if apiErr.LimitKind != "invoices" || apiErr.LimitCurrent != 10 || apiErr.LimitMax != 10 {
		t.Fatalf("expected invoices 10 of 10, got %q %d of %d", apiErr.LimitKind, apiErr.LimitCurrent, apiErr.LimitMax)
	}
}

func TestErrorOtherValidationIsNotPlanLimitReached(t *testing.T) {
	apiErr := errorFrom(t, 422, `{"error":{"message":"Customer not found","type":"invalid_request_error","param":"customer_id"}}`)

	if apiErr.Kind != manza.KindValidation || apiErr.Param != "customer_id" {
		t.Fatalf("expected validation on customer_id, got %s on %q", apiErr.Kind, apiErr.Param)
	}
	if apiErr.LimitKind != "" || apiErr.LimitCurrent != 0 || apiErr.LimitMax != 0 {
		t.Fatalf("expected no plan limit, got %q %d of %d", apiErr.LimitKind, apiErr.LimitCurrent, apiErr.LimitMax)
	}
}

// Only a 422 is a plan cap: the same type on another status keeps its kind.
func TestErrorPlanLimitReachedTypeOnAnotherStatus(t *testing.T) {
	apiErr := errorFrom(t, 500, `{"error":{"type":"plan_limit_reached","message":"boom","kind":"invoices","current":1,"max":1}}`)

	if apiErr.Kind != manza.KindServer || apiErr.LimitKind != "" {
		t.Fatalf("expected a server error without a plan limit, got %s (%q)", apiErr.Kind, apiErr.LimitKind)
	}
}

func TestDefaultBaseURL(t *testing.T) {
	var host string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		host = r.URL.Scheme + "://" + r.URL.Host
		return nil, errors.New("stop")
	})
	t.Setenv("MANZA_BASE_URL", "")
	t.Setenv("ZAZU_BASE_URL", "")
	client, err := manza.New(manza.WithAPIKey("k"), manza.WithHTTPClient(&http.Client{Transport: transport}))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = client.Entity.Get(context.Background())
	if host != "https://ma.manza.finance" {
		t.Fatalf("expected default base URL https://ma.manza.finance, got %s", host)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
