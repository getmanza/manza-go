package manza

import (
	"context"
	"net/url"
	"strings"
)

// Attributes is a request body for create/update calls — snake_case keys,
// exactly what the API accepts (see the per-endpoint docs).
type Attributes map[string]any

func setIfPresent(v url.Values, key, value string) {
	if value != "" {
		v.Set(key, value)
	}
}

// AccountsService — accounts and their transactions.
type AccountsService struct{ client *Client }

// AccountListParams filters GET /api/accounts.
type AccountListParams struct {
	ListParams
	Status       string
	CurrencyCode string
}

// List calls GET /api/accounts.
func (s *AccountsService) List(ctx context.Context, params AccountListParams) (*Page, error) {
	base := url.Values{}
	setIfPresent(base, "status", params.Status)
	setIfPresent(base, "currency_code", params.CurrencyCode)
	return s.client.listPage(ctx, "api/accounts", base, params.ListParams)
}

// Get calls GET /api/accounts/:id.
func (s *AccountsService) Get(ctx context.Context, id string) (*Response, error) {
	return s.client.get(ctx, encodePath("api/accounts", id), nil)
}

// TransactionListParams filters GET /api/accounts/:id/transactions.
type TransactionListParams struct {
	ListParams
	Operation    string
	PostedAfter  string // ISO-8601
	PostedBefore string // ISO-8601
}

// ListTransactions calls GET /api/accounts/:account_id/transactions.
func (s *AccountsService) ListTransactions(ctx context.Context, accountID string, params TransactionListParams) (*Page, error) {
	base := url.Values{}
	setIfPresent(base, "operation", params.Operation)
	setIfPresent(base, "posted_after", params.PostedAfter)
	setIfPresent(base, "posted_before", params.PostedBefore)
	return s.client.listPage(ctx, encodePath("api/accounts", accountID, "transactions"), base, params.ListParams)
}

// GetTransaction calls GET /api/accounts/:account_id/transactions/:id.
func (s *AccountsService) GetTransaction(ctx context.Context, accountID, transactionID string) (*Response, error) {
	return s.client.get(ctx, encodePath("api/accounts", accountID, "transactions", transactionID), nil)
}

// BeneficiariesService — saved transfer recipients. Each beneficiary embeds
// its bank accounts; the one flagged `default` is used when a transfer names
// only the beneficiary_id. There is no update or delete via the API.
type BeneficiariesService struct{ client *Client }

// List calls GET /api/beneficiaries.
func (s *BeneficiariesService) List(ctx context.Context, params ListParams) (*Page, error) {
	return s.client.listPage(ctx, "api/beneficiaries", nil, params)
}

// Get calls GET /api/beneficiaries/:id.
func (s *BeneficiariesService) Get(ctx context.Context, id string) (*Response, error) {
	return s.client.get(ctx, encodePath("api/beneficiaries", id), nil)
}

// Create calls POST /api/beneficiaries.
// Attributes: beneficiary_type ("individual" | "business"; inferred from
// person_name / company_name when omitted), person_name, company_name, email,
// phone_number. Values must be strings. Shares a 10/minute limit with
// CreateExternalAccount.
func (s *BeneficiariesService) Create(ctx context.Context, attributes Attributes) (*Response, error) {
	return s.client.post(ctx, "api/beneficiaries", attributes)
}

// ListExternalAccounts calls GET /api/beneficiaries/:beneficiary_id/external_accounts.
func (s *BeneficiariesService) ListExternalAccounts(ctx context.Context, beneficiaryID string, params ListParams) (*Page, error) {
	return s.client.listPage(ctx, encodePath("api/beneficiaries", beneficiaryID, "external_accounts"), nil, params)
}

// GetExternalAccount calls GET /api/beneficiaries/:beneficiary_id/external_accounts/:id.
func (s *BeneficiariesService) GetExternalAccount(ctx context.Context, beneficiaryID, id string) (*Response, error) {
	return s.client.get(ctx, encodePath("api/beneficiaries", beneficiaryID, "external_accounts", id), nil)
}

// CreateExternalAccount calls POST /api/beneficiaries/:beneficiary_id/external_accounts.
// Required: account_number. Optional: name, country_code, currency_code,
// account_type ("bank" only), bank_identifier (required in ZA, rejected in
// MA, where it is derived from the RIB).
func (s *BeneficiariesService) CreateExternalAccount(ctx context.Context, beneficiaryID string, attributes Attributes) (*Response, error) {
	return s.client.post(ctx, encodePath("api/beneficiaries", beneficiaryID, "external_accounts"), attributes)
}

// CheckoutSessionsService — one-off hosted checkout sessions. No list,
// update, or delete; sessions are created and inspected by id.
type CheckoutSessionsService struct{ client *Client }

// Create calls POST /api/checkout_sessions.
// Required attributes: account_id, amount, success_url. Optional: cancel_url,
// description, customer_email, customer_name, metadata, collect_billing_address,
// billing_address. The response carries settled_at and transaction once the
// session has cleared (status "clearing" → "complete").
func (s *CheckoutSessionsService) Create(ctx context.Context, attributes Attributes) (*Response, error) {
	return s.client.post(ctx, "api/checkout_sessions", attributes)
}

// Get calls GET /api/checkout_sessions/:id.
func (s *CheckoutSessionsService) Get(ctx context.Context, id string) (*Response, error) {
	return s.client.get(ctx, encodePath("api/checkout_sessions", id), nil)
}

// CustomersService — individuals or businesses the entity invoices.
type CustomersService struct{ client *Client }

// CustomerListParams filters GET /api/customers.
type CustomerListParams struct {
	ListParams
	Query string // matches company name, person name, email
}

// List calls GET /api/customers.
func (s *CustomersService) List(ctx context.Context, params CustomerListParams) (*Page, error) {
	base := url.Values{}
	setIfPresent(base, "q", params.Query)
	return s.client.listPage(ctx, "api/customers", base, params.ListParams)
}

// Get calls GET /api/customers/:id.
func (s *CustomersService) Get(ctx context.Context, id string) (*Response, error) {
	return s.client.get(ctx, encodePath("api/customers", id), nil)
}

// Create calls POST /api/customers.
func (s *CustomersService) Create(ctx context.Context, attributes Attributes) (*Response, error) {
	return s.client.post(ctx, "api/customers", attributes)
}

// Update calls PATCH /api/customers/:id.
func (s *CustomersService) Update(ctx context.Context, id string, attributes Attributes) (*Response, error) {
	return s.client.patch(ctx, encodePath("api/customers", id), attributes)
}

// Delete calls DELETE /api/customers/:id.
func (s *CustomersService) Delete(ctx context.Context, id string) (*Response, error) {
	return s.client.delete(ctx, encodePath("api/customers", id))
}

// EntityService — the current entity (the tenant the API key belongs to).
type EntityService struct{ client *Client }

// Get calls GET /api/entity.
func (s *EntityService) Get(ctx context.Context) (*Response, error) {
	return s.client.get(ctx, "api/entity", nil)
}

// InvoicesService — invoices and their lifecycle actions.
type InvoicesService struct{ client *Client }

// InvoiceListParams filters GET /api/invoices.
type InvoiceListParams struct {
	ListParams
	Status     string
	CustomerID string
}

// List calls GET /api/invoices.
func (s *InvoicesService) List(ctx context.Context, params InvoiceListParams) (*Page, error) {
	base := url.Values{}
	setIfPresent(base, "status", params.Status)
	setIfPresent(base, "customer_id", params.CustomerID)
	return s.client.listPage(ctx, "api/invoices", base, params.ListParams)
}

// Get calls GET /api/invoices/:id.
func (s *InvoicesService) Get(ctx context.Context, id string) (*Response, error) {
	return s.client.get(ctx, encodePath("api/invoices", id), nil)
}

// Create calls POST /api/invoices.
func (s *InvoicesService) Create(ctx context.Context, attributes Attributes) (*Response, error) {
	return s.client.post(ctx, "api/invoices", attributes)
}

// Update calls PATCH /api/invoices/:id.
func (s *InvoicesService) Update(ctx context.Context, id string, attributes Attributes) (*Response, error) {
	return s.client.patch(ctx, encodePath("api/invoices", id), attributes)
}

// Send calls POST /api/invoices/:id/send.
func (s *InvoicesService) Send(ctx context.Context, id string) (*Response, error) {
	return s.client.post(ctx, encodePath("api/invoices", id, "send"), nil)
}

// MarkAsPaid calls POST /api/invoices/:id/mark_as_paid.
func (s *InvoicesService) MarkAsPaid(ctx context.Context, id string) (*Response, error) {
	return s.client.post(ctx, encodePath("api/invoices", id, "mark_as_paid"), nil)
}

// Cancel calls POST /api/invoices/:id/cancel.
func (s *InvoicesService) Cancel(ctx context.Context, id string) (*Response, error) {
	return s.client.post(ctx, encodePath("api/invoices", id, "cancel"), nil)
}

// CreditNote calls POST /api/invoices/:id/credit_note.
func (s *InvoicesService) CreditNote(ctx context.Context, id string) (*Response, error) {
	return s.client.post(ctx, encodePath("api/invoices", id, "credit_note"), nil)
}

// Delete calls DELETE /api/invoices/:id.
func (s *InvoicesService) Delete(ctx context.Context, id string) (*Response, error) {
	return s.client.delete(ctx, encodePath("api/invoices", id))
}

// CreatePaymentLink calls POST /api/invoices/:invoice_id/payment_link.
func (s *InvoicesService) CreatePaymentLink(ctx context.Context, invoiceID, accountID string) (*Response, error) {
	return s.client.post(ctx, encodePath("api/invoices", invoiceID, "payment_link"), Attributes{"account_id": accountID})
}

// PaymentLinksService — standalone payment links (not attached to an invoice).
type PaymentLinksService struct{ client *Client }

// PaymentLinkListParams filters GET /api/payment_links.
type PaymentLinkListParams struct {
	ListParams
	Status   string
	LinkType string
}

// List calls GET /api/payment_links.
func (s *PaymentLinksService) List(ctx context.Context, params PaymentLinkListParams) (*Page, error) {
	base := url.Values{}
	setIfPresent(base, "status", params.Status)
	setIfPresent(base, "link_type", params.LinkType)
	return s.client.listPage(ctx, "api/payment_links", base, params.ListParams)
}

// Get calls GET /api/payment_links/:id.
func (s *PaymentLinksService) Get(ctx context.Context, id string) (*Response, error) {
	return s.client.get(ctx, encodePath("api/payment_links", id), nil)
}

// Create calls POST /api/payment_links.
func (s *PaymentLinksService) Create(ctx context.Context, attributes Attributes) (*Response, error) {
	return s.client.post(ctx, "api/payment_links", attributes)
}

// Cancel calls POST /api/payment_links/:id/cancel.
func (s *PaymentLinksService) Cancel(ctx context.Context, id string) (*Response, error) {
	return s.client.post(ctx, encodePath("api/payment_links", id, "cancel"), nil)
}

// PayeeTrustRequestsService — requests to trust payees for
// machine-authorized transfers. The API key can only ask: a member holding
// payment-authorize permission approves the request in the Manza app. Status:
// pending → approved / declined / cancelled. No list, update, or delete.
type PayeeTrustRequestsService struct{ client *Client }

// Create calls POST /api/payee_trust_requests with at most 100 bank
// account ids.
func (s *PayeeTrustRequestsService) Create(ctx context.Context, externalAccountIDs []string) (*Response, error) {
	return s.client.post(ctx, "api/payee_trust_requests", payeeTrustRequestBody{ExternalAccountIDs: externalAccountIDs})
}

// Get calls GET /api/payee_trust_requests/:id.
func (s *PayeeTrustRequestsService) Get(ctx context.Context, id string) (*Response, error) {
	return s.client.get(ctx, encodePath("api/payee_trust_requests", id), nil)
}

type payeeTrustRequestBody struct {
	ExternalAccountIDs []string `json:"external_account_ids"`
}

// TransferDraftsService — API-initiated transfers. Creating a draft never
// executes a transfer by itself. A draft inside the entity's
// machine-authorization envelope (trusted payee, within limits) is sent to
// the enrolled transfer authorizer as a `payment.authorization_requested`
// webhook; answer it with Authorize or Decline, using an API key other than
// the one that created the draft. Every other draft goes to the in-app
// approval flow, where a manager or legal representative approves it. Poll
// Get (status: requested → processing → completed / failed) or subscribe to
// the `transfer.executed` webhook to follow execution.
type TransferDraftsService struct{ client *Client }

// Create calls POST /api/transfer_drafts.
// Required: account_id, amount, and exactly one of beneficiary_id
// (external transfer) or destination_account_id (own-account move).
// Optional: external_account_id, currency_code, payment_reference,
// internal_notes, client_reference (unique per entity, at most 128
// characters; a duplicate returns a conflict *Error whose PaymentID names
// the existing draft).
func (s *TransferDraftsService) Create(ctx context.Context, attributes Attributes) (*Response, error) {
	return s.client.post(ctx, "api/transfer_drafts", attributes)
}

// Get calls GET /api/transfer_drafts/:id.
func (s *TransferDraftsService) Get(ctx context.Context, id string) (*Response, error) {
	return s.client.get(ctx, encodePath("api/transfer_drafts", id), nil)
}

type authorizeBody struct {
	AuthorizationID string `json:"authorization_id"`
	Signature       string `json:"signature"`
}

// Authorize calls POST /api/transfer_drafts/:id/authorize.
//
// Executes the draft. authorizationID comes from the
// `payment.authorization_requested` webhook; build signature with
// SignatureInput and Sign. Requires the `transfers:authorize` scope on a key
// other than the draft's creator (otherwise a forbidden *Error of type
// `same_key_forbidden`). A blank signature is refused locally with
// *ArgumentError: the API counts it as a failed attempt, and five fail the
// challenge.
func (s *TransferDraftsService) Authorize(ctx context.Context, id, authorizationID, signature string) (*Response, error) {
	if strings.TrimSpace(signature) == "" {
		return nil, &ArgumentError{Message: "signature cannot be blank"}
	}
	return s.client.post(ctx, encodePath("api/transfer_drafts", id, "authorize"),
		authorizeBody{AuthorizationID: authorizationID, Signature: signature})
}

type declineBody struct {
	AuthorizationID string `json:"authorization_id"`
	Reason          string `json:"reason,omitempty"`
}

// Decline calls POST /api/transfer_drafts/:id/decline.
//
// Declines the challenge and deletes the draft; the response is the
// authorization (status "declined"). An empty reason is omitted from the
// request.
func (s *TransferDraftsService) Decline(ctx context.Context, id, authorizationID, reason string) (*Response, error) {
	return s.client.post(ctx, encodePath("api/transfer_drafts", id, "decline"),
		declineBody{AuthorizationID: authorizationID, Reason: reason})
}

// WebhookEndpointsService — webhook endpoint management.
type WebhookEndpointsService struct{ client *Client }

// List calls GET /api/webhook_endpoints.
func (s *WebhookEndpointsService) List(ctx context.Context, params ListParams) (*Page, error) {
	return s.client.listPage(ctx, "api/webhook_endpoints", nil, params)
}

// Get calls GET /api/webhook_endpoints/:id.
func (s *WebhookEndpointsService) Get(ctx context.Context, id string) (*Response, error) {
	return s.client.get(ctx, encodePath("api/webhook_endpoints", id), nil)
}

// Create calls POST /api/webhook_endpoints.
func (s *WebhookEndpointsService) Create(ctx context.Context, attributes Attributes) (*Response, error) {
	return s.client.post(ctx, "api/webhook_endpoints", attributes)
}

// Update calls PATCH /api/webhook_endpoints/:id.
func (s *WebhookEndpointsService) Update(ctx context.Context, id string, attributes Attributes) (*Response, error) {
	return s.client.patch(ctx, encodePath("api/webhook_endpoints", id), attributes)
}

// Delete calls DELETE /api/webhook_endpoints/:id.
func (s *WebhookEndpointsService) Delete(ctx context.Context, id string) (*Response, error) {
	return s.client.delete(ctx, encodePath("api/webhook_endpoints", id))
}

// Test calls POST /api/webhook_endpoints/:id/test.
func (s *WebhookEndpointsService) Test(ctx context.Context, id string) (*Response, error) {
	return s.client.post(ctx, encodePath("api/webhook_endpoints", id, "test"), nil)
}

// RegenerateSecret calls POST /api/webhook_endpoints/:id/regenerate_secret.
func (s *WebhookEndpointsService) RegenerateSecret(ctx context.Context, id string) (*Response, error) {
	return s.client.post(ctx, encodePath("api/webhook_endpoints", id, "regenerate_secret"), nil)
}

// Enable calls POST /api/webhook_endpoints/:id/enable.
func (s *WebhookEndpointsService) Enable(ctx context.Context, id string) (*Response, error) {
	return s.client.post(ctx, encodePath("api/webhook_endpoints", id, "enable"), nil)
}

// Disable calls POST /api/webhook_endpoints/:id/disable.
func (s *WebhookEndpointsService) Disable(ctx context.Context, id string) (*Response, error) {
	return s.client.post(ctx, encodePath("api/webhook_endpoints", id, "disable"), nil)
}
