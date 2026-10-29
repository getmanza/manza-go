module github.com/getmanza/manza-go/examples/transfer_authorizer

go 1.24

require github.com/getmanza/manza-go v1.0.0

// The example runs against this checkout of the SDK. Its own go.mod keeps it
// out of the published manza-go module.
replace github.com/getmanza/manza-go => ../..
