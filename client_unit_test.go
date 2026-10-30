package manza_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	manza "github.com/getmanza/manza-go"
)

func TestNewRequiresAPIKey(t *testing.T) {
	t.Setenv("MANZA_API_KEY", "")
	t.Setenv("ZAZU_API_KEY", "")

	_, err := manza.New()
	if err == nil {
		t.Fatal("expected configuration error without an API key")
	}
	if _, ok := err.(*manza.ConfigurationError); !ok {
		t.Fatalf("expected *manza.ConfigurationError, got %T", err)
	}
}

func TestListLimitValidation(t *testing.T) {
	client, err := manza.New(manza.WithAPIKey("test"), manza.WithBaseURL("http://127.0.0.1:1"))
	if err != nil {
		t.Fatalf("build client: %v", err)
	}

	_, err = client.Beneficiaries.List(context.Background(), manza.ListParams{Limit: manza.MaxPerPage + 1})
	if err == nil {
		t.Fatal("expected limit validation error")
	}
}

// The raw request target, not the decoded URL.Path: the base path keeps its
// "/" and only the id is escaped, inside its own segment.
func TestByIDRequestTarget(t *testing.T) {
	var target string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target = r.RequestURI
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{}")
	}))
	t.Cleanup(server.Close)

	client, err := manza.New(manza.WithAPIKey("test"), manza.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("build client: %v", err)
	}

	if _, err := client.Payments.Get(context.Background(), "abc"); err != nil {
		t.Fatalf("Payments.Get: %v", err)
	}
	if target != "/api/payments/abc" {
		t.Errorf("Payments.Get request target = %q, want %q", target, "/api/payments/abc")
	}

	if _, err := client.Customers.Get(context.Background(), "a/b"); err != nil {
		t.Fatalf("Customers.Get: %v", err)
	}
	if target != "/api/customers/a%2Fb" {
		t.Errorf("Customers.Get request target = %q, want %q", target, "/api/customers/a%2Fb")
	}
}
