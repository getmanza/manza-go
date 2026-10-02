package zazu

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// signatureVersion prefixes every signature input.
const signatureVersion = "manza.transfer-authorization.v1"

// TransferAuthorizationFields are the inputs to a machine-authorization
// signature. Build them from your OWN record of the transfer, not from the
// webhook's `signature_input` (which is there only to compare against).
type TransferAuthorizationFields struct {
	PaymentID    string
	Nonce        string // one-time, from the payment.authorization_requested webhook
	Amount       string // the API's decimal string verbatim, e.g. "2500.0"
	CurrencyCode string
	AccountID    string
	Payee        string // see PayeeFor
	// ClientReference is empty when the transfer has none.
	ClientReference string
}

// SignatureInput builds the pipe-joined, versioned string that Sign signs:
//
//	manza.transfer-authorization.v1|<payment_id>|<nonce>|<amount>|<currency_code>|<account_id>|<payee>|<client_reference or "">
//
// Amount must be the API's decimal string verbatim ("2500.0", not "2500.00"
// or a formatted float), otherwise the server's signature will not match.
func SignatureInput(f TransferAuthorizationFields) string {
	return strings.Join([]string{
		signatureVersion, f.PaymentID, f.Nonce, f.Amount, f.CurrencyCode, f.AccountID, f.Payee, f.ClientReference,
	}, "|")
}

// Sign returns the lowercase hex HMAC-SHA256 of signatureInput under the
// authorizer endpoint's signing secret. Pass the result to
// TransferDraftsService.Authorize.
func Sign(secret, signatureInput string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signatureInput))
	return hex.EncodeToString(mac.Sum(nil))
}

// PayeeFor returns the payee token: `ext:<id>` for a beneficiary's bank
// account, `own:<id>` for one of the entity's own accounts. Pass exactly one
// id; the other must be empty.
func PayeeFor(externalAccountID, destinationAccountID string) (string, error) {
	if (externalAccountID == "") == (destinationAccountID == "") {
		return "", &ArgumentError{Message: "pass exactly one of externalAccountID or destinationAccountID"}
	}
	if destinationAccountID != "" {
		return "own:" + destinationAccountID, nil
	}
	return "ext:" + externalAccountID, nil
}
