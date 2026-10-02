package manza_test

import (
	"context"
	"encoding/json"
	"testing"

	manza "github.com/getmanza/manza-go"
)

func TestCustomerDecode(t *testing.T) {
	server := startReplayServer(t, "customers/get")
	client := replayClient(t, server)

	resp, err := client.Customers.Get(context.Background(), fixtureID(t, "MANZA_FIXTURE_CUSTOMER_ID"))
	if err != nil {
		t.Fatalf("customers get: %v", err)
	}
	var customer manza.Customer
	if err := resp.Decode(&customer); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if customer.ID != fixtureID(t, "MANZA_FIXTURE_CUSTOMER_ID") || customer.CustomerType != "business" {
		t.Fatalf("unexpected customer %#v", customer)
	}
	if customer.ICENumber == nil || customer.TaxID != nil {
		t.Fatalf("expected ice_number set and tax_id null, got %#v", customer)
	}
	if customer.RegistrationNumber != nil || customer.VATNumber != nil {
		t.Fatalf("expected registration/vat absent in this cassette, got %#v", customer)
	}
}

func TestInvoiceDecode(t *testing.T) {
	server := startReplayServer(t, "invoices/get")
	client := replayClient(t, server)

	resp, err := client.Invoices.Get(context.Background(), fixtureID(t, "MANZA_FIXTURE_INVOICE_ID"))
	if err != nil {
		t.Fatalf("invoices get: %v", err)
	}
	var invoice manza.Invoice
	if err := resp.Decode(&invoice); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if invoice.ID != fixtureID(t, "MANZA_FIXTURE_INVOICE_ID") || invoice.DeliveryDate != nil {
		t.Fatalf("unexpected invoice %#v", invoice)
	}
}

func TestPaymentLinkDecode(t *testing.T) {
	server := startReplayServer(t, "payment_links/get")
	client := replayClient(t, server)

	resp, err := client.PaymentLinks.Get(context.Background(), fixtureID(t, "MANZA_FIXTURE_PAYMENT_LINK_ID"))
	if err != nil {
		t.Fatalf("payment links get: %v", err)
	}
	var link manza.PaymentLink
	if err := resp.Decode(&link); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if link.Status != "active" || link.SettledAt != nil || link.CollectBillingAddress != nil {
		t.Fatalf("unexpected payment link %#v", link)
	}
}

// Market-gated keys are absent (not null) outside MA, and the clearing status
// and settlement fields decode.
func TestOptionalAndClearingFields(t *testing.T) {
	var customer manza.Customer
	if err := json.Unmarshal([]byte(`{"id":"c","vat_number":"V1","registration_number":"R1"}`), &customer); err != nil {
		t.Fatal(err)
	}
	if customer.TaxID != nil || customer.ICENumber != nil {
		t.Fatalf("expected absent tax_id/ice_number to be nil, got %#v", customer)
	}
	if customer.VATNumber == nil || *customer.VATNumber != "V1" || *customer.RegistrationNumber != "R1" {
		t.Fatalf("unexpected customer %#v", customer)
	}

	var session manza.CheckoutSession
	raw := `{"id":"cs","status":"clearing","settled_at":"2026-10-02T15:00:00+00:00","customer_name":"Acme",` +
		`"collect_billing_address":true,"billing_address":{"city":"Casablanca"},"transaction":{"id":"t1"}}`
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		t.Fatal(err)
	}
	if session.Status != manza.StatusClearing || session.SettledAt == nil || *session.CustomerName != "Acme" ||
		!*session.CollectBillingAddress || session.BillingAddress["city"] != "Casablanca" || session.Transaction["id"] != "t1" {
		t.Fatalf("unexpected session %#v", session)
	}

	var invoice manza.Invoice
	if err := json.Unmarshal([]byte(`{"id":"i"}`), &invoice); err != nil || invoice.DeliveryDate != nil {
		t.Fatalf("expected absent delivery_date to be nil: %v %#v", err, invoice)
	}
}
