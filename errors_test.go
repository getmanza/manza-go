package zazu_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	zazu "github.com/getzazu/zazu-go"
)

func TestErrorKindByStatus(t *testing.T) {
	cases := map[int]string{
		400: zazu.KindValidation,
		401: zazu.KindAuthentication,
		403: zazu.KindForbidden,
		404: zazu.KindNotFound,
		409: zazu.KindConflict,
		422: zazu.KindValidation,
		429: zazu.KindRateLimit,
		500: zazu.KindServer,
		418: zazu.KindAPI,
	}
	for status, kind := range cases {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":{"type":"boom","message":"nope","payment_id":"pay_1"}}`))
			}))
			t.Cleanup(server.Close)

			_, err := replayClient(t, server).Entity.Get(context.Background())
			var apiErr *zazu.Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected *zazu.Error, got %T", err)
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

func TestDefaultBaseURL(t *testing.T) {
	var host string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		host = r.URL.Scheme + "://" + r.URL.Host
		return nil, errors.New("stop")
	})
	t.Setenv("ZAZU_BASE_URL", "")
	client, err := zazu.New(zazu.WithAPIKey("k"), zazu.WithHTTPClient(&http.Client{Transport: transport}))
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
