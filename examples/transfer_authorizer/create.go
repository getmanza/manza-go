package transferauthorizer

import (
	"context"
	"errors"
	"fmt"
	"io"

	manza "github.com/getmanza/manza-go"
)

// Transfer is what cmd/create_transfer asks the API to pay.
type Transfer struct {
	AccountID       string
	BeneficiaryID   string
	Amount          string // a decimal string, e.g. "10.00"
	ClientReference string // unique per entity; generate one per transfer
}

// CreateTransfer creates a payment with key A, writes it to the ledger from
// the API's response, and prints where the API routed it. The returned error
// is the line to print before exiting.
func CreateTransfer(ctx context.Context, creator *manza.Client, ledger *Ledger, t Transfer, out io.Writer) error {
	resp, err := creator.Payments.Create(ctx, manza.Attributes{
		"account_id":        t.AccountID,
		"beneficiary_id":    t.BeneficiaryID,
		"amount":            t.Amount,
		"payment_reference": "Transfer authorizer example",
		"client_reference":  t.ClientReference,
	})
	if err != nil {
		// A timeout may still have created the payment: look for this
		// client_reference in the app before rerunning, or the rerun creates a
		// second transfer.
		return fmt.Errorf("create failed for client_reference %s: %s", t.ClientReference, describeError(err))
	}
	payment := resp.Body

	destination := str(payment["destination_account_id"])
	external := str(payment["external_account_id"])
	if destination != "" {
		external = ""
	}
	payee, err := manza.PayeeFor(external, destination)
	if err != nil {
		return fmt.Errorf("payment %s names neither an external_account_id nor a destination_account_id, so it was not "+
			"written to the ledger: if it reaches the authorizer, the authorizer declines it", str(payment["id"]))
	}
	err = ledger.Record(t.ClientReference, Entry{
		PaymentID: str(payment["id"]), Amount: str(payment["amount"]), CurrencyCode: str(payment["currency_code"]),
		AccountID: str(payment["account_id"]), Payee: payee,
	})
	if errors.Is(err, ErrLockNotReleased) {
		// The entry is written; only the next Record would wait on the lock.
		fmt.Fprintf(out, "warning: payment %s is in the ledger, but %v\n", str(payment["id"]), err)
	} else if err != nil {
		return fmt.Errorf("payment %s (client_reference %s) was created but not written to the ledger (%v): "+
			"if it reaches the authorizer, the authorizer declines it", str(payment["id"]), t.ClientReference, err)
	}
	fmt.Fprintf(out, "created payment %s: %s %s to %s (client_reference %s)\n",
		str(payment["id"]), str(payment["amount"]), str(payment["currency_code"]), payee, t.ClientReference)

	authorization, _ := payment["authorization"].(map[string]any)
	fmt.Fprintln(out, Describe(authorization))
	return nil
}

// describeError spells out an API error's kind, type and request id: quote
// the request id to support.
func describeError(err error) string {
	var apiErr *manza.Error
	if errors.As(err, &apiErr) {
		return fmt.Sprintf("%s: %s (type %q, request_id %q)", describeKind(apiErr), apiErr.Message, apiErr.Type, apiErr.RequestID)
	}
	return err.Error()
}
