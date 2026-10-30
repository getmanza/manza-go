package transferauthorizer_test

import (
	"strings"
	"testing"

	transferauthorizer "github.com/getmanza/manza-go/examples/transfer_authorizer"
)

func expectContains(t *testing.T, line string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(line, fragment) {
			t.Fatalf("expected %q in %q", fragment, line)
		}
	}
}

func TestDescribeSaysTheChallengeWentToTheAuthorizerWhenPending(t *testing.T) {
	expectContains(t, transferauthorizer.Describe(map[string]any{"status": "pending"}), "challenge sent")
}

func TestDescribeExplainsEveryCreateTimeRefusalReason(t *testing.T) {
	expected := map[string]string{
		"untrusted_payee":      "bank details are not trusted",
		"unrestricted_api_key": "key A has no IP allowlist",
		"over_new_payee_limit": "limit per transfer to a new payee",
		"over_transfer_limit":  "over the limit per transfer",
		"over_daily_limit":     "limit per 24 hours",
		"currency_mismatch":    "not the enrolled currency",
		"suspended":            "is suspended",
		"secret_expired":       "secret has expired",
	}
	if len(transferauthorizer.RefusalReasons) != len(expected) {
		t.Fatalf("expected %d reasons, got %d", len(expected), len(transferauthorizer.RefusalReasons))
	}
	for reason, words := range expected {
		line := transferauthorizer.Describe(map[string]any{"status": "refused", "refusal_reason": reason})
		expectContains(t, line, "approvers", "refused", reason, words)
	}
}

func TestDescribePointsAtTheDocsForAnUnknownReason(t *testing.T) {
	line := transferauthorizer.Describe(map[string]any{"status": "refused", "refusal_reason": "something_new"})
	expectContains(t, line, "something_new", "transfer authorization docs")
}

func TestDescribeSaysNoAuthorizerWasEnrolledWithoutAnAuthorization(t *testing.T) {
	expectContains(t, transferauthorizer.Describe(nil), "no transfer authorizer")
}
