package transferauthorizer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	manza "github.com/getmanza/manza-go"
)

// Event is the only webhook event the authorizer answers.
const Event = "payment.authorization_requested"

const (
	attempts      = 3
	maxRetryAfter = 30 * time.Second // a challenge expires, so never park for long
	busy          = "authorization_busy"
)

// checkedFields maps a ledger field to the webhook's `data.payment` field.
// All must match.
var checkedFields = []struct{ ledger, webhook string }{
	{"payment_id", "id"},
	{"amount", "amount"},
	{"currency_code", "currency_code"},
	{"account_id", "account_id"},
	{"payee", "payee"},
}

// App answers `payment.authorization_requested` webhooks with key B (the
// authorizer key):
//
//  1. Verify the delivery (401 when the signature or timestamp is bad).
//  2. Find the payment in your own ledger by its client_reference.
//  3. Missing or different from what you created: decline with a reason.
//  4. A payee not trusted yet (`payee_trusted: false`, only with "Approve
//     transfers to new payees" on) is trusted by the authorize, unless
//     AllowNewPayees is false (ALLOW_NEW_PAYEES=false), which declines it.
//  5. Otherwise sign the challenge from the ledger and authorize it.
//
// It answers 200 first and does steps 2-5 on the Executor (a goroutine by
// default): the delivery has a timeout, and a retried webhook is safe
// because authorize replays idempotently.
type App struct {
	Client *manza.Client // key B
	Secret string        // the enrolled endpoint's signing secret
	Ledger *Ledger
	// Executor runs the ledger check, signing and answer after the 200.
	Executor func(job func())
	Logger   *log.Logger
	// LedgerWait is how long to wait for the ledger entry: the webhook can
	// arrive before create_transfer has written it from the create response.
	LedgerWait time.Duration
	// RetryDelay is the base delay between attempts.
	RetryDelay time.Duration
	// AllowNewPayees false declines transfers to payees not trusted yet.
	AllowNewPayees bool
}

// NewApp returns an App with the defaults: a goroutine per answer, logs on
// stdout, a 10 s ledger wait and a 2 s base retry delay.
func NewApp(client *manza.Client, secret string, ledger *Ledger, allowNewPayees bool) *App {
	return &App{
		Client:         client,
		Secret:         secret,
		Ledger:         ledger,
		Executor:       func(job func()) { go job() },
		Logger:         log.New(os.Stdout, "", log.LstdFlags),
		LedgerWait:     10 * time.Second,
		RetryDelay:     2 * time.Second,
		AllowNewPayees: allowNewPayees,
	}
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logf("WARN", "rejected a delivery: %v", err)
		respond(w, http.StatusUnauthorized)
		return
	}
	event, err := manza.ConstructWebhookEvent(body, r.Header, a.Secret)
	if err != nil {
		var argErr *manza.ArgumentError
		if errors.As(err, &argErr) {
			err = errors.New(argErr.Message)
		}
		a.logf("WARN", "rejected a delivery: %v", err)
		respond(w, http.StatusUnauthorized)
		return
	}
	a.logf("INFO", "received %v (%v), signature verified", event["event"], event["event_id"])

	if event["event"] != Event {
		a.logf("INFO", "ignoring %v: this app only answers %s", event["event"], Event)
		respond(w, http.StatusOK)
		return
	}

	data, _ := event["data"].(map[string]any)
	a.Executor(func() { a.answer(data) })
	respond(w, http.StatusOK)
}

func respond(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	if status == http.StatusOK {
		fmt.Fprint(w, "ok")
	} else {
		fmt.Fprint(w, "unauthorized")
	}
}

func (a *App) answer(data map[string]any) {
	if err := a.check(data); err != nil {
		var apiErr *manza.Error
		if errors.As(err, &apiErr) {
			a.logf("ERROR", "API error %s: %s (type %q, request_id %q)", describeKind(apiErr), apiErr.Message, apiErr.Type, apiErr.RequestID)
			return
		}
		a.logf("ERROR", "could not answer the challenge: %v", err)
	}
}

func (a *App) check(data map[string]any) error {
	payment, ok := data["payment"].(map[string]any)
	if !ok {
		return errors.New("the webhook has no data.payment")
	}
	authorization, ok := data["authorization"].(map[string]any)
	if !ok {
		return errors.New("the webhook has no data.authorization")
	}
	paymentID, authorizationID := str(payment["id"]), str(authorization["id"])
	clientReference := str(payment["client_reference"])

	entry, found, err := a.waitForEntry(clientReference)
	if err != nil {
		return err
	}
	mismatch := fmt.Sprintf("client_reference %s is not in the ledger", inspect(payment["client_reference"]))
	if found {
		mismatch = mismatchReason(entry, payment)
	}
	if mismatch != "" {
		return a.decline(paymentID, authorizationID, mismatch)
	}

	// Anything but true counts as new, so a webhook without the field is
	// declined under ALLOW_NEW_PAYEES=false rather than trusted blindly.
	if payment["payee_trusted"] != true {
		if !a.AllowNewPayees {
			return a.decline(paymentID, authorizationID,
				fmt.Sprintf("new payee %v: ALLOW_NEW_PAYEES=false declines payees not trusted yet", payment["payee"]))
		}
		a.logf("INFO", "new payee %v, will be trusted on authorize", payment["payee"])
	}

	a.logf("INFO", "ledger match for %s: %v %v to %v", clientReference, payment["amount"], payment["currency_code"], payment["payee"])
	return a.authorize(entry, authorization, clientReference, data["signature_input"])
}

func (a *App) waitForEntry(clientReference string) (Entry, bool, error) {
	deadline := time.Now().Add(a.LedgerWait)
	for {
		entry, found, err := a.Ledger.Find(clientReference)
		if err != nil || found || !time.Now().Before(deadline) {
			return entry, found, err
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func mismatchReason(entry Entry, payment map[string]any) string {
	recorded := map[string]string{
		"payment_id": entry.PaymentID, "amount": entry.Amount, "currency_code": entry.CurrencyCode,
		"account_id": entry.AccountID, "payee": entry.Payee,
	}
	for _, field := range checkedFields {
		if got, ok := payment[field.webhook].(string); ok && got == recorded[field.ledger] {
			continue
		}
		return fmt.Sprintf("%s differs from the ledger (ledger %q, webhook %s)",
			field.ledger, recorded[field.ledger], inspect(payment[field.webhook]))
	}
	return ""
}

// authorize signs from the ledger, never from the webhook: `signature_input`
// in the webhook is only there to compare against.
func (a *App) authorize(entry Entry, authorization map[string]any, clientReference string, webhookInput any) error {
	input := manza.SignatureInput(manza.TransferAuthorizationFields{
		PaymentID:       entry.PaymentID,
		Nonce:           str(authorization["nonce"]),
		Amount:          entry.Amount,
		CurrencyCode:    entry.CurrencyCode,
		AccountID:       entry.AccountID,
		Payee:           entry.Payee,
		ClientReference: clientReference,
	})
	// A signature over a different input is sure to fail, and failed attempts
	// count toward suspending the authorizer. Leave the challenge unanswered
	// instead: it expires and goes to your approvers.
	if webhookInput != input {
		a.logf("WARN", "signature input differs from the webhook's: ledger %q, webhook %s; "+
			"not answering, the challenge goes to your approvers when it expires", input, inspect(webhookInput))
		return nil
	}

	signature := manza.Sign(a.Secret, input)
	a.logf("INFO", "signed the challenge for payment %s", entry.PaymentID)

	resp, err := a.withRetries(func() (*manza.Response, error) {
		return a.Client.Payments.Authorize(context.Background(), entry.PaymentID, str(authorization["id"]), signature)
	})
	if err != nil {
		return err
	}
	a.logf("INFO", "authorized payment %s (status %s)", entry.PaymentID, inspect(resp.Body["status"]))
	return nil
}

func (a *App) decline(paymentID, authorizationID, reason string) error {
	_, err := a.withRetries(func() (*manza.Response, error) {
		return a.Client.Payments.Decline(context.Background(), paymentID, authorizationID, reason)
	})
	if err != nil {
		return err
	}
	a.logf("WARN", "declined payment %s: %s", paymentID, reason)
	return nil
}

// withRetries retries what is worth another try: the API did not answer,
// asked us to slow down, or was busy with a bank-detail edit, trust or
// revocation on the same payee (409 `authorization_busy`). Authorize and
// decline replay idempotently, so a retry is safe.
func (a *App) withRetries(call func() (*manza.Response, error)) (*manza.Response, error) {
	for attempt := 1; ; attempt++ {
		resp, err := call()
		if err == nil || attempt >= attempts || !transient(err) {
			return resp, err
		}
		delay := a.RetryDelay * time.Duration(attempt)
		requestID := ""
		var apiErr *manza.Error
		if errors.As(err, &apiErr) {
			requestID = apiErr.RequestID
			if apiErr.Kind == manza.KindRateLimit && apiErr.RetryAfter > 0 {
				delay = min(time.Duration(apiErr.RetryAfter)*time.Second, maxRetryAfter)
			}
		}
		a.logf("WARN", "%v (request_id %q), retrying in %s", err, requestID, delay)
		time.Sleep(delay)
	}
}

func transient(err error) bool {
	var connErr *manza.ConnectionError
	if errors.As(err, &connErr) {
		return true
	}
	var apiErr *manza.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.Kind {
	case manza.KindServer, manza.KindRateLimit:
		return true
	case manza.KindConflict:
		return apiErr.Type == busy
	}
	return false
}

func (a *App) logf(level, format string, args ...any) {
	a.Logger.Printf(level+" "+format, args...)
}

// describeKind names an API error by its Kind and status.
func describeKind(err *manza.Error) string {
	return fmt.Sprintf("%s (%d)", err.Kind, err.Status)
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// inspect quotes a string and spells out nil, so a log line shows exactly
// what the webhook carried.
func inspect(v any) string {
	switch v := v.(type) {
	case nil:
		return "nil"
	case string:
		return strconv.Quote(v)
	default:
		return fmt.Sprint(v)
	}
}
