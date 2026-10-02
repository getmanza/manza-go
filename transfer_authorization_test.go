package manza_test

import (
	"errors"
	"testing"

	manza "github.com/getmanza/manza-go"
)

// Fixed test vector, shared by every SDK in the family (see
// spec/manza/transfer_authorization_spec.rb in manza-ruby). Each signer must
// produce exactly these hex digests from these inputs; they were computed
// independently with:
//
//	printf '%s' '<input>' | openssl dgst -sha256 -hmac 'whsec_test_vector_secret'
const vectorSecret = "whsec_test_vector_secret"

func vectorFields() manza.TransferAuthorizationFields {
	return manza.TransferAuthorizationFields{
		PaymentID:    "0199a1b2-0000-7000-8000-000000000001",
		Nonce:        "n0nce-0123456789abcdef",
		Amount:       "2500.0",
		CurrencyCode: "MAD",
		AccountID:    "0199a1b2-0000-7000-8000-000000000002",
	}
}

func TestTransferAuthorizationExternalPayeeWithClientReference(t *testing.T) {
	payee, err := manza.PayeeFor("0199a1b2-0000-7000-8000-000000000003", "")
	if err != nil {
		t.Fatalf("payee: %v", err)
	}
	fields := vectorFields()
	fields.Payee = payee
	fields.ClientReference = "po_1"

	input := manza.SignatureInput(fields)
	want := "manza.transfer-authorization.v1|0199a1b2-0000-7000-8000-000000000001|n0nce-0123456789abcdef|" +
		"2500.0|MAD|0199a1b2-0000-7000-8000-000000000002|ext:0199a1b2-0000-7000-8000-000000000003|po_1"
	if input != want {
		t.Fatalf("signature input\n got %s\nwant %s", input, want)
	}
	if got := manza.Sign(vectorSecret, input); got != "6e8eaec0f89a4eb3b22df1133b3d6dfebfa8505c34c58ed0ff192516e4223078" {
		t.Fatalf("signature %s", got)
	}
}

func TestTransferAuthorizationOwnPayeeWithoutClientReference(t *testing.T) {
	payee, err := manza.PayeeFor("", "0199a1b2-0000-7000-8000-000000000004")
	if err != nil {
		t.Fatalf("payee: %v", err)
	}
	fields := vectorFields()
	fields.Payee = payee

	input := manza.SignatureInput(fields)
	if suffix := "|own:0199a1b2-0000-7000-8000-000000000004|"; input[len(input)-len(suffix):] != suffix {
		t.Fatalf("expected input to end with %q, got %s", suffix, input)
	}
	if got := manza.Sign(vectorSecret, input); got != "af9440b1de1bebb51f381ce43e3d0d27b6a4ccb99dcd548c0b5435ff4fdd1895" {
		t.Fatalf("signature %s", got)
	}
}

func TestPayeeForRequiresExactlyOne(t *testing.T) {
	for name, ids := range map[string][2]string{"both": {"a", "b"}, "neither": {"", ""}} {
		_, err := manza.PayeeFor(ids[0], ids[1])
		var argErr *manza.ArgumentError
		if !errors.As(err, &argErr) {
			t.Fatalf("%s: expected *manza.ArgumentError, got %T (%v)", name, err, err)
		}
	}
}
