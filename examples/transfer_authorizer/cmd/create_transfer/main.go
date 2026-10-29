// The creator process (key A). Creates a payment, writes it to the ledger
// from the API's response, and says where the API routed it.
//
//	go run ./cmd/create_transfer          # 10.00
//	go run ./cmd/create_transfer 250.00
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	manza "github.com/getmanza/manza-go"
	transferauthorizer "github.com/getmanza/manza-go/examples/transfer_authorizer"
)

func main() {
	if err := transferauthorizer.LoadDotenv(".env"); err != nil {
		log.Fatalf("read .env: %v", err)
	}
	amount := "10.00"
	if len(os.Args) > 1 {
		amount = os.Args[1]
	}
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		log.Fatal(err)
	}

	creator, err := manza.New(manza.WithAPIKey(os.Getenv("MANZA_CREATOR_API_KEY")))
	if err != nil {
		log.Fatal(err)
	}
	err = transferauthorizer.CreateTransfer(context.Background(), creator, transferauthorizer.NewLedger("ledger.json"),
		transferauthorizer.Transfer{
			AccountID:       os.Getenv("MANZA_ACCOUNT_ID"),
			BeneficiaryID:   os.Getenv("MANZA_BENEFICIARY_ID"),
			Amount:          amount,
			ClientReference: "example-" + hex.EncodeToString(suffix),
		}, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
