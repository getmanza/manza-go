package manza_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	manza "github.com/getmanza/manza-go"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"API_KEY", "BASE_URL", "API_VERSION"} {
		t.Setenv("MANZA_"+name, "")
		t.Setenv("ZAZU_"+name, "")
	}
}

func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	t.Cleanup(manza.SetWarnWriter(&buf))
	return &buf
}

func TestEnvPrefersMANZA(t *testing.T) {
	clearEnv(t)
	warnings := captureWarnings(t)
	t.Setenv("MANZA_API_KEY", "new-key")
	t.Setenv("ZAZU_API_KEY", "old-key")
	t.Setenv("MANZA_BASE_URL", "https://new.example")
	t.Setenv("ZAZU_BASE_URL", "https://old.example")

	c, err := manza.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := c.APIKeyForTest(); got != "new-key" {
		t.Fatalf("api key = %q", got)
	}
	if got := c.BaseURLForTest(); got != "https://new.example" {
		t.Fatalf("base url = %q", got)
	}
	if warnings.Len() != 0 {
		t.Fatalf("unexpected warning: %s", warnings)
	}
}

func TestEnvFallsBackToZAZUWithWarning(t *testing.T) {
	cases := []struct{ name, zazu, manza, value string }{
		{"API_KEY", "ZAZU_API_KEY", "MANZA_API_KEY", "old-key"},
		{"BASE_URL", "ZAZU_BASE_URL", "MANZA_BASE_URL", "https://old.example"},
		{"API_VERSION", "ZAZU_API_VERSION", "MANZA_API_VERSION", "2026-01-01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			warnings := captureWarnings(t)
			t.Setenv("MANZA_API_KEY", "k")
			t.Setenv(tc.zazu, tc.value)
			if tc.name == "API_KEY" {
				t.Setenv("MANZA_API_KEY", "")
			}

			c, err := manza.New()
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			got := map[string]string{
				"API_KEY":     c.APIKeyForTest(),
				"BASE_URL":    c.BaseURLForTest(),
				"API_VERSION": c.APIVersionForTest(),
			}[tc.name]
			if got != tc.value {
				t.Fatalf("%s = %q, want %q", tc.name, got, tc.value)
			}
			msg := warnings.String()
			if !strings.Contains(msg, tc.zazu) || !strings.Contains(msg, tc.manza) || !strings.Contains(msg, "deprecated") {
				t.Fatalf("warning does not name %s -> %s: %q", tc.zazu, tc.manza, msg)
			}
		})
	}
}

func TestZAZUWarningIsEmittedOnce(t *testing.T) {
	clearEnv(t)
	warnings := captureWarnings(t)
	t.Setenv("ZAZU_API_KEY", "old-key")

	for i := 0; i < 3; i++ {
		if _, err := manza.New(); err != nil {
			t.Fatalf("New: %v", err)
		}
	}
	if n := strings.Count(warnings.String(), "ZAZU_API_KEY"); n != 1 {
		t.Fatalf("warned %d times, want 1: %q", n, warnings)
	}
}

func TestMissingKeyMessageNamesMANZA(t *testing.T) {
	clearEnv(t)
	_, err := manza.New()
	if err == nil || !strings.Contains(err.Error(), "MANZA_API_KEY") {
		t.Fatalf("error = %v", err)
	}
}

func TestSendsManzaVersionHeaderAndUserAgent(t *testing.T) {
	clearEnv(t)
	var gotUA, gotVersion, gotLegacy string
	server := newHeaderServer(t, func(ua, version, legacy string) { gotUA, gotVersion, gotLegacy = ua, version, legacy })
	c, err := manza.New(manza.WithAPIKey("k"), manza.WithBaseURL(server.URL), manza.WithAPIVersion("2026-01-01"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.Entity.Get(t.Context()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotUA != "manza-go/"+manza.Version {
		t.Fatalf("User-Agent = %q", gotUA)
	}
	if gotVersion != "2026-01-01" || gotLegacy != "" {
		t.Fatalf("Manza-Version = %q, Zazu-Version = %q", gotVersion, gotLegacy)
	}
}

func newHeaderServer(t *testing.T, record func(ua, version, legacy string)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		record(r.Header.Get("User-Agent"), r.Header.Get("Manza-Version"), r.Header.Get("Zazu-Version"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	return server
}
