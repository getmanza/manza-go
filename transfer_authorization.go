package manza

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// signatureVersion prefixes every signature input. The API verifies v2 for
// every request that is unpinned or sends `Manza-Version: 2026-11-13` or
// later; a client pinned to `Manza-Version: 2026-03-27` must sign v1 and
// stays on manza-go 1.x.
const signatureVersion = "manza.transfer-authorization.v2"

// TransferAuthorizationFields are the inputs to a machine-authorization
// signature. Build them from your OWN record of the transfer, not from the
// webhook's `signature_input_v2` (which is there only to compare against).
type TransferAuthorizationFields struct {
	PaymentID    string
	Nonce        string // one-time, from the payment.authorization_requested webhook
	Amount       string // the API's decimal string verbatim, e.g. "2500.0"
	CurrencyCode string
	AccountID    string
	Payee        string // see PayeeFor
	// ClientReference is empty when the transfer has none.
	ClientReference string
	// TransferMethod is the payment's rail as the API stores it: the create
	// response's `transfer_method` ("standard", "eft", ...). Always set it
	// from your record: v2 signs it, and a draft that carries one does not
	// verify against an input left empty. Empty only where the API returned
	// null (a market with no rail choice).
	TransferMethod string
}

// SignatureInput builds the pipe-joined, versioned string that Sign signs:
//
//	manza.transfer-authorization.v2|<payment_id>|<nonce>|<amount>|<currency_code>|<account_id>|<payee>|<client_reference or "">|<transfer_method or "">
//
// Amount must be the API's decimal string verbatim ("2500.0", not "2500.00"
// or a formatted float), otherwise the server's signature will not match.
func SignatureInput(f TransferAuthorizationFields) string {
	return strings.Join([]string{
		signatureVersion, f.PaymentID, f.Nonce, f.Amount, f.CurrencyCode, f.AccountID, f.Payee, f.ClientReference,
		f.TransferMethod,
	}, "|")
}

// Sign returns the lowercase hex HMAC-SHA256 of signatureInput under the
// authorizer endpoint's signing secret. Pass the result to
// PaymentsService.Authorize.
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
