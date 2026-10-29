module github.com/getmanza/manza-go/examples/transfer_authorizer

go 1.24

require github.com/getmanza/manza-go v1.0.0

// The example runs against this checkout of the SDK. Its own go.mod keeps it
// out of the published manza-go module. Copied out of this repository, drop
// the replace and require the first manza-go release with Client.Payments
// and ConstructWebhookEvent (the one after 1.0.0).
replace github.com/getmanza/manza-go => ../..
