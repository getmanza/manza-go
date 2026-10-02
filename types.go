package zazu

// Typed response models. Decode a *Response onto one with Response.Decode.
// Field names are the API's snake_case keys. Pointers mark values that can
// be null or absent; timestamps and decimal amounts stay strings, exactly as
// the API sends them (amounts must be passed verbatim to SignatureInput).

// StatusClearing is the checkout-session and payment-link status between a
// customer's payment and settlement; settled_at is set once it clears.
const StatusClearing = "clearing"

// BeneficiaryType values.
const (
	BeneficiaryTypeIndividual = "individual"
	BeneficiaryTypeBusiness   = "business"
)

// Authorization is a machine-authorization challenge on a transfer draft,
// and the response of TransferDraftsService.Decline.
type Authorization struct {
	ID           string  `json:"id"`
	Status       string  `json:"status"` // pending, authorized, declined, ...
	ExpiresAt    string  `json:"expires_at"`
	AuthorizedAt *string `json:"authorized_at,omitempty"`
	DeclinedAt   *string `json:"declined_at,omitempty"`
}

// TransferRef is the executed transfer attached to an authorized draft.
type TransferRef struct {
	ID     string `json:"id"`
	Status string `json:"status"` // e.g. submitted
}

// TransferDraft is an API-initiated transfer.
type TransferDraft struct {
	ID                   string         `json:"id"`
	Status               string         `json:"status"`
	CurrencyCode         string         `json:"currency_code"`
	PaymentReference     *string        `json:"payment_reference"`
	ClientReference      *string        `json:"client_reference"`
	AccountID            string         `json:"account_id"`
	BeneficiaryID        *string        `json:"beneficiary_id"`
	ExternalAccountID    *string        `json:"external_account_id"`
	DestinationAccountID *string        `json:"destination_account_id"`
	Amount               string         `json:"amount"`
	Transfer             *TransferRef   `json:"transfer"`
	Authorization        *Authorization `json:"authorization"`
	CreatedAt            string         `json:"created_at"`
	UpdatedAt            string         `json:"updated_at"`
}

// ExternalAccount is a beneficiary's bank account.
type ExternalAccount struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	AccountNumber  string `json:"account_number"`
	BankIdentifier string `json:"bank_identifier"`
	CurrencyCode   string `json:"currency_code"`
	Default        bool   `json:"default"`
}

// Beneficiary is a saved transfer recipient.
type Beneficiary struct {
	ID               string            `json:"id"`
	BeneficiaryType  string            `json:"beneficiary_type"` // individual | business
	Name             string            `json:"name"`
	Email            *string           `json:"email"`
	PhoneNumber      *string           `json:"phone_number"`
	ExternalAccounts []ExternalAccount `json:"external_accounts"`
	CreatedAt        string            `json:"created_at"`
}

// PayeeTrustRequest is a request to trust payees for machine-authorized
// transfers.
type PayeeTrustRequest struct {
	ID                 string   `json:"id"`
	Status             string   `json:"status"` // pending, approved, declined, cancelled
	ExternalAccountIDs []string `json:"external_account_ids"`
	CreatedAt          string   `json:"created_at"`
	ResolvedAt         *string  `json:"resolved_at"`
}

// CheckoutSession is a one-off hosted checkout session. Status is open,
// clearing (StatusClearing), or a terminal state.
type CheckoutSession struct {
	ID                    string         `json:"id"`
	URL                   string         `json:"url"`
	Status                string         `json:"status"`
	Amount                string         `json:"amount"`
	CurrencyCode          string         `json:"currency_code"`
	Description           *string        `json:"description"`
	CustomerEmail         *string        `json:"customer_email"`
	CustomerName          *string        `json:"customer_name"`
	CollectBillingAddress *bool          `json:"collect_billing_address"`
	BillingAddress        map[string]any `json:"billing_address"`
	SuccessURL            string         `json:"success_url"`
	CancelURL             *string        `json:"cancel_url"`
	Metadata              map[string]any `json:"metadata"`
	ExpiresAt             string         `json:"expires_at"`
	CompletedAt           *string        `json:"completed_at"`
	SettledAt             *string        `json:"settled_at"`
	CreatedAt             string         `json:"created_at"`
	Transaction           map[string]any `json:"transaction"`
}

// PaymentLink is a standalone payment link. Status is active, clearing
// (StatusClearing), or a terminal state.
type PaymentLink struct {
	ID                    string         `json:"id"`
	Slug                  string         `json:"slug"`
	LinkType              string         `json:"link_type"`
	Status                string         `json:"status"`
	Amount                *string        `json:"amount"`
	CurrencyCode          string         `json:"currency_code"`
	Title                 *string        `json:"title"`
	Description           *string        `json:"description"`
	PaymentReference      *string        `json:"payment_reference"`
	PaymentsCount         int            `json:"payments_count"`
	MaxPayments           *int           `json:"max_payments"`
	RedirectURL           *string        `json:"redirect_url"`
	InvoiceID             *string        `json:"invoice_id"`
	CollectBillingAddress *bool          `json:"collect_billing_address"`
	BillingAddress        map[string]any `json:"billing_address"`
	URL                   string         `json:"url"`
	ExpiresAt             *string        `json:"expires_at"`
	PaidAt                *string        `json:"paid_at"`
	SettledAt             *string        `json:"settled_at"`
	CreatedAt             string         `json:"created_at"`
	UpdatedAt             string         `json:"updated_at"`
}

// Customer is an individual or business the entity invoices. TaxID and
// ICENumber are market-gated (absent outside Morocco); RegistrationNumber
// and VATNumber are absent where the entity's market does not collect them.
type Customer struct {
	ID                 string  `json:"id"`
	CustomerType       string  `json:"customer_type"`
	Name               string  `json:"name"`
	PersonName         *string `json:"person_name"`
	CompanyName        *string `json:"company_name"`
	Email              *string `json:"email"`
	Phone              *string `json:"phone"`
	RegistrationNumber *string `json:"registration_number,omitempty"`
	VATNumber          *string `json:"vat_number,omitempty"`
	TaxID              *string `json:"tax_id,omitempty"`
	ICENumber          *string `json:"ice_number,omitempty"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

// Invoice carries the headline fields; use Response.Body for the rest.
// DeliveryDate is market-gated: the key is absent outside Morocco.
type Invoice struct {
	ID           string  `json:"id"`
	Number       string  `json:"number"`
	Status       string  `json:"status"`
	CurrencyCode string  `json:"currency_code"`
	IssueDate    string  `json:"issue_date"`
	DueDate      string  `json:"due_date"`
	DeliveryDate *string `json:"delivery_date,omitempty"`
	Total        string  `json:"total"`
}
