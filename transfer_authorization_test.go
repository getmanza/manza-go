package manza_test

import (
	"errors"
	"strings"
	"testing"

	manza "github.com/getmanza/manza-go/v2"
)

// Fixed test vectors, shared by every SDK in the family (see
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

func TestTransferAuthorizationSignsVersionV2(t *testing.T) {
	input := manza.SignatureInput(vectorFields())
	if !strings.HasPrefix(input, "manza.transfer-authorization.v2|") {
		t.Fatalf("expected a v2 input, got %s", input)
	}
}

func TestTransferAuthorizationExternalPayeeWithClientReferenceAndTransferMethod(t *testing.T) {
	payee, err := manza.PayeeFor("0199a1b2-0000-7000-8000-000000000003", "")
	if err != nil {
		t.Fatalf("payee: %v", err)
	}
	fields := vectorFields()
	fields.Payee = payee
	fields.ClientReference = "po_1"
	fields.TransferMethod = "eft"

	input := manza.SignatureInput(fields)
	want := "manza.transfer-authorization.v2|0199a1b2-0000-7000-8000-000000000001|n0nce-0123456789abcdef|" +
		"2500.0|MAD|0199a1b2-0000-7000-8000-000000000002|ext:0199a1b2-0000-7000-8000-000000000003|po_1|eft"
	if input != want {
		t.Fatalf("signature input\n got %s\nwant %s", input, want)
	}
	if got := manza.Sign(vectorSecret, input); got != "904da7d0b73b2c919ffa1ae64d262ce5825ee4d2c8420fd70a3d257353e6c43a" {
		t.Fatalf("signature %s", got)
	}
}

func TestTransferAuthorizationEmptyTransferMethod(t *testing.T) {
	payee, err := manza.PayeeFor("0199a1b2-0000-7000-8000-000000000003", "")
	if err != nil {
		t.Fatalf("payee: %v", err)
	}
	fields := vectorFields()
	fields.Payee = payee
	fields.ClientReference = "po_1"

	input := manza.SignatureInput(fields)
	if suffix := "|ext:0199a1b2-0000-7000-8000-000000000003|po_1|"; !strings.HasSuffix(input, suffix) {
		t.Fatalf("expected input to end with %q, got %s", suffix, input)
	}
	if got := manza.Sign(vectorSecret, input); got != "029dd12924a846d7af2f4aecb61e4f4f9bd64d37315230864fb03917adc29edf" {
		t.Fatalf("signature %s", got)
	}
}

func TestTransferAuthorizationOwnPayeeWithoutClientReferenceOrTransferMethod(t *testing.T) {
	payee, err := manza.PayeeFor("", "0199a1b2-0000-7000-8000-000000000004")
	if err != nil {
		t.Fatalf("payee: %v", err)
	}
	fields := vectorFields()
	fields.Payee = payee

	input := manza.SignatureInput(fields)
	if suffix := "|own:0199a1b2-0000-7000-8000-000000000004||"; !strings.HasSuffix(input, suffix) {
		t.Fatalf("expected input to end with %q, got %s", suffix, input)
	}
	if got := manza.Sign(vectorSecret, input); got != "a21ccf43b94f23d7a595a36bd788c27c7adbc5686a5ad4427b22d9957568c9fb" {
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
