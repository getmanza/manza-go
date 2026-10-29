package transferauthorizer_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	manza "github.com/getmanza/manza-go"
	transferauthorizer "github.com/getmanza/manza-go/examples/transfer_authorizer"
)

// Drives the example's authorizer with a signed copy of the documented
// `payment.authorization_requested` payload (getmanza/app
// WebhookEvents::PaymentAuthorizationRequested.example_payload, verbatim),
// against a stubbed Manza API. No cassette, no network: the same cases as
// manza-ruby's spec/examples/transfer_authorizer_spec.rb.

const (
	secret          = "whsec_example_authorizer_secret"
	draftID         = "01958e51-7c5a-7000-8000-000000000012"
	authorizationID = "01958e51-7c5a-7000-8000-000000000011"
	nonce           = "9f86d081884c7d659a2feaa0c55ad015"
	payeeToken      = "ext:01958e51-7c5a-7000-8000-000000000014"
	webhookInput    = "manza.transfer-authorization.v1|01958e51-7c5a-7000-8000-000000000012|" +
		"9f86d081884c7d659a2feaa0c55ad015|2500.0|ZAR|01958e51-7c5a-7000-8000-000000000013|" +
		"ext:01958e51-7c5a-7000-8000-000000000014|po_8812"
	authorizePath = "/api/payments/" + draftID + "/authorize"
	declinePath   = "/api/payments/" + draftID + "/decline"
)

func examplePayload() map[string]any {
	return map[string]any{
		"event":      "payment.authorization_requested",
		"event_id":   "01958e51-7c5a-7000-8000-000000000010",
		"created_at": "2026-10-01T12:00:00Z",
		"data": map[string]any{
			"authorization": map[string]any{
				"id":                authorizationID,
				"nonce":             nonce,
				"expires_at":        "2026-10-01T13:00:00Z",
				"signature_version": "manza.transfer-authorization.v1",
			},
			"payment": map[string]any{
				"id":                     draftID,
				"amount":                 "2500.0",
				"currency_code":          "ZAR",
				"account_id":             "01958e51-7c5a-7000-8000-000000000013",
				"payee":                  payeeToken,
				"beneficiary_id":         "01958e51-7c5a-7000-8000-000000000015",
				"external_account_id":    "01958e51-7c5a-7000-8000-000000000014",
				"destination_account_id": nil,
				"payment_reference":      "INV-2026-001",
				"client_reference":       "po_8812",
				"payee_trusted":          false,
			},
			"signature_input": webhookInput,
		},
	}
}

func ledgerEntry() transferauthorizer.Entry {
	return transferauthorizer.Entry{
		PaymentID:    draftID,
		Amount:       "2500.0",
		CurrencyCode: "ZAR",
		AccountID:    "01958e51-7c5a-7000-8000-000000000013",
		Payee:        payeeToken,
	}
}

func paymentOf(payload map[string]any) map[string]any {
	return payload["data"].(map[string]any)["payment"].(map[string]any)
}

type stubResponse struct {
	status int
	body   string
	header map[string]string
}

type recordedRequest struct {
	method, path, authorization string
	body                        map[string]any
}

// stubAPI answers each "METHOD /path" with its responses in turn (the last
// one repeats) and records every request.
type stubAPI struct {
	t         *testing.T
	mu        sync.Mutex
	responses map[string][]stubResponse
	requests  []recordedRequest
}

func newStubAPI(t *testing.T) (*stubAPI, *httptest.Server) {
	api := &stubAPI{t: t, responses: map[string][]stubResponse{}}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	return api, server
}

func (s *stubAPI) on(method, path string, responses ...stubResponse) {
	s.responses[method+" "+path] = responses
}

func (s *stubAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			s.t.Errorf("request body is not JSON: %s", raw)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, recordedRequest{r.Method, r.URL.Path, r.Header.Get("Authorization"), body})

	key := r.Method + " " + r.URL.Path
	queue := s.responses[key]
	if len(queue) == 0 {
		s.t.Errorf("unexpected request %s", key)
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
	resp := queue[0]
	if len(queue) > 1 {
		s.responses[key] = queue[1:]
	}
	w.Header().Set("Content-Type", "application/json")
	for k, v := range resp.header {
		w.Header().Set(k, v)
	}
	w.WriteHeader(resp.status)
	io.WriteString(w, resp.body)
}

func (s *stubAPI) calls(method, path string) []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []recordedRequest
	for _, r := range s.requests {
		if r.method == method && r.path == path {
			out = append(out, r)
		}
	}
	return out
}

func (s *stubAPI) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func ok(body string) stubResponse { return stubResponse{status: 200, body: body} }

type harness struct {
	app    *transferauthorizer.App
	api    *stubAPI
	ledger *transferauthorizer.Ledger
	log    *bytes.Buffer
}

func newHarness(t *testing.T, allowNewPayees bool) *harness {
	api, server := newStubAPI(t)
	client, err := manza.New(manza.WithAPIKey("key-b"), manza.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	ledger := transferauthorizer.NewLedger(filepath.Join(t.TempDir(), "ledger.json"))
	logs := &bytes.Buffer{}
	app := transferauthorizer.NewApp(client, secret, ledger, allowNewPayees)
	app.Executor = func(job func()) { job() }
	app.Logger = log.New(logs, "", 0)
	app.LedgerWait = 0
	app.RetryDelay = 0
	return &harness{app: app, api: api, ledger: ledger, log: logs}
}

func (h *harness) record(t *testing.T, entry transferauthorizer.Entry) {
	t.Helper()
	if err := h.ledger.Record("po_8812", entry); err != nil {
		t.Fatalf("record: %v", err)
	}
}

func (h *harness) deliverSigned(body []byte, signature string) *httptest.ResponseRecorder {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	if signature == "" {
		signature = manza.SignWebhook(body, timestamp, secret)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Manza-Signature", signature)
	req.Header.Set("X-Manza-Timestamp", timestamp)
	rec := httptest.NewRecorder()
	h.app.ServeHTTP(rec, req)
	return rec
}

func (h *harness) deliver(t *testing.T, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return h.deliverSigned(body, "")
}

func (h *harness) expectLog(t *testing.T, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(h.log.String(), fragment) {
			t.Fatalf("expected the log to contain %q:\n%s", fragment, h.log.String())
		}
	}
}

func expectedSignature() string {
	entry := ledgerEntry()
	return manza.Sign(secret, manza.SignatureInput(manza.TransferAuthorizationFields{
		PaymentID: entry.PaymentID, Nonce: nonce, Amount: entry.Amount, CurrencyCode: entry.CurrencyCode,
		AccountID: entry.AccountID, Payee: entry.Payee, ClientReference: "po_8812",
	}))
}

func TestAuthorizerSignsFromTheLedgerAndAuthorizesWithKeyB(t *testing.T) {
	h := newHarness(t, true)
	h.record(t, ledgerEntry())
	h.api.on("POST", authorizePath, ok(`{"id":"`+draftID+`","status":"processing"}`))

	rec := h.deliver(t, examplePayload())

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	calls := h.api.calls("POST", authorizePath)
	if len(calls) != 1 {
		t.Fatalf("expected one authorize, got %d", len(calls))
	}
	want := map[string]any{"authorization_id": authorizationID, "signature": expectedSignature()}
	if calls[0].authorization != "Bearer key-b" || !equalJSON(calls[0].body, want) {
		t.Fatalf("unexpected authorize %#v", calls[0])
	}
	h.expectLog(t, "verified", "ledger match", "signed", `authorized payment `+draftID+` (status "processing")`)
}

func TestAuthorizerLogsTheAPIErrorTypeAndRequestID(t *testing.T) {
	h := newHarness(t, true)
	h.record(t, ledgerEntry())
	h.api.on("POST", authorizePath, stubResponse{status: 422, header: map[string]string{"X-Request-Id": "req_123"},
		body: `{"error":{"message":"Signature does not match","type":"invalid_signature"}}`})

	if rec := h.deliver(t, examplePayload()); rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	h.expectLog(t, "API error", "invalid_signature", "req_123")
}

func TestAuthorizerDoesNotAuthorizeWhenTheWebhookSignatureInputDiffers(t *testing.T) {
	h := newHarness(t, true)
	h.record(t, ledgerEntry())
	payload := examplePayload()
	payload["data"].(map[string]any)["signature_input"] = "something-else"

	h.deliver(t, payload)

	if h.api.total() != 0 {
		t.Fatalf("expected no API call, got %d", h.api.total())
	}
	h.expectLog(t, "something-else", webhookInput)
}

func TestAuthorizerRetriesAfterATransientFailure(t *testing.T) {
	h := newHarness(t, true)
	h.record(t, ledgerEntry())
	h.api.on("POST", authorizePath, stubResponse{status: 503, body: "{}"}, ok(`{"status":"processing"}`))

	h.deliver(t, examplePayload())

	if n := len(h.api.calls("POST", authorizePath)); n != 2 {
		t.Fatalf("expected two attempts, got %d", n)
	}
	h.expectLog(t, "retrying", "authorized")
}

func TestAuthorizerGivesUpAfterThreeAttempts(t *testing.T) {
	h := newHarness(t, true)
	h.record(t, ledgerEntry())
	h.api.on("POST", authorizePath, stubResponse{status: 503, body: "{}"})

	h.deliver(t, examplePayload())

	if n := len(h.api.calls("POST", authorizePath)); n != 3 {
		t.Fatalf("expected three attempts, got %d", n)
	}
	h.expectLog(t, "API error server (503)")
}

func TestAuthorizerRetriesWhileTheAPIAnswersAuthorizationBusy(t *testing.T) {
	h := newHarness(t, true)
	h.record(t, ledgerEntry())
	h.api.on("POST", authorizePath, stubResponse{status: 409, body: `{"error":{"type":"authorization_busy"}}`},
		ok(`{"status":"processing"}`))

	h.deliver(t, examplePayload())

	if n := len(h.api.calls("POST", authorizePath)); n != 2 {
		t.Fatalf("expected two attempts, got %d", n)
	}
	h.expectLog(t, "retrying", "authorized")
}

func TestAuthorizerDoesNotRetryAnotherConflict(t *testing.T) {
	h := newHarness(t, true)
	h.record(t, ledgerEntry())
	h.api.on("POST", authorizePath, stubResponse{status: 409, body: `{"error":{"type":"already_answered"}}`})

	h.deliver(t, examplePayload())

	if n := len(h.api.calls("POST", authorizePath)); n != 1 {
		t.Fatalf("expected one attempt, got %d", n)
	}
	h.expectLog(t, "API error conflict (409)")
}

func TestAuthorizerDoesNotRetryAValidationError(t *testing.T) {
	h := newHarness(t, true)
	h.record(t, ledgerEntry())
	h.api.on("POST", authorizePath, stubResponse{status: 422, body: `{"error":{"type":"invalid_signature"}}`})

	h.deliver(t, examplePayload())

	if n := len(h.api.calls("POST", authorizePath)); n != 1 {
		t.Fatalf("expected one attempt, got %d", n)
	}
}

func TestAuthorizerLogsThatANewPayeeWillBeTrusted(t *testing.T) {
	h := newHarness(t, true)
	h.record(t, ledgerEntry())
	h.api.on("POST", authorizePath, ok(`{"status":"processing"}`))

	h.deliver(t, examplePayload())

	h.expectLog(t, "new payee "+payeeToken+", will be trusted on authorize")
}

func TestAuthorizerDoesNotLogANewPayeeWhenThePayeeIsTrusted(t *testing.T) {
	h := newHarness(t, true)
	h.record(t, ledgerEntry())
	h.api.on("POST", authorizePath, ok(`{"status":"processing"}`))
	payload := examplePayload()
	paymentOf(payload)["payee_trusted"] = true

	h.deliver(t, payload)

	if strings.Contains(h.log.String(), "new payee") {
		t.Fatalf("expected no new-payee line:\n%s", h.log.String())
	}
}

func TestAuthorizerWithoutNewPayees(t *testing.T) {
	declined := func(t *testing.T, h *harness) {
		t.Helper()
		calls := h.api.calls("POST", declinePath)
		if len(calls) != 1 {
			t.Fatalf("expected one decline, got %d", len(calls))
		}
		reason, _ := calls[0].body["reason"].(string)
		if calls[0].body["authorization_id"] != authorizationID || !strings.Contains(reason, "new payee") {
			t.Fatalf("unexpected decline %#v", calls[0].body)
		}
		if n := len(h.api.calls("POST", authorizePath)); n != 0 {
			t.Fatalf("expected no authorize, got %d", n)
		}
	}

	t.Run("declines a webhook that does not say whether the payee is trusted", func(t *testing.T) {
		h := newHarness(t, false)
		h.record(t, ledgerEntry())
		h.api.on("POST", declinePath, ok(`{}`))
		payload := examplePayload()
		delete(paymentOf(payload), "payee_trusted")

		h.deliver(t, payload)

		declined(t, h)
	})

	t.Run("declines a transfer to a payee that is not trusted yet", func(t *testing.T) {
		h := newHarness(t, false)
		h.record(t, ledgerEntry())
		h.api.on("POST", declinePath, ok(`{"id":"`+authorizationID+`","status":"declined"}`))

		h.deliver(t, examplePayload())

		declined(t, h)
		h.expectLog(t, "declined", "new payee")
	})

	t.Run("authorizes a trusted payee", func(t *testing.T) {
		h := newHarness(t, false)
		h.record(t, ledgerEntry())
		h.api.on("POST", authorizePath, ok(`{"status":"processing"}`))
		payload := examplePayload()
		paymentOf(payload)["payee_trusted"] = true

		h.deliver(t, payload)

		if n := len(h.api.calls("POST", authorizePath)); n != 1 {
			t.Fatalf("expected one authorize, got %d", n)
		}
	})
}

func TestAuthorizerDeclinesALedgerMismatchNamingTheField(t *testing.T) {
	h := newHarness(t, true)
	entry := ledgerEntry()
	entry.Amount = "25000.0"
	h.record(t, entry)
	h.api.on("POST", declinePath, ok(`{"id":"`+authorizationID+`","status":"declined"}`))

	rec := h.deliver(t, examplePayload())

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	calls := h.api.calls("POST", declinePath)
	if len(calls) != 1 {
		t.Fatalf("expected one decline, got %d", len(calls))
	}
	if reason, _ := calls[0].body["reason"].(string); calls[0].body["authorization_id"] != authorizationID || !strings.Contains(reason, "amount") {
		t.Fatalf("unexpected decline %#v", calls[0].body)
	}
	if n := len(h.api.calls("POST", authorizePath)); n != 0 {
		t.Fatalf("expected no authorize, got %d", n)
	}
	h.expectLog(t, "declined")
}

func TestAuthorizerDeclinesAClientReferenceItNeverIssued(t *testing.T) {
	h := newHarness(t, true)
	h.api.on("POST", declinePath, ok(`{}`))

	h.deliver(t, examplePayload())

	calls := h.api.calls("POST", declinePath)
	if len(calls) != 1 {
		t.Fatalf("expected one decline, got %d", len(calls))
	}
	if reason, _ := calls[0].body["reason"].(string); !strings.Contains(reason, "po_8812") {
		t.Fatalf("unexpected reason %q", reason)
	}
}

func TestAuthorizerAnswers401ToABadSignatureAndCallsNothing(t *testing.T) {
	h := newHarness(t, true)
	body, _ := json.Marshal(examplePayload())

	rec := h.deliverSigned(body, strings.Repeat("0", 64))

	if rec.Code != 401 || h.api.total() != 0 {
		t.Fatalf("expected 401 and no call, got %d and %d calls", rec.Code, h.api.total())
	}
	h.expectLog(t, "rejected a delivery: invalid webhook signature")
}

func TestAuthorizerAnswers401ToASignedBodyThatIsNotAJSONObject(t *testing.T) {
	h := newHarness(t, true)

	rec := h.deliverSigned([]byte("[]"), "")

	if rec.Code != 401 || h.api.total() != 0 {
		t.Fatalf("expected 401 and no call, got %d and %d calls", rec.Code, h.api.total())
	}
}

func TestAuthorizerAnswers200ToAnotherEventAndCallsNothing(t *testing.T) {
	h := newHarness(t, true)

	rec := h.deliver(t, map[string]any{"event": "test.ping", "event_id": "0199a1b2-0000-7000-8000-000000000010"})

	if rec.Code != 200 || h.api.total() != 0 {
		t.Fatalf("expected 200 and no call, got %d and %d calls", rec.Code, h.api.total())
	}
	h.expectLog(t, "ignoring test.ping")
}

func equalJSON(a, b map[string]any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

func TestAuthorizerAnswers401ToAStaleTimestampAndCallsNothing(t *testing.T) {
	h := newHarness(t, true)
	h.record(t, ledgerEntry())
	body, _ := json.Marshal(examplePayload())
	timestamp := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("X-Manza-Signature", manza.SignWebhook(body, timestamp, secret))
	req.Header.Set("X-Manza-Timestamp", timestamp)
	rec := httptest.NewRecorder()

	h.app.ServeHTTP(rec, req)

	if rec.Code != 401 || h.api.total() != 0 {
		t.Fatalf("expected 401 and no call, got %d and %d calls", rec.Code, h.api.total())
	}
	h.expectLog(t, "rejected a delivery: webhook timestamp outside tolerance")
}

func TestAuthorizerRejectsAnOversizedBodyBeforeVerifying(t *testing.T) {
	h := newHarness(t, true)

	rec := h.deliverSigned(bytes.Repeat([]byte("a"), transferauthorizer.MaxBodyBytes+1), "")

	if rec.Code != 413 || h.api.total() != 0 {
		t.Fatalf("expected 413 and no call, got %d and %d calls", rec.Code, h.api.total())
	}
}
