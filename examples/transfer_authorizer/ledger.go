// Package transferauthorizer is a runnable two-key transfer authorizer: a
// creator (key A, cmd/create_transfer), an authorizer webhook server (key B,
// cmd/authorizer), a preflight (cmd/preflight) and the ledger they share.
// It is an example, not part of the published manza-go module.
package transferauthorizer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Entry is what you recorded about a payment you created.
type Entry struct {
	PaymentID    string `json:"payment_id"`
	Amount       string `json:"amount"`
	CurrencyCode string `json:"currency_code"`
	AccountID    string `json:"account_id"`
	Payee        string `json:"payee"`
}

// Ledger is your own record of the transfers you created, keyed by the
// client_reference you generated. A JSON file, so the creator and the
// authorizer can run as two processes. Swap it for your database in
// production.
//
//	{ "po_8812": { "payment_id": "...", "amount": "2500.0",
//	               "currency_code": "MAD", "account_id": "...",
//	               "payee": "ext:..." } }
type Ledger struct{ path string }

// NewLedger returns the ledger stored at path.
func NewLedger(path string) *Ledger { return &Ledger{path: path} }

// Record stores entry under clientReference.
//
// Writers are serialized on a separate lock file; the new ledger is written
// to a temporary file and renamed over the old one, so a reader sees the old
// or the new ledger, never a half-written one, and a crash mid-write leaves
// the old ledger intact.
func (l *Ledger) Record(clientReference string, entry Entry) error {
	unlock, err := lockFile(l.path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()

	entries, err := l.read()
	if err != nil {
		return err
	}
	entries[clientReference] = entry
	encoded, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", l.path, os.Getpid())
	if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, l.path)
}

// Find returns the entry for clientReference, and whether there is one.
func (l *Ledger) Find(clientReference string) (Entry, bool, error) {
	entries, err := l.read()
	if err != nil {
		return Entry{}, false, err
	}
	entry, ok := entries[clientReference]
	return entry, ok, nil
}

func (l *Ledger) read() (map[string]Entry, error) {
	entries := map[string]Entry{}
	raw, err := os.ReadFile(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return entries, nil
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(raw)) == "" {
		return entries, nil
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("read ledger %s: %w", l.path, err)
	}
	if entries == nil { // the file held JSON null
		entries = map[string]Entry{}
	}
	return entries, nil
}
