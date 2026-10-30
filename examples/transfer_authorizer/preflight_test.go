package transferauthorizer_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	transferauthorizer "github.com/getmanza/manza-go/examples/transfer_authorizer"
)

// Drives the example's preflight against a stub of the documented
// responses. No cassette, no network: each failure is a stubbed response.
// The same cases as manza-ruby's spec/examples/preflight_spec.rb.

const (
	accountID     = "01958e51-7c5a-7000-8000-000000000013"
	beneficiaryID = "01958e51-7c5a-7000-8000-000000000015"
	endpointID    = "01958e51-7c5a-7000-8000-000000000020"
)

type preflightStub struct {
	env        map[string]string
	authorizer map[string]any
	endpoint   map[string]any
	payee      map[string]any
	currency   string
	// overrides answers a path with a fixed status and body instead.
	overrides map[string]stubResponse

	mu       sync.Mutex
	requests int
}

func newPreflightStub() *preflightStub {
	return &preflightStub{
		env: map[string]string{
			"MANZA_CREATOR_API_KEY":    "key-a",
			"MANZA_AUTHORIZER_API_KEY": "key-b",
			"MANZA_AUTHORIZER_SECRET":  "whsec_example",
			"MANZA_ACCOUNT_ID":         accountID,
			"MANZA_BENEFICIARY_ID":     beneficiaryID,
		},
		authorizer: map[string]any{
			"status": "active", "suspension_reason": nil, "currency_code": "MAD", "per_transfer_limit": "1000.0",
			"daily_limit": "5000.0", "daily_authorized_total": "100.0", "secret_expires_at": "2027-01-18T00:00:00Z",
			"webhook_endpoint_id": endpointID, "enrolled_at": "2026-10-01T08:30:00Z",
		},
		endpoint:  map[string]any{"id": endpointID, "url": "https://authorizer.example.com/", "status": "active"},
		payee:     map[string]any{"id": "01958e51-7c5a-7000-8000-000000000014", "default": true, "trusted": true},
		currency:  "MAD",
		overrides: map[string]stubResponse{},
	}
}

func (s *preflightStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests++
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if override, ok := s.overrides[r.URL.Path]; ok {
		w.WriteHeader(override.status)
		w.Write([]byte(override.body))
		return
	}
	// Each read must come with the right key.
	keys := map[string]string{
		"/api/entity":                          "key-a",
		"/api/transfer_authorizer":             "key-b",
		"/api/accounts/" + accountID:           "key-a",
		"/api/webhook_endpoints/" + endpointID: "key-a",
		"/api/beneficiaries/" + beneficiaryID:  "key-a",
	}
	bodies := map[string]any{
		"/api/entity":                          map[string]any{"id": "e1", "name": "Acme"},
		"/api/transfer_authorizer":             s.authorizer,
		"/api/accounts/" + accountID:           map[string]any{"id": accountID, "currency_code": s.currency},
		"/api/webhook_endpoints/" + endpointID: s.endpoint,
		"/api/beneficiaries/" + beneficiaryID:  map[string]any{"id": beneficiaryID, "external_accounts": []any{s.payee}},
	}
	key, known := keys[r.URL.Path]
	if !known || r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer "+key {
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte(`{"error":{"type":"unexpected_request","message":"` + r.Method + " " + r.URL.Path + `"}}`))
		return
	}
	json.NewEncoder(w).Encode(bodies[r.URL.Path])
}

func apiError(status int, errorType string) stubResponse {
	return stubResponse{status: status, body: `{"error":{"type":"` + errorType + `","message":"refused"}}`}
}

func (s *preflightStub) run(t *testing.T, amount string) (bool, string) {
	t.Helper()
	server := httptest.NewServer(s)
	t.Cleanup(server.Close)
	s.env["MANZA_BASE_URL"] = server.URL
	out := &bytes.Buffer{}
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	ok := transferauthorizer.NewPreflight(func(name string) string { return s.env[name] }, amount, out, now).Run(context.Background())
	return ok, out.String()
}

func linesWith(output, prefix string) []string {
	var out []string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			out = append(out, line)
		}
	}
	return out
}

func failures(output string) []string { return linesWith(output, "✗") }

func expectFirstFailure(t *testing.T, output string, fragments ...string) {
	t.Helper()
	got := failures(output)
	if len(got) == 0 {
		t.Fatalf("expected a failure:\n%s", output)
	}
	for _, fragment := range fragments {
		if !strings.Contains(got[0], fragment) {
			t.Fatalf("expected %q in the first failure %q:\n%s", fragment, got[0], output)
		}
	}
}

func TestPreflightPassesEveryStepWhenTheSetupIsComplete(t *testing.T) {
	ok, output := newPreflightStub().run(t, "10.00")

	if !ok || len(failures(output)) != 0 {
		t.Fatalf("expected success:\n%s", output)
	}
	if n := len(linesWith(output, "✓")); n != 9 {
		t.Fatalf("expected 9 passed steps, got %d:\n%s", n, output)
	}
}

func TestPreflightStopsBeforeAnyRequestOnAMissingSetting(t *testing.T) {
	for _, name := range []string{"MANZA_AUTHORIZER_API_KEY", "MANZA_BENEFICIARY_ID"} {
		t.Run(name, func(t *testing.T) {
			stub := newPreflightStub()
			stub.env[name] = ""
			ok, output := stub.run(t, "10.00")

			if ok || stub.requests != 0 {
				t.Fatalf("expected a failure and no request, got ok=%v and %d requests", ok, stub.requests)
			}
			expectFirstFailure(t, output, name)
		})
	}
}

func TestPreflightFailsWhenTheAmountIsNotADecimalAndRunsTheOtherChecks(t *testing.T) {
	ok, output := newPreflightStub().run(t, "ten")

	got := failures(output)
	if ok || len(got) != 1 || !strings.Contains(got[0], `"ten"`) || !strings.Contains(got[0], "10.00") {
		t.Fatalf("expected one amount failure:\n%s", output)
	}
	if !strings.Contains(output, "✓ transfer authorizer is active") || strings.Contains(output, "limit per transfer and") {
		t.Fatalf("expected the other checks without the limits:\n%s", output)
	}
}

func TestPreflightFailsWhenTheAmountIsNotPositive(t *testing.T) {
	for _, amount := range []string{"-5", "0", "1/2", "1e2"} {
		_, output := newPreflightStub().run(t, amount)

		expectFirstFailure(t, output, `"`+amount+`"`, "positive")
	}
}

func TestPreflightStopsBeforeAnyRequestOnAMissingKeyEvenWithABadAmount(t *testing.T) {
	stub := newPreflightStub()
	stub.env["MANZA_CREATOR_API_KEY"] = ""
	_, output := stub.run(t, "0")

	if n := len(failures(output)); n != 2 || stub.requests != 0 {
		t.Fatalf("expected two failures and no request, got %d and %d:\n%s", n, stub.requests, output)
	}
}

func TestPreflightFailsWhenBothKeysAreTheSame(t *testing.T) {
	stub := newPreflightStub()
	stub.env["MANZA_AUTHORIZER_API_KEY"] = "key-a"
	_, output := stub.run(t, "10.00")

	expectFirstFailure(t, output, "different")
}

func TestPreflightFailsOnARefusedRead(t *testing.T) {
	cases := []struct {
		name      string
		path      string
		response  stubResponse
		fragments []string
	}{
		{"key A cannot reach the entity", "/api/entity", apiError(401, "authentication_error"),
			[]string{"key A", "MANZA_CREATOR_API_KEY"}},
		{"no authorizer is enrolled", "/api/transfer_authorizer", apiError(404, "not_found_error"),
			[]string{"no transfer authorizer is enrolled", "Transfer approval"}},
		{"key B cannot read the authorizer", "/api/transfer_authorizer", apiError(403, "insufficient_scope"),
			[]string{"key B", "insufficient_scope", "transfers:authorize"}},
		{"key A cannot read the account", "/api/accounts/" + accountID, apiError(403, "insufficient_scope"),
			[]string{"key A", "accounts:read"}},
		{"key A cannot read the enrolled webhook endpoint", "/api/webhook_endpoints/" + endpointID, apiError(403, "insufficient_scope"),
			[]string{"key A", "webhook_endpoints:read"}},
		{"key A cannot read the beneficiary", "/api/beneficiaries/" + beneficiaryID, apiError(404, "not_found_error"),
			[]string{"key A", "MANZA_BENEFICIARY_ID"}},
		{"the enrolled webhook endpoint was deleted", "/api/webhook_endpoints/" + endpointID, apiError(404, "not_found_error"),
			[]string{"was deleted", "enrol"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newPreflightStub()
			stub.overrides[tc.path] = tc.response
			_, output := stub.run(t, "10.00")

			expectFirstFailure(t, output, tc.fragments...)
		})
	}
}

func TestPreflightFailsOnceOnTheAuthorizerLineWhenTheSecretHasExpired(t *testing.T) {
	stub := newPreflightStub()
	stub.authorizer["status"] = "suspended"
	stub.authorizer["suspension_reason"] = "secret_expired"
	stub.authorizer["secret_expires_at"] = "2026-10-19T00:00:00Z"
	_, output := stub.run(t, "10.00")

	got := failures(output)
	if len(got) != 1 || !strings.Contains(got[0], "suspended (secret_expired)") || !strings.Contains(got[0], "rotates the secret") {
		t.Fatalf("expected one failure on the authorizer line:\n%s", output)
	}
}

func TestPreflightFailsWithTheSuspensionReasonAndItsFix(t *testing.T) {
	stub := newPreflightStub()
	stub.authorizer["status"] = "suspended"
	stub.authorizer["suspension_reason"] = "endpoint_disabled"
	_, output := stub.run(t, "10.00")

	expectFirstFailure(t, output, "suspended (endpoint_disabled)", "re-enable")
}

func TestPreflightFailsWhenTheAccountCurrencyIsNotTheEnrolledOne(t *testing.T) {
	stub := newPreflightStub()
	stub.currency = "ZAR"
	_, output := stub.run(t, "10.00")

	expectFirstFailure(t, output, "ZAR", "MAD")
}

func TestPreflightFailsWhenTheSecretExpiresWithin7Days(t *testing.T) {
	stub := newPreflightStub()
	stub.authorizer["secret_expires_at"] = "2026-10-23T00:00:00Z"
	_, output := stub.run(t, "10.00")

	expectFirstFailure(t, output, "secret expires", "rotate")
}

func TestPreflightFailsWhenTheSigningSecretIsNotSet(t *testing.T) {
	stub := newPreflightStub()
	stub.env["MANZA_AUTHORIZER_SECRET"] = ""
	_, output := stub.run(t, "10.00")

	expectFirstFailure(t, output, "MANZA_AUTHORIZER_SECRET")
}

func TestPreflightFailsWhenTheEnrolledWebhookEndpointIsDisabled(t *testing.T) {
	stub := newPreflightStub()
	stub.endpoint["status"] = "disabled"
	_, output := stub.run(t, "10.00")

	expectFirstFailure(t, output, "disabled", "enable", "if the authorizer stays suspended")
}

func TestPreflightWarnsWhenTheEndpointURLChangesWhenTheTunnelRestarts(t *testing.T) {
	stub := newPreflightStub()
	stub.endpoint["url"] = "https://abc123.ngrok-free.app/"
	ok, output := stub.run(t, "10.00")

	warnings := linesWith(output, "!")
	if !ok || len(warnings) == 0 || !strings.Contains(warnings[0], "ngrok-free.app") || !strings.Contains(warnings[0], "reserved domain") {
		t.Fatalf("expected a passing run with a tunnel warning:\n%s", output)
	}
}

func TestPreflightDoesNotWarnForAHostThatOnlyEndsLikeATunnel(t *testing.T) {
	stub := newPreflightStub()
	stub.endpoint["url"] = "https://notngrok-free.app/"
	_, output := stub.run(t, "10.00")

	if warnings := linesWith(output, "!"); len(warnings) != 0 {
		t.Fatalf("expected no warning:\n%s", output)
	}
}

func TestPreflightReadsTheLegacyBaseURL(t *testing.T) {
	stub := newPreflightStub()
	server := httptest.NewServer(stub)
	t.Cleanup(server.Close)
	stub.env["ZAZU_BASE_URL"] = server.URL
	out := &bytes.Buffer{}
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	ok := transferauthorizer.NewPreflight(func(name string) string { return stub.env[name] }, "10.00", out, now).Run(context.Background())

	if !ok || stub.requests == 0 {
		t.Fatalf("expected the stub at ZAZU_BASE_URL to answer:\n%s", out.String())
	}
}

func TestPreflightFailsWhenTheAmountIsOverTheLimitPerTransfer(t *testing.T) {
	_, output := newPreflightStub().run(t, "1500.00")

	expectFirstFailure(t, output, "limit per transfer")
}

func TestPreflightFailsWhenTheAmountDoesNotFitTheLast24HoursRoom(t *testing.T) {
	stub := newPreflightStub()
	stub.authorizer["daily_authorized_total"] = "4995.0"
	_, output := stub.run(t, "10.00")

	expectFirstFailure(t, output, "24 hours", "5.00 left")
}

func TestPreflightWarnsThatANewPayeeNeedsTheToggle(t *testing.T) {
	stub := newPreflightStub()
	stub.payee["trusted"] = false
	ok, output := stub.run(t, "10.00")

	warnings := linesWith(output, "!")
	if !ok || len(warnings) == 0 {
		t.Fatalf("expected a passing run with a warning:\n%s", output)
	}
	for _, fragment := range []string{"not trusted yet", "Approve transfers to new payees", "IP allowlist"} {
		if !strings.Contains(warnings[0], fragment) {
			t.Fatalf("expected %q in %q", fragment, warnings[0])
		}
	}
}

func TestPreflightReportsAnExternalAccountsFieldThatIsNotAList(t *testing.T) {
	stub := newPreflightStub()
	stub.overrides["/api/beneficiaries/"+beneficiaryID] = stubResponse{status: 200, body: `{"id":"b","external_accounts":"x"}`}
	_, output := stub.run(t, "10.00")

	expectFirstFailure(t, output, "external_accounts is not a list")
}

func TestPreflightFailsWhenTheBeneficiaryHasNoDefaultBankAccount(t *testing.T) {
	stub := newPreflightStub()
	stub.payee["default"] = false
	_, output := stub.run(t, "10.00")

	expectFirstFailure(t, output, "default bank account")
}
