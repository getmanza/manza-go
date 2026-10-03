package manza_test

import (
	"context"
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
