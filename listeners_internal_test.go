package manza

import (
	"errors"
	"strings"
	"testing"
)

func TestStreamTargetTreatsTheDefaultPortAsTheSameHost(t *testing.T) {
	client, err := New(WithAPIKey("test"), WithBaseURL("https://api.example"))
	if err != nil {
		t.Fatalf("build client: %v", err)
	}
	for _, streamURL := range []string{"https://api.example:443/s?token=t", "https://API.example/s?token=t"} {
		if target, err := client.streamTarget(streamURL); err != nil || target != streamURL {
			t.Errorf("%s: target %q, err %v", streamURL, target, err)
		}
	}
	if _, err := client.streamTarget("https://api.example:8443/s"); err == nil {
		t.Error("another port was accepted")
	}
}

// The signed token in the query never reaches an error message.
func TestStreamTargetKeepsTheTokenOutOfTheError(t *testing.T) {
	client, err := New(WithAPIKey("test"), WithBaseURL("https://api.example"))
	if err != nil {
		t.Fatalf("build client: %v", err)
	}
	_, err = client.streamTarget("https://other.example/s?token=secret-token")
	var argErr *ArgumentError
	if !errors.As(err, &argErr) {
		t.Fatalf("err = %v, want *ArgumentError", err)
	}
	if strings.Contains(argErr.Message, "secret-token") {
		t.Errorf("message leaks the token: %s", argErr.Message)
	}
}
