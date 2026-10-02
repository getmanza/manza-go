# zazu-go

Go SDK for the [Zazu](https://zazu.ma) API.

```bash
go get github.com/getzazu/zazu-go
```

```go
import zazu "github.com/getzazu/zazu-go"

client, err := zazu.New(zazu.WithAPIKey(os.Getenv("ZAZU_API_KEY")))
if err != nil { ... }

entity, err := client.Entity.Get(ctx)

page, err := client.Accounts.List(ctx, zazu.AccountListParams{})
for _, account := range page.Data {
    fmt.Println(account["id"], account["name"])
}

// Initiate a transfer — it lands in your workspace's in-app approval
// queue; the API never executes a transfer itself.
draft, err := client.TransferDrafts.Create(ctx, zazu.Attributes{
    "account_id":        accountID,
    "beneficiary_id":    beneficiaryID,
    "amount":            "150.00",
    "payment_reference": "INV-000042",
})
```

## Response shape

Response bodies are returned as-is from the API — `snake_case` keys in a
`map[string]any` (`resp.Body`). The same shape ships across every Zazu SDK
(Ruby, TypeScript, Python, Go, ...) so the cassette contract is one-to-one.

For typed access, decode the raw body onto a model from `types.go`
(`TransferDraft`, `Beneficiary`, `ExternalAccount`, `PayeeTrustRequest`,
`CheckoutSession`, `PaymentLink`, `Customer`, `Invoice`, `Authorization`):

```go
var draft zazu.TransferDraft
if err := resp.Decode(&draft); err != nil { ... }
```

Market-gated keys (`tax_id`, `ice_number`, `delivery_date`) are absent
outside Morocco, so they are pointers and `nil` there.

## Hosts

| Market | Base URL |
|---|---|
| Morocco (default) | `https://ma.manza.finance` |
| South Africa | `https://za.manza.finance` |

Override with `zazu.WithBaseURL(...)` or `ZAZU_BASE_URL`. The replay
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
// draft is a zazu.TransferDraft (decoded via resp.Decode) or your own record.
// Its ExternalAccountID and ClientReference are *string: nil for an
// own-account move and for a transfer without a client_reference.
payee, err := zazu.PayeeFor(*draft.ExternalAccountID, "") // or ("", destinationAccountID)
if err != nil {
    return err
}
clientReference := ""
if draft.ClientReference != nil {
    clientReference = *draft.ClientReference
}
input := zazu.SignatureInput(zazu.TransferAuthorizationFields{
    PaymentID:       draft.ID,
    Nonce:           nonce,
    Amount:          draft.Amount, // the API's string verbatim, e.g. "2500.0"
    CurrencyCode:    draft.CurrencyCode,
    AccountID:       draft.AccountID,
    Payee:           payee,
    ClientReference: clientReference, // "" when the transfer has none
})
signature := zazu.Sign(signingSecret, input) // lowercase hex HMAC-SHA256

resp, err := authorizerClient.TransferDrafts.Authorize(ctx, draft.ID, authorizationID, signature)
// or: authorizerClient.TransferDrafts.Decline(ctx, draft.ID, authorizationID, "reason") // "" omits reason
```

`Authorize` refuses a blank signature locally (`*zazu.ArgumentError`)
because the API counts a missing signature as a failed attempt.
`Create` accepts an optional `client_reference` (at most 128 characters,
unique per entity); a duplicate returns a conflict error carrying the
existing draft's `PaymentID`.

## Errors

Non-2xx responses come back as `*zazu.Error` with `Status`, `Kind`
(`zazu.KindAuthentication`, `KindForbidden`, `KindNotFound`,
`KindValidation` for 400 and 422, `KindConflict` for 409, `KindRateLimit`,
`KindServer`, `KindAPI`), the API's `Type`/`Message`/`Param`, and the
`RequestID`. A conflict also carries `PaymentID`. Discriminate on `Kind`,
not on status codes:

```go
var apiErr *zazu.Error
if errors.As(err, &apiErr) && apiErr.Kind == zazu.KindConflict {
    fmt.Println("already created as", apiErr.PaymentID)
}
```

Transport failures are `*zazu.ConnectionError`, a misconfigured client is
`*zazu.ConfigurationError`, and a value the SDK refuses to send is
`*zazu.ArgumentError`.

## Tests

Tests replay the canonical cassettes recorded by
[zazu-ruby](https://github.com/getzazu/zazu-ruby). The cassettes are
downloaded from the Ruby SDK's release tarball and served from an
`httptest.Server`. Same interactions, same assertions, every language.

```bash
scripts/fetch-cassettes.sh
go test ./...
```

## The SDK family

- [zazu-ruby](https://github.com/getzazu/zazu-ruby) — reference implementation (records the cassettes)
- [zazu-ts](https://github.com/getzazu/zazu-ts)
- [zazu-python](https://github.com/getzazu/zazu-python)
- [cli](https://github.com/getzazu/cli)
