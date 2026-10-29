package transferauthorizer_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	manza "github.com/getmanza/manza-go"
	transferauthorizer "github.com/getmanza/manza-go/examples/transfer_authorizer"
)

func createTransfer(t *testing.T, responses ...stubResponse) (*stubAPI, *transferauthorizer.Ledger, string, error) {
	t.Helper()
	api, server := newStubAPI(t)
	api.on("POST", "/api/payments", responses...)
	client, err := manza.New(manza.WithAPIKey("key-a"), manza.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	ledger := transferauthorizer.NewLedger(filepath.Join(t.TempDir(), "ledger.json"))
	out := &bytes.Buffer{}
	err = transferauthorizer.CreateTransfer(context.Background(), client, ledger, transferauthorizer.Transfer{
		AccountID: "acc", BeneficiaryID: "ben", Amount: "10.00", ClientReference: "example-1",
	}, out)
	return api, ledger, out.String(), err
}

func TestCreateTransferRecordsThePaymentFromTheResponse(t *testing.T) {
	api, ledger, out, err := createTransfer(t, stubResponse{status: 201, body: `{"id":"pay_1","amount":"10.0",` +
		`"currency_code":"MAD","account_id":"acc","external_account_id":"ext_1","destination_account_id":null,` +
		`"authorization":{"status":"pending"}}`})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	calls := api.calls("POST", "/api/payments")
	if len(calls) != 1 || calls[0].authorization != "Bearer key-a" || calls[0].body["client_reference"] != "example-1" ||
		calls[0].body["amount"] != "10.00" {
		t.Fatalf("unexpected create %#v", calls)
	}
	entry, found, err := ledger.Find("example-1")
	if err != nil || !found {
		t.Fatalf("expected a ledger entry, got %v %v", found, err)
	}
	want := transferauthorizer.Entry{PaymentID: "pay_1", Amount: "10.0", CurrencyCode: "MAD", AccountID: "acc", Payee: "ext:ext_1"}
	if entry != want {
		t.Fatalf("entry %#v, want %#v", entry, want)
	}
	expectContains(t, out, "created payment pay_1: 10.0 MAD to ext:ext_1 (client_reference example-1)", "challenge sent")
}

func TestCreateTransferSaysWhyARefusedPaymentWentToTheApprovers(t *testing.T) {
	_, _, out, err := createTransfer(t, stubResponse{status: 201, body: `{"id":"pay_1","amount":"11.0",` +
		`"currency_code":"MAD","account_id":"acc","external_account_id":"ext_1",` +
		`"authorization":{"status":"refused","refusal_reason":"over_transfer_limit"}}`})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	expectContains(t, out, "sent to your approvers", "over_transfer_limit")
}

func TestCreateTransferReportsTheErrorTypeAndRequestID(t *testing.T) {
	_, ledger, _, err := createTransfer(t, stubResponse{status: 422, header: map[string]string{"X-Request-Id": "req_9"},
		body: `{"error":{"type":"invalid_amount","message":"Amount is invalid"}}`})
	if err == nil {
		t.Fatal("expected an error")
	}
	expectContains(t, err.Error(), "create failed for client_reference example-1", "invalid_amount", "req_9")
	if _, found, _ := ledger.Find("example-1"); found {
		t.Fatal("expected nothing in the ledger")
	}
}

func TestCreateTransferDoesNotRecordAPaymentWithoutAPayee(t *testing.T) {
	_, ledger, _, err := createTransfer(t, stubResponse{status: 201, body: `{"id":"pay_1","amount":"10.0","currency_code":"MAD","account_id":"acc"}`})
	if err == nil || !strings.Contains(err.Error(), "not written to the ledger") {
		t.Fatalf("expected a not-written error, got %v", err)
	}
	if _, found, _ := ledger.Find("example-1"); found {
		t.Fatal("expected nothing in the ledger")
	}
}

func TestCreateTransferWarnsWhenTheLedgerLockIsStuckButKeepsTheEntry(t *testing.T) {
	t.Cleanup(transferauthorizer.FailLockRelease())

	_, ledger, out, err := createTransfer(t, stubResponse{status: 201, body: `{"id":"pay_1","amount":"10.0",` +
		`"currency_code":"MAD","account_id":"acc","external_account_id":"ext_1","authorization":{"status":"pending"}}`})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	expectContains(t, out, "warning: payment pay_1 is in the ledger, but ledger lock not released: lock file stuck",
		"created payment pay_1")
	if _, found, err := ledger.Find("example-1"); err != nil || !found {
		t.Fatalf("expected the entry kept, got %v %v", found, err)
	}
}

func TestCreateTransferDoesNotRecordAResponseMissingAField(t *testing.T) {
	_, ledger, _, err := createTransfer(t, stubResponse{status: 201, body: `{"id":"pay_1","currency_code":"MAD",` +
		`"account_id":"acc","external_account_id":"ext_1"}`})
	if err == nil || !strings.Contains(err.Error(), "amount") || !strings.Contains(err.Error(), "not written to the ledger") {
		t.Fatalf("expected a missing-field error, got %v", err)
	}
	if _, found, _ := ledger.Find("example-1"); found {
		t.Fatal("expected nothing in the ledger")
	}
}
