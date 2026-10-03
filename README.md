# manza-go

Go SDK for the Manza API.

```bash
go get github.com/getmanza/manza-go
```

```go
import manza "github.com/getmanza/manza-go"

client, err := manza.New(manza.WithAPIKey(os.Getenv("MANZA_API_KEY")))
if err != nil { ... }

entity, err := client.Entity.Get(ctx)

page, err := client.Accounts.List(ctx, manza.AccountListParams{})
for _, account := range page.Data {
    fmt.Println(account["id"], account["name"])
}

// Initiate a transfer — it lands in your workspace's in-app approval
// queue; the API never executes a transfer itself.
draft, err := client.TransferDrafts.Create(ctx, manza.Attributes{
    "account_id":        accountID,
    "beneficiary_id":    beneficiaryID,
    "amount":            "150.00",
    "payment_reference": "INV-000042",
})
```

## Response shape

Response bodies are returned as-is from the API — `snake_case` keys in a
`map[string]any` (`resp.Body`). The same shape ships across every Manza SDK
(Ruby, TypeScript, Python, Go, ...) so the cassette contract is one-to-one.

For typed access, decode the raw body onto a model from `types.go`
(`TransferDraft`, `Beneficiary`, `ExternalAccount`, `PayeeTrustRequest`,
`CheckoutSession`, `PaymentLink`, `Customer`, `Invoice`, `Authorization`):

```go
var draft manza.TransferDraft
if err := resp.Decode(&draft); err != nil { ... }
```

Market-gated keys (`tax_id`, `ice_number`, `delivery_date`) are absent
outside Morocco, so they are pointers and `nil` there.

## Hosts

| Market | Base URL |
|---|---|
| Morocco (default) | `https://ma.manza.finance` |
| South Africa | `https://za.manza.finance` |

Override with `manza.WithBaseURL(...)` or `MANZA_BASE_URL` (the legacy `ZAZU_BASE_URL` still works, with a one-time deprecation warning). The replay
cassettes were recorded against `https://ma.manza.dev` (the staging host).

## Resources

| Service | Methods |
|---|---|
| `Accounts` | `List`, `Get`, `ListTransactions`, `GetTransaction` |
| `Beneficiaries` | `List`, `Get`, `Create`, `ListExternalAccounts`, `GetExternalAccount`, `CreateExternalAccount` |
| `CheckoutSessions` | `Create`, `Get` |
| `Customers` | `List`, `Get`, `Create`, `Update`, `Delete` |
| `Entity` | `Get` |
| `Invoices` | `List`, `Get`, `Create`, `Update`, `Send`, `MarkAsPaid`, `Cancel`, `CreditNote`, `Delete`, `CreatePaymentLink` |
| `PayeeTrustRequests` | `Create(ctx, externalAccountIDs)`, `Get` |
| `PaymentLinks` | `List`, `Get`, `Create`, `Cancel` |
| `TransferDrafts` | `Create`, `Get`, `Authorize`, `Decline` |
| `WebhookEndpoints` | `List`, `Get`, `Create`, `Update`, `Delete`, `Test`, `RegenerateSecret`, `Enable`, `Disable` |

## Machine-authorizing transfers

A draft inside your machine-authorization envelope (trusted payee, within
limits) is sent to your enrolled transfer authorizer as a
`payment.authorization_requested` webhook carrying `authorization_id` and a
one-time `nonce`. Build the signature input from your **own** record of the
transfer (not the webhook's `signature_input`), sign it with the authorizer
endpoint's signing secret, and answer with a key other than the one that
created the draft:

```go
// draft is a manza.TransferDraft (decoded via resp.Decode) or your own record.
// Its ExternalAccountID, DestinationAccountID and ClientReference are *string:
// exactly one of the first two is set, and ClientReference is nil for a
// transfer without a client_reference.
externalAccountID, destinationAccountID := "", ""
if draft.ExternalAccountID != nil {
    externalAccountID = *draft.ExternalAccountID
}
if draft.DestinationAccountID != nil {
    destinationAccountID = *draft.DestinationAccountID
}
payee, err := manza.PayeeFor(externalAccountID, destinationAccountID)
if err != nil {
    return err
}
clientReference := ""
if draft.ClientReference != nil {
    clientReference = *draft.ClientReference
}
input := manza.SignatureInput(manza.TransferAuthorizationFields{
    PaymentID:       draft.ID,
    Nonce:           nonce,
    Amount:          draft.Amount, // the API's string verbatim, e.g. "2500.0"
    CurrencyCode:    draft.CurrencyCode,
    AccountID:       draft.AccountID,
    Payee:           payee,
    ClientReference: clientReference, // "" when the transfer has none
})
signature := manza.Sign(signingSecret, input) // lowercase hex HMAC-SHA256

resp, err := authorizerClient.TransferDrafts.Authorize(ctx, draft.ID, authorizationID, signature)
// or: authorizerClient.TransferDrafts.Decline(ctx, draft.ID, authorizationID, "reason") // "" omits reason
```

`Authorize` refuses a blank signature locally (`*manza.ArgumentError`)
because the API counts a missing signature as a failed attempt.
`Create` accepts an optional `client_reference` (at most 128 characters,
unique per entity); a duplicate returns a conflict error carrying the
existing draft's `PaymentID`.

## Errors

Non-2xx responses come back as `*manza.Error` with `Status`, `Kind`
(`manza.KindAuthentication`, `KindForbidden`, `KindNotFound`,
`KindValidation` for 400 and 422, `KindConflict` for 409, `KindRateLimit`,
`KindServer`, `KindAPI`), the API's `Type`/`Message`/`Param`, and the
`RequestID`. A conflict also carries `PaymentID`. Discriminate on `Kind`,
not on status codes:

```go
var apiErr *manza.Error
if errors.As(err, &apiErr) && apiErr.Kind == manza.KindConflict {
    fmt.Println("already created as", apiErr.PaymentID)
}
```

Transport failures are `*manza.ConnectionError`, a misconfigured client is
`*manza.ConfigurationError`, and a value the SDK refuses to send is
`*manza.ArgumentError`.

## Tests

Tests replay the canonical cassettes recorded by
[manza-ruby](https://github.com/getmanza/manza-ruby). The cassettes are
downloaded from the Ruby SDK's release tarball and served from an
`httptest.Server`. Same interactions, same assertions, every language.

```bash
scripts/fetch-cassettes.sh
go test ./...
```

## The SDK family

- [manza-ruby](https://github.com/getmanza/manza-ruby) — reference implementation (records the cassettes)
- [manza-ts](https://github.com/getmanza/manza-ts)
- [manza-python](https://github.com/getmanza/manza-python)
- [cli](https://github.com/getmanza/cli)
