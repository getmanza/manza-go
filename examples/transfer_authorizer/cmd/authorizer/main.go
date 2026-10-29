// The authorizer process (key B): answers payment.authorization_requested
// webhooks on PORT (default 4599). Point your tunnel at the same port.
//
//	go run ./cmd/authorizer
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	manza "github.com/getmanza/manza-go"
	transferauthorizer "github.com/getmanza/manza-go/examples/transfer_authorizer"
)

func main() {
	if err := transferauthorizer.LoadDotenv(filepath.Join(transferauthorizer.Dir(), ".env")); err != nil {
		log.Fatalf("read .env: %v", err)
	}
	var missing []string
	for _, name := range []string{"MANZA_AUTHORIZER_API_KEY", "MANZA_AUTHORIZER_SECRET"} {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "Set %s in .env\n", strings.Join(missing, " and "))
		os.Exit(1)
	}

	client, err := manza.New(manza.WithAPIKey(os.Getenv("MANZA_AUTHORIZER_API_KEY")))
	if err != nil {
		log.Fatal(err)
	}
	var allowNewPayees bool
	switch value := strings.ToLower(strings.TrimSpace(os.Getenv("ALLOW_NEW_PAYEES"))); value {
	case "", "true", "1", "yes":
		allowNewPayees = true
	case "false", "0", "no":
		allowNewPayees = false
	default:
		// A typo must not leave new payees allowed by accident.
		fmt.Fprintf(os.Stderr, "ALLOW_NEW_PAYEES=%q: use true or false\n", value)
		os.Exit(1)
	}
	app := transferauthorizer.NewApp(client, os.Getenv("MANZA_AUTHORIZER_SECRET"),
		transferauthorizer.NewLedger(filepath.Join(transferauthorizer.Dir(), "ledger.json")), allowNewPayees)

	port := os.Getenv("PORT")
	if port == "" {
		port = "4599"
	}
	app.Logger.Printf("INFO listening on :%s (ALLOW_NEW_PAYEES=%t)", port, allowNewPayees)
	server := &http.Server{Addr: ":" + port, Handler: app, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(server.ListenAndServe())
}
