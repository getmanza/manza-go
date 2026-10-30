// Checks every setup step before you create a transfer, and says how to fix
// the ones that fail:
//
//	go run ./cmd/preflight          # for a 10.00 transfer
//	go run ./cmd/preflight 250.00
//
// ✓ passed, ✗ failed (exit status 1), ! cannot be checked through the API.
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	transferauthorizer "github.com/getmanza/manza-go/examples/transfer_authorizer"
)

func main() {
	if err := transferauthorizer.LoadDotenv(filepath.Join(transferauthorizer.Dir(), ".env")); err != nil {
		log.Fatalf("read .env: %v", err)
	}
	amount := "10.00"
	if len(os.Args) > 1 {
		amount = os.Args[1]
	}
	if !transferauthorizer.NewPreflight(os.Getenv, amount, os.Stdout, time.Now()).Run(context.Background()) {
		os.Exit(1)
	}
}
