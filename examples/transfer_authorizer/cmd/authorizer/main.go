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
	"strings"
	"time"

	manza "github.com/getmanza/manza-go"
	transferauthorizer "github.com/getmanza/manza-go/examples/transfer_authorizer"
)

func main() {
	if err := transferauthorizer.LoadDotenv(".env"); err != nil {
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
	allowNewPayees := true
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ALLOW_NEW_PAYEES"))) {
	case "false", "0", "no":
		allowNewPayees = false
	}
	app := transferauthorizer.NewApp(client, os.Getenv("MANZA_AUTHORIZER_SECRET"),
		transferauthorizer.NewLedger("ledger.json"), allowNewPayees)

	port := os.Getenv("PORT")
	if port == "" {
		port = "4599"
	}
	app.Logger.Printf("INFO listening on :%s (ALLOW_NEW_PAYEES=%t)", port, allowNewPayees)
	server := &http.Server{Addr: ":" + port, Handler: app, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(server.ListenAndServe())
}
