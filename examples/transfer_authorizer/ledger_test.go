package transferauthorizer_test

import (
	"os"
	"path/filepath"
	"testing"

	transferauthorizer "github.com/getmanza/manza-go/examples/transfer_authorizer"
)

func TestLedgerRecordsAndFindsEntriesAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := transferauthorizer.NewLedger(path).Record("po_1", ledgerEntry()); err != nil {
		t.Fatalf("record: %v", err)
	}
	other := ledgerEntry()
	other.PaymentID = "pay_2"
	if err := transferauthorizer.NewLedger(path).Record("po_2", other); err != nil {
		t.Fatalf("record: %v", err)
	}

	reader := transferauthorizer.NewLedger(path)
	for reference, want := range map[string]transferauthorizer.Entry{"po_1": ledgerEntry(), "po_2": other} {
		entry, found, err := reader.Find(reference)
		if err != nil || !found || entry != want {
			t.Fatalf("%s: got %#v %v %v", reference, entry, found, err)
		}
	}
	if _, found, err := reader.Find("po_3"); err != nil || found {
		t.Fatalf("expected po_3 missing, got %v %v", found, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("expected a 0600 ledger, got %v %v", info, err)
	}
}

func TestLedgerTreatsAMissingOrEmptyFileAsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	if _, found, err := transferauthorizer.NewLedger(path).Find("po_1"); err != nil || found {
		t.Fatalf("missing file: %v %v", found, err)
	}
	if err := os.WriteFile(path, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, found, err := transferauthorizer.NewLedger(path).Find("po_1"); err != nil || found {
		t.Fatalf("empty file: %v %v", found, err)
	}
}
