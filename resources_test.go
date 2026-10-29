package manza_test

// Mirror of manza-ruby's spec/manza/resources/*_spec.rb — same cassettes,
// same assertions, per the cross-language SDK contract.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	manza "github.com/getmanza/manza-go"
)

func replayClient(t *testing.T, server *httptest.Server) *manza.Client {
	t.Helper()
	client, err := manza.New(
		manza.WithAPIKey("test-api-key-for-replay"),
		manza.WithBaseURL(server.URL),
	)
	if err != nil {
		t.Fatalf("build client: %v", err)
	}
	return client
}

func TestEntityGet(t *testing.T) {
	server := startReplayServer(t, "entity/get")
	client := replayClient(t, server)

	resp, err := client.Entity.Get(context.Background())
	if err != nil {
		t.Fatalf("entity get: %v", err)
	}
	if _, ok := resp.Body["id"].(string); !ok {
		t.Fatalf("expected string id, got %#v", resp.Body["id"])
	}
}

func TestAccounts(t *testing.T) {
	server := startReplayServer(t, "accounts/list", "accounts/get", "accounts/list_transactions", "accounts/get_transaction")
	client := replayClient(t, server)
	ctx := context.Background()

	page, err := client.Accounts.List(ctx, manza.AccountListParams{})
	if err != nil {
		t.Fatalf("accounts list: %v", err)
	}
	if page.Data == nil {
		t.Fatal("expected data rows")
	}

	accountID := fixtureID(t, "MANZA_FIXTURE_ACCOUNT_ID")
	if _, err := client.Accounts.Get(ctx, accountID); err != nil {
		t.Fatalf("accounts get: %v", err)
	}

	if _, err := client.Accounts.ListTransactions(ctx, accountID, manza.TransactionListParams{}); err != nil {
		t.Fatalf("list transactions: %v", err)
	}

	txID := fixtureID(t, "MANZA_FIXTURE_TRANSACTION_ID")
	if _, err := client.Accounts.GetTransaction(ctx, accountID, txID); err != nil {
		t.Fatalf("get transaction: %v", err)
	}
}

func TestCustomers(t *testing.T) {
	server := startReplayServer(t, "customers/list", "customers/get", "customers/create", "customers/update", "customers/delete")
	client := replayClient(t, server)
	ctx := context.Background()

	if _, err := client.Customers.List(ctx, manza.CustomerListParams{}); err != nil {
		t.Fatalf("customers list: %v", err)
	}

	customerID := fixtureID(t, "MANZA_FIXTURE_CUSTOMER_ID")
	resp, err := client.Customers.Get(ctx, customerID)
	if err != nil {
		t.Fatalf("customers get: %v", err)
	}
	if _, ok := resp.Body["id"].(string); !ok {
		t.Fatalf("expected string id, got %#v", resp.Body["id"])
	}
}

func TestInvoices(t *testing.T) {
	server := startReplayServer(t, "invoices/list", "invoices/get")
	client := replayClient(t, server)
	ctx := context.Background()

	page, err := client.Invoices.List(ctx, manza.InvoiceListParams{})
	if err != nil {
		t.Fatalf("invoices list: %v", err)
	}
	if page.Data == nil {
		t.Fatal("expected data rows")
	}

	invoiceID := fixtureID(t, "MANZA_FIXTURE_INVOICE_ID")
	if _, err := client.Invoices.Get(ctx, invoiceID); err != nil {
		t.Fatalf("invoices get: %v", err)
	}
}

func TestPaymentLinks(t *testing.T) {
	server := startReplayServer(t, "payment_links/list", "payment_links/get", "payment_links/create", "payment_links/cancel")
	client := replayClient(t, server)
	ctx := context.Background()

	if _, err := client.PaymentLinks.List(ctx, manza.PaymentLinkListParams{}); err != nil {
		t.Fatalf("payment links list: %v", err)
	}

	resp, err := client.PaymentLinks.Create(ctx, manza.Attributes{
		"account_id":  fixtureID(t, "MANZA_FIXTURE_ACCOUNT_ID"),
		"amount":      "100.00",
		"title":       "SDK fixture",
		"description": "Created by zazu-ruby fixture spec",
		"link_type":   "single",
	})
	if err != nil {
		t.Fatalf("payment links create: %v", err)
	}
	if resp.Status != 201 {
		t.Fatalf("expected 201, got %d", resp.Status)
	}

	if _, err := client.PaymentLinks.Cancel(ctx, fixtureID(t, "MANZA_FIXTURE_CANCELLABLE_PAYMENT_LINK_ID")); err != nil {
		t.Fatalf("payment links cancel: %v", err)
	}
}

func TestCheckoutSessions(t *testing.T) {
	server := startReplayServer(t, "checkout_sessions/get")
	client := replayClient(t, server)

	resp, err := client.CheckoutSessions.Get(context.Background(), fixtureID(t, "MANZA_FIXTURE_CHECKOUT_SESSION_ID"))
	if err != nil {
		t.Fatalf("checkout sessions get: %v", err)
	}
	var session manza.CheckoutSession
	if err := resp.Decode(&session); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if session.ID != fixtureID(t, "MANZA_FIXTURE_CHECKOUT_SESSION_ID") || session.Status != "open" {
		t.Fatalf("unexpected session %#v", session)
	}
	if session.SettledAt != nil || session.Transaction != nil || session.CustomerName != nil || session.CollectBillingAddress != nil {
		t.Fatalf("expected the new fields to decode as nil, got %#v", session)
	}
	if session.BillingAddress == nil || len(session.BillingAddress) != 0 {
		t.Fatalf("expected empty billing_address, got %#v", session.BillingAddress)
	}
}

func TestCheckoutSessionsCreate(t *testing.T) {
	server := startReplayServer(t, "checkout_sessions/create")
	client := replayClient(t, server)

	resp, err := client.CheckoutSessions.Create(context.Background(), manza.Attributes{
		"account_id":     fixtureID(t, "MANZA_FIXTURE_ACCOUNT_ID"),
		"amount":         "100.00",
		"success_url":    "https://example.com/zazu-fixture-success?session_id={CHECKOUT_SESSION_ID}",
		"cancel_url":     "https://example.com/zazu-fixture-cancel",
		"description":    "Created by zazu-ruby fixture spec",
		"customer_email": "fixture@example.com",
		"metadata":       map[string]any{"order_id": "ORD-FIXTURE"},
	})
	if err != nil {
		t.Fatalf("checkout sessions create: %v", err)
	}
	if resp.Status != 201 {
		t.Fatalf("expected 201, got %d", resp.Status)
	}
	if status, _ := resp.Body["status"].(string); status != "open" {
		t.Fatalf("expected open, got %q", status)
	}
	if _, ok := resp.Body["url"].(string); !ok {
		t.Fatalf("expected string url, got %#v", resp.Body["url"])
	}
}

func TestWebhookEndpoints(t *testing.T) {
	server := startReplayServer(t, "webhook_endpoints/list", "webhook_endpoints/get")
	client := replayClient(t, server)
	ctx := context.Background()

	if _, err := client.WebhookEndpoints.List(ctx, manza.ListParams{}); err != nil {
		t.Fatalf("webhook endpoints list: %v", err)
	}
	if _, err := client.WebhookEndpoints.Get(ctx, fixtureID(t, "MANZA_FIXTURE_WEBHOOK_ID")); err != nil {
		t.Fatalf("webhook endpoints get: %v", err)
	}
}

func TestPaymentsCreate(t *testing.T) {
	server := startReplayServer(t, "payments/create")
	client := replayClient(t, server)

	resp, err := client.Payments.Create(context.Background(), manza.Attributes{
		"account_id":        fixtureID(t, "MANZA_FIXTURE_ACCOUNT_ID"),
		"beneficiary_id":    fixtureID(t, "MANZA_FIXTURE_BENEFICIARY_ID"),
		"amount":            "10.00",
		"payment_reference": "SDK fixture",
		"internal_notes":    "[zazu-ruby-fixture] create spec - do not approve",
		"client_reference":  fixtureID(t, "MANZA_FIXTURE_CLIENT_REFERENCE"),
	})
	if err != nil {
		t.Fatalf("payments create: %v", err)
	}
	if resp.Status != 201 {
		t.Fatalf("expected 201, got %d", resp.Status)
	}
	if status, _ := resp.Body["status"].(string); status != "requested" {
		t.Fatalf("expected requested status, got %q", status)
	}
	if ref, _ := resp.Body["client_reference"].(string); ref != fixtureID(t, "MANZA_FIXTURE_CLIENT_REFERENCE") {
		t.Fatalf("expected client_reference echoed, got %q", ref)
	}
	// Inside the envelope: the challenge went to the authorizer.
	if authorization, _ := resp.Body["authorization"].(map[string]any); authorization["status"] != "pending" {
		t.Fatalf("expected a pending authorization, got %#v", resp.Body["authorization"])
	}
	if resp.Body["transfer"] != nil {
		t.Fatalf("expected nil transfer before approval, got %#v", resp.Body["transfer"])
	}
}

func TestPaymentsCreateDuplicate(t *testing.T) {
	server := startReplayServer(t, "payments/create_duplicate")
	client := replayClient(t, server)

	_, err := client.Payments.Create(context.Background(), manza.Attributes{
		"account_id":       fixtureID(t, "MANZA_FIXTURE_ACCOUNT_ID"),
		"beneficiary_id":   fixtureID(t, "MANZA_FIXTURE_BENEFICIARY_ID"),
		"amount":           "10.00",
		"client_reference": fixtureID(t, "MANZA_FIXTURE_AUTHORIZABLE_CLIENT_REFERENCE"),
	})
	var apiErr *manza.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *manza.Error, got %T (%v)", err, err)
	}
	if apiErr.Kind != manza.KindConflict || apiErr.Status != 409 {
		t.Fatalf("expected conflict/409, got %s/%d", apiErr.Kind, apiErr.Status)
	}
	if apiErr.Type != "duplicate_client_reference" {
		t.Fatalf("expected duplicate_client_reference, got %q", apiErr.Type)
	}
	if want := fixtureID(t, "MANZA_FIXTURE_AUTHORIZABLE_DRAFT_ID"); apiErr.PaymentID != want {
		t.Fatalf("expected payment_id %q, got %q", want, apiErr.PaymentID)
	}
}

func TestPaymentsGet(t *testing.T) {
	server := startReplayServer(t, "payments/get")
	client := replayClient(t, server)

	got, err := client.Payments.Get(context.Background(), fixtureID(t, "MANZA_FIXTURE_TRANSFER_DRAFT_ID"))
	if err != nil {
		t.Fatalf("payments get: %v", err)
	}
	if _, ok := got.Body["status"].(string); !ok {
		t.Fatalf("expected string status, got %#v", got.Body["status"])
	}
	if _, ok := got.Body["transfer"]; !ok {
		t.Fatal("expected a transfer key")
	}
}

func TestPaymentsAuthorizeBlankSignature(t *testing.T) {
	// No server: a blank signature must fail before any HTTP call.
	client, err := manza.New(manza.WithAPIKey("test"), manza.WithBaseURL("http://127.0.0.1:1"))
	if err != nil {
		t.Fatalf("build client: %v", err)
	}
	for _, signature := range []string{"", "   "} {
		_, err := client.Payments.Authorize(context.Background(), "draft", "auth", signature)
		var argErr *manza.ArgumentError
		if !errors.As(err, &argErr) {
			t.Fatalf("signature %q: expected *manza.ArgumentError, got %T (%v)", signature, err, err)
		}
	}
}

func TestPaymentsAuthorizeBadSignature(t *testing.T) {
	server := startReplayServer(t, "payments/authorize_bad_signature")
	client := replayClient(t, server)

	_, err := client.Payments.Authorize(context.Background(),
		fixtureID(t, "MANZA_FIXTURE_BAD_SIGNATURE_DRAFT_ID"),
		fixtureID(t, "MANZA_FIXTURE_BAD_SIGNATURE_AUTHORIZATION_ID"),
		strings.Repeat("0", 64))
	var apiErr *manza.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *manza.Error, got %T (%v)", err, err)
	}
	if apiErr.Kind != manza.KindValidation || apiErr.Type != "invalid_signature" {
		t.Fatalf("expected validation/invalid_signature, got %s/%s", apiErr.Kind, apiErr.Type)
	}
}

func TestPaymentsAuthorizeSameKey(t *testing.T) {
	server := startReplayServer(t, "payments/authorize_same_key")
	client := replayClient(t, server)

	_, err := client.Payments.Authorize(context.Background(),
		fixtureID(t, "MANZA_FIXTURE_AUTHORIZABLE_DRAFT_ID"),
		fixtureID(t, "MANZA_FIXTURE_AUTHORIZABLE_AUTHORIZATION_ID"),
		strings.Repeat("0", 64))
	var apiErr *manza.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *manza.Error, got %T (%v)", err, err)
	}
	if apiErr.Kind != manza.KindForbidden || apiErr.Type != "same_key_forbidden" {
		t.Fatalf("expected forbidden/same_key_forbidden, got %s/%s", apiErr.Kind, apiErr.Type)
	}
}

func TestPaymentsAuthorize(t *testing.T) {
	server := startReplayServer(t, "payments/authorize")
	client := replayClient(t, server)

	draftID := fixtureID(t, "MANZA_FIXTURE_AUTHORIZABLE_DRAFT_ID")
	payee, err := manza.PayeeFor(fixtureID(t, "MANZA_FIXTURE_TRUSTED_EXTERNAL_ACCOUNT_ID"), "")
	if err != nil {
		t.Fatalf("payee: %v", err)
	}
	input := manza.SignatureInput(manza.TransferAuthorizationFields{
		PaymentID:       draftID,
		Nonce:           fixtureID(t, "MANZA_FIXTURE_AUTHORIZABLE_NONCE"),
		Amount:          "10.0",
		CurrencyCode:    "MAD",
		AccountID:       fixtureID(t, "MANZA_FIXTURE_ACCOUNT_ID"),
		Payee:           payee,
		ClientReference: fixtureID(t, "MANZA_FIXTURE_AUTHORIZABLE_CLIENT_REFERENCE"),
	})

	resp, err := client.Payments.Authorize(context.Background(), draftID,
		fixtureID(t, "MANZA_FIXTURE_AUTHORIZABLE_AUTHORIZATION_ID"),
		manza.Sign("replay-secret", input))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("expected 200, got %d", resp.Status)
	}
	var draft manza.TransferDraft
	if err := resp.Decode(&draft); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if draft.ID != draftID {
		t.Fatalf("expected draft %q, got %q", draftID, draft.ID)
	}
	if draft.Authorization == nil || draft.Authorization.Status != "authorized" {
		t.Fatalf("expected authorized authorization, got %#v", draft.Authorization)
	}
	if draft.Transfer == nil || draft.Transfer.Status != "submitted" {
		t.Fatalf("expected submitted transfer, got %#v", draft.Transfer)
	}
}

func TestPaymentsGetRefused(t *testing.T) {
	server := startReplayServer(t, "payments/get_refused")
	client := replayClient(t, server)

	resp, err := client.Payments.Get(context.Background(), fixtureID(t, "MANZA_FIXTURE_REFUSED_DRAFT_ID"))
	if err != nil {
		t.Fatalf("payments get: %v", err)
	}
	var payment manza.TransferDraft
	if err := resp.Decode(&payment); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payment.ID != fixtureID(t, "MANZA_FIXTURE_REFUSED_DRAFT_ID") {
		t.Fatalf("unexpected id %q", payment.ID)
	}
	if payment.Authorization == nil || payment.Authorization.Status != "refused" {
		t.Fatalf("expected a refused authorization, got %#v", payment.Authorization)
	}
	if reason := payment.Authorization.RefusalReason; reason == nil || *reason != "over_transfer_limit" {
		t.Fatalf("expected refusal_reason over_transfer_limit, got %v", reason)
	}
}

// The fresh bank account was never trusted; with "Approve transfers to new
// payees" on, the authorizer approves it and the signature trusts it.
func TestPaymentsAuthorizeNewPayee(t *testing.T) {
	server := startReplayServer(t, "payments/authorize_new_payee")
	client := replayClient(t, server)

	draftID := fixtureID(t, "MANZA_FIXTURE_NEW_PAYEE_DRAFT_ID")
	payee, err := manza.PayeeFor(fixtureID(t, "MANZA_FIXTURE_EXTERNAL_ACCOUNT_ID"), "")
	if err != nil {
		t.Fatalf("payee: %v", err)
	}
	input := manza.SignatureInput(manza.TransferAuthorizationFields{
		PaymentID:       draftID,
		Nonce:           fixtureID(t, "MANZA_FIXTURE_NEW_PAYEE_NONCE"),
		Amount:          "10.0",
		CurrencyCode:    "MAD",
		AccountID:       fixtureID(t, "MANZA_FIXTURE_ACCOUNT_ID"),
		Payee:           payee,
		ClientReference: fixtureID(t, "MANZA_FIXTURE_NEW_PAYEE_CLIENT_REFERENCE"),
	})

	resp, err := client.Payments.Authorize(context.Background(), draftID,
		fixtureID(t, "MANZA_FIXTURE_NEW_PAYEE_AUTHORIZATION_ID"),
		manza.Sign("replay-secret", input))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	var payment manza.TransferDraft
	if err := resp.Decode(&payment); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != 200 || payment.ID != draftID {
		t.Fatalf("expected 200 for %q, got %d for %q", draftID, resp.Status, payment.ID)
	}
	if payment.Authorization == nil || payment.Authorization.Status != "authorized" || payment.Authorization.RefusalReason != nil {
		t.Fatalf("expected an authorized authorization without refusal_reason, got %#v", payment.Authorization)
	}
}

// TransferDrafts is the deprecated alias of Payments: the same service, so it
// calls /api/payments too.
func TestTransferDraftsIsAnAliasOfPayments(t *testing.T) {
	server := startReplayServer(t, "payments/get")
	client := replayClient(t, server)

	if client.TransferDrafts != client.Payments {
		t.Fatal("expected TransferDrafts to be the Payments service")
	}
	var service *manza.TransferDraftsService = client.Payments
	resp, err := service.Get(context.Background(), fixtureID(t, "MANZA_FIXTURE_TRANSFER_DRAFT_ID"))
	if err != nil {
		t.Fatalf("transfer drafts get: %v", err)
	}
	if _, ok := resp.Body["status"].(string); !ok {
		t.Fatalf("expected string status, got %#v", resp.Body["status"])
	}
}

func TestPaymentsDecline(t *testing.T) {
	server := startReplayServer(t, "payments/decline")
	client := replayClient(t, server)

	resp, err := client.Payments.Decline(context.Background(),
		fixtureID(t, "MANZA_FIXTURE_DECLINABLE_DRAFT_ID"),
		fixtureID(t, "MANZA_FIXTURE_DECLINABLE_AUTHORIZATION_ID"),
		"SDK fixture")
	if err != nil {
		t.Fatalf("decline: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("expected 200, got %d", resp.Status)
	}
	var authorization manza.Authorization
	if err := resp.Decode(&authorization); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if authorization.ID != fixtureID(t, "MANZA_FIXTURE_DECLINABLE_AUTHORIZATION_ID") || authorization.Status != "declined" {
		t.Fatalf("unexpected authorization %#v", authorization)
	}
	if authorization.DeclinedAt == nil || authorization.AuthorizedAt != nil {
		t.Fatalf("expected declined_at set and authorized_at nil, got %#v", authorization)
	}
}

func TestPaymentsDeclineOmitsAbsentReason(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got = string(raw)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)

	if _, err := replayClient(t, server).Payments.Decline(context.Background(), "d", "a", ""); err != nil {
		t.Fatalf("decline: %v", err)
	}
	if got != `{"authorization_id":"a"}` {
		t.Fatalf("expected reason omitted, got %s", got)
	}
}

func TestBeneficiaries(t *testing.T) {
	server := startReplayServer(t, "beneficiaries/list", "beneficiaries/get")
	client := replayClient(t, server)
	ctx := context.Background()

	page, err := client.Beneficiaries.List(ctx, manza.ListParams{})
	if err != nil {
		t.Fatalf("beneficiaries list: %v", err)
	}
	if len(page.Data) == 0 {
		t.Fatal("expected at least one beneficiary")
	}
	if _, ok := page.Data[0]["external_accounts"].([]any); !ok {
		t.Fatalf("expected embedded external_accounts, got %#v", page.Data[0]["external_accounts"])
	}

	resp, err := client.Beneficiaries.Get(ctx, fixtureID(t, "MANZA_FIXTURE_BENEFICIARY_ID"))
	if err != nil {
		t.Fatalf("beneficiaries get: %v", err)
	}
	if _, ok := resp.Body["id"].(string); !ok {
		t.Fatalf("expected string id, got %#v", resp.Body["id"])
	}
}

func TestBeneficiariesCreate(t *testing.T) {
	server := startReplayServer(t, "beneficiaries/create")
	client := replayClient(t, server)

	resp, err := client.Beneficiaries.Create(context.Background(), manza.Attributes{
		"beneficiary_type": "business",
		"company_name":     "Manza Fixture Beneficiary spec",
		"email":            "fixture-beneficiary-spec@example.com",
	})
	if err != nil {
		t.Fatalf("beneficiaries create: %v", err)
	}
	if resp.Status != 201 {
		t.Fatalf("expected 201, got %d", resp.Status)
	}
	var beneficiary manza.Beneficiary
	if err := resp.Decode(&beneficiary); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if beneficiary.BeneficiaryType != manza.BeneficiaryTypeBusiness {
		t.Fatalf("expected business, got %q", beneficiary.BeneficiaryType)
	}
	if beneficiary.ExternalAccounts == nil || len(beneficiary.ExternalAccounts) != 0 {
		t.Fatalf("expected empty external_accounts, got %#v", beneficiary.ExternalAccounts)
	}
}

func TestBeneficiariesListExternalAccounts(t *testing.T) {
	server := startReplayServer(t, "beneficiaries/list_external_accounts")
	client := replayClient(t, server)

	page, err := client.Beneficiaries.ListExternalAccounts(context.Background(),
		fixtureID(t, "MANZA_FIXTURE_CREATED_BENEFICIARY_ID"), manza.ListParams{})
	if err != nil {
		t.Fatalf("list external accounts: %v", err)
	}
	if page.HasMore {
		t.Fatal("expected last page")
	}
	if len(page.Data) != 1 {
		t.Fatalf("expected one row, got %d", len(page.Data))
	}
	if id, _ := page.Data[0]["id"].(string); id != fixtureID(t, "MANZA_FIXTURE_EXTERNAL_ACCOUNT_ID") {
		t.Fatalf("unexpected id %q", id)
	}
	if _, ok := page.Data[0]["account_number"].(string); !ok {
		t.Fatalf("expected string account_number, got %#v", page.Data[0]["account_number"])
	}
}

func TestBeneficiariesGetExternalAccount(t *testing.T) {
	server := startReplayServer(t, "beneficiaries/get_external_account")
	client := replayClient(t, server)

	resp, err := client.Beneficiaries.GetExternalAccount(context.Background(),
		fixtureID(t, "MANZA_FIXTURE_CREATED_BENEFICIARY_ID"), fixtureID(t, "MANZA_FIXTURE_EXTERNAL_ACCOUNT_ID"))
	if err != nil {
		t.Fatalf("get external account: %v", err)
	}
	var account manza.ExternalAccount
	if err := resp.Decode(&account); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if account.ID != fixtureID(t, "MANZA_FIXTURE_EXTERNAL_ACCOUNT_ID") {
		t.Fatalf("unexpected id %q", account.ID)
	}
	if _, ok := resp.Body["default"]; !ok {
		t.Fatal("expected a default key")
	}
}

// Recorded after the new-payee authorize: its signature trusted the payee.
func TestBeneficiariesGetExternalAccountTrusted(t *testing.T) {
	server := startReplayServer(t, "beneficiaries/get_external_account_trusted")
	client := replayClient(t, server)

	resp, err := client.Beneficiaries.GetExternalAccount(context.Background(),
		fixtureID(t, "MANZA_FIXTURE_CREATED_BENEFICIARY_ID"), fixtureID(t, "MANZA_FIXTURE_EXTERNAL_ACCOUNT_ID"))
	if err != nil {
		t.Fatalf("get external account: %v", err)
	}
	var account manza.ExternalAccount
	if err := resp.Decode(&account); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if account.ID != fixtureID(t, "MANZA_FIXTURE_EXTERNAL_ACCOUNT_ID") || !account.Trusted {
		t.Fatalf("expected the trusted account, got %#v", account)
	}
}

func TestBeneficiariesCreateExternalAccount(t *testing.T) {
	server := startReplayServer(t, "beneficiaries/create_external_account")
	client := replayClient(t, server)

	resp, err := client.Beneficiaries.CreateExternalAccount(context.Background(),
		fixtureID(t, "MANZA_FIXTURE_CREATED_BENEFICIARY_ID"), manza.Attributes{
			"account_number": fixtureID(t, "MANZA_FIXTURE_NEW_ACCOUNT_NUMBER"),
			"name":           "Fixture Secondary Account",
		})
	if err != nil {
		t.Fatalf("create external account: %v", err)
	}
	if resp.Status != 201 {
		t.Fatalf("expected 201, got %d", resp.Status)
	}
	var account manza.ExternalAccount
	if err := resp.Decode(&account); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if account.Name != "Fixture Secondary Account" || account.Default {
		t.Fatalf("unexpected account %#v", account)
	}
}

func TestPayeeTrustRequests(t *testing.T) {
	server := startReplayServer(t, "payee_trust_requests/create", "payee_trust_requests/get")
	client := replayClient(t, server)
	ctx := context.Background()

	externalAccountID := fixtureID(t, "MANZA_FIXTURE_EXTERNAL_ACCOUNT_ID")
	resp, err := client.PayeeTrustRequests.Create(ctx, []string{externalAccountID})
	if err != nil {
		t.Fatalf("payee trust requests create: %v", err)
	}
	if resp.Status != 201 {
		t.Fatalf("expected 201, got %d", resp.Status)
	}
	var created manza.PayeeTrustRequest
	if err := resp.Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Status != "pending" || len(created.ExternalAccountIDs) != 1 || created.ExternalAccountIDs[0] != externalAccountID {
		t.Fatalf("unexpected trust request %#v", created)
	}

	id := fixtureID(t, "MANZA_FIXTURE_PAYEE_TRUST_REQUEST_ID")
	got, err := client.PayeeTrustRequests.Get(ctx, id)
	if err != nil {
		t.Fatalf("payee trust requests get: %v", err)
	}
	var fetched manza.PayeeTrustRequest
	if err := got.Decode(&fetched); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if fetched.ID != id || fetched.ResolvedAt != nil {
		t.Fatalf("unexpected trust request %#v", fetched)
	}
}

func TestTransferAuthorizerGet(t *testing.T) {
	server := startReplayServer(t, "transfer_authorizer/get")
	client := replayClient(t, server)

	resp, err := client.TransferAuthorizer.Get(context.Background())
	if err != nil {
		t.Fatalf("transfer authorizer get: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("expected 200, got %d", resp.Status)
	}
	for _, key := range []string{"per_transfer_limit", "daily_limit", "daily_authorized_total", "secret_expires_at", "webhook_endpoint_id", "enrolled_at"} {
		if _, ok := resp.Body[key]; !ok {
			t.Fatalf("expected a %s key in %#v", key, resp.Body)
		}
	}
	var authorizer manza.TransferAuthorizer
	if err := resp.Decode(&authorizer); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if authorizer.Status != "active" || authorizer.SuspensionReason != nil || authorizer.CurrencyCode != "MAD" {
		t.Fatalf("unexpected authorizer %#v", authorizer)
	}
	if authorizer.PerTransferLimit == "" || authorizer.DailyLimit == "" || authorizer.DailyAuthorizedTotal == "" ||
		authorizer.SecretExpiresAt == "" || authorizer.WebhookEndpointID == "" || authorizer.EnrolledAt == "" {
		t.Fatalf("expected every field set, got %#v", authorizer)
	}
}

func TestTransferAuthorizerGetNotEnrolled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/transfer_authorizer" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"type":"not_found_error","message":"No transfer authorizer is enrolled"}}`))
	}))
	t.Cleanup(server.Close)

	_, err := replayClient(t, server).TransferAuthorizer.Get(context.Background())
	var apiErr *manza.Error
	if !errors.As(err, &apiErr) || apiErr.Kind != manza.KindNotFound {
		t.Fatalf("expected a not_found *manza.Error, got %T (%v)", err, err)
	}
}
