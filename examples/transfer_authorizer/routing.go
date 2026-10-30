package transferauthorizer

import "fmt"

// RefusalReasons are the reasons a payment can be refused when it is
// created, in words. The transfer authorization docs' troubleshooting table
// has the fix for each.
var RefusalReasons = map[string]string{
	"untrusted_payee": "the payee's bank details are not trusted, and \"Approve transfers to new payees\" is off " +
		"or their trust was revoked or they were edited",
	"unrestricted_api_key": "the payee is new and key A has no IP allowlist",
	"over_new_payee_limit": "the payee is new and the amount is over the limit per transfer to a new payee",
	"over_transfer_limit":  "the amount is over the limit per transfer",
	"over_daily_limit":     "this amount plus the last 24 hours' authorized total is over the limit per 24 hours",
	"currency_mismatch":    "the account's currency is not the enrolled currency",
	"suspended":            "transfer authorization is suspended, or its webhook endpoint is disabled",
	"secret_expired":       "the signing secret has expired",
}

// Describe says where the API routed a new payment, from its
// `authorization`: pending means the challenge went to the authorizer;
// refused, with a `refusal_reason`, means it went to your in-app approvers.
func Describe(authorization map[string]any) string {
	if authorization == nil {
		return "no transfer authorizer was enrolled (or the feature is off): sent to your approvers"
	}
	if authorization["status"] == "pending" {
		return "challenge sent, waiting for the authorizer: watch the authorizer terminal"
	}
	reason, _ := authorization["refusal_reason"].(string)
	words, ok := RefusalReasons[reason]
	if !ok {
		words = "see the transfer authorization docs' troubleshooting table"
	}
	return fmt.Sprintf("sent to your approvers, not the authorizer: %v (%s), %s", authorization["status"], reason, words)
}
