package manza_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	manza "github.com/getmanza/manza-go"
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
