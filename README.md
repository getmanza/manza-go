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

// Initiate a transfer (a payment). Creating one never executes it: it goes
// to your enrolled transfer authorizer or to your in-app approvers.
payment, err := client.Payments.Create(ctx, manza.Attributes{
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
(`TransferDraft`, `Authorization`, `TransferAuthorizer`, `Beneficiary`,
`ExternalAccount`, `PayeeTrustRequest`, `CheckoutSession`, `PaymentLink`,
`Customer`, `Invoice`):

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
| `Payments` | `Create`, `Get` (`authorization`: status, `refusal_reason`), `Authorize`, `Decline` |
| `TransferAuthorizer` | `Get` (status, `suspension_reason`, limits, `daily_authorized_total`; a not-found `*manza.Error` when none is enrolled) |
| `TransferDrafts` | Deprecated alias of `Payments` (the same service; it calls `/api/payments`) |
| `WebhookEndpoints` | `List`, `Get`, `Create`, `Update`, `Delete`, `Test`, `RegenerateSecret`, `Enable`, `Disable` |

## Machine-authorized transfers

A payment inside your entity's authorization envelope (trusted payee, or a
new one when you allow it, within limits) is sent to your enrolled
authorizer endpoint as a `payment.authorization_requested` webhook carrying
an `authorization.id` and a one-time `nonce`. Your system checks it against
its own ledger, signs it and authorizes it with a **different API key** from
the one that created it.
[`examples/transfer_authorizer/`](examples/transfer_authorizer/) runs the
whole flow; the [transfer authorization docs](https://ma.manza.finance/docs/api/transfer-authorization)
explain every step.

### Setup, step by step

1. **Feature flags.** Ask support to enable API transfers and transfer authorization (`release_api_webhook_authorization`) on your workspace.
2. **Two API keys.** Key A creates payments (`transfers:write`; `beneficiaries:request_trust` for step 6; `entity:read`, `accounts:read`, `webhook_endpoints:read` and `beneficiaries:read` for the preflight in step 7). Key B answers the challenges (`transfers:authorize`). The creating key is always refused with 403 `same_key_forbidden`, so keep key B and the signing secret on the system that holds your ledger. Bind key B to an IP allowlist: authorizing from a key without one is refused with 403 `unrestricted_api_key`. Bind key A to one too if you allow new payees (step 6): a payment to a new payee from a key without an allowlist goes to your approvers (`unrestricted_api_key`).
3. **A stable public URL.** Your authorizer needs an HTTPS URL that never changes: an enrolled endpoint's URL is locked. Use a reserved ngrok domain (`ngrok http --url=your-name.ngrok.app 4599`) or a named Cloudflare tunnel. Never a free ngrok URL: it changes on every restart.
4. **Create the webhook endpoint** at that URL:

   ```go
   _, err := client.WebhookEndpoints.Create(ctx, manza.Attributes{
       "url":    "https://your-name.ngrok.app/",
       "events": []string{"transfer.executed"},
   })
   ```

5. **Enrol it in the app**: Settings → Developer → Transfer approval. Pick the endpoint, the currency and both limits, confirm with your two-factor code, and copy the signing secret: it is shown once.
6. **Allow new payees, or trust each payee.** Turn on *Approve transfers to new payees* at enrolment (or later, same page, with your two-factor code) and set the *Limit per transfer to a new payee*. Your authorizer may then approve a transfer to bank details your workspace never trusted, up to that limit, and its signature trusts them: the webhook's `payment.payee_trusted` is `false` for such a transfer. Bank details whose trust was revoked, or that were edited after trusting, still go to your approvers. Without the option, an approver marks the beneficiary's bank account as trusted from the Beneficiaries list, or you ask them to:

   ```go
   _, err := client.PayeeTrustRequests.Create(ctx, []string{"01a1..."})
   ```

   `Trusted` on the `ExternalAccount` from `client.Beneficiaries.GetExternalAccount` says which it is. The API does not show whether the option is on.
7. **Run the preflight, then the example** (from `examples/transfer_authorizer/`, after filling in `.env`): `go run ./cmd/preflight` checks what the API shows of steps 1–6 and prints a fix for each that fails. What it cannot see, check by hand: that key B is bound to an IP allowlist, and, for a payee not trusted yet, that *Approve transfers to new payees* is on and key A is bound to an IP allowlist too (it warns, it does not fail). Then `go run ./cmd/authorizer` in one terminal, `go run ./cmd/create_transfer` in another.
8. **Troubleshooting**: a payment that goes to your approvers instead has an `authorization` with status `refused` and a `refusal_reason` (`cmd/create_transfer` prints it in words); `client.TransferAuthorizer.Get` shows a suspension and its `suspension_reason`. An authorize answered 409 `authorization_busy` (a bank-detail edit, trust or trust revocation on the same payee was in flight) is safe to retry. The reasons, and an API error's `Type`, are in the [transfer authorization docs](https://ma.manza.finance/docs/api/transfer-authorization).

### Verifying the webhook

Pass the **raw request body**, never re-serialized JSON.
`manza.ConstructWebhookEvent` reads `X-Manza-Signature` and
`X-Manza-Timestamp` (names in any case, the CGI `HTTP_X_MANZA_*` form, or the
legacy `X-Zazu-*` names), checks the signature in constant time and the
timestamp within 5 minutes, and returns the parsed event (a
`map[string]any`). It returns a `*manza.ArgumentError` otherwise:

```go
func handle(w http.ResponseWriter, r *http.Request) {
    // Cap the body: it is read before the signature can be checked.
    body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
    var tooLarge *http.MaxBytesError
    if errors.As(err, &tooLarge) {
        w.WriteHeader(http.StatusRequestEntityTooLarge)
        return
    } else if err != nil {
        w.WriteHeader(http.StatusBadRequest)
        return
    }
    event, err := manza.ConstructWebhookEvent(body, r.Header, signingSecret)
    if err != nil { // *manza.ArgumentError: "invalid webhook signature", ...
        http.Error(w, err.Error(), http.StatusUnauthorized)
        return
    }
    if event["event"] == "payment.authorization_requested" {
        // answer it (see below), after responding 200
    }
    w.WriteHeader(http.StatusOK)
}
```

`manza.VerifyWebhook(payload, signature, timestamp, secret)` returns a
`bool` instead, and `manza.SignWebhook` builds a signature for tests. Both
verifiers take options: `manza.WithWebhookTolerance(d)` changes the 5
minutes, and `manza.WithoutWebhookTolerance()` skips the timestamp check.
Use the latter only to replay a captured delivery in a local tool or test,
never on a live endpoint: without the check, anyone holding a captured
delivery can replay it.

### Signing the challenge

Sign the payment from **your own record** of it (not the webhook's
`signature_input`, which is there only to compare against) with the
endpoint's signing secret, and answer with key B:

```go
// payment is a manza.TransferDraft (decoded via resp.Decode) or your own record.
// Its ExternalAccountID, DestinationAccountID and ClientReference are *string:
// exactly one of the first two is set, and ClientReference is nil for a
// transfer without a client_reference.
externalAccountID, destinationAccountID := "", ""
if payment.ExternalAccountID != nil {
    externalAccountID = *payment.ExternalAccountID
}
if payment.DestinationAccountID != nil {
    destinationAccountID = *payment.DestinationAccountID
}
payee, err := manza.PayeeFor(externalAccountID, destinationAccountID)
if err != nil {
    return err
}
clientReference := ""
if payment.ClientReference != nil {
    clientReference = *payment.ClientReference
}
input := manza.SignatureInput(manza.TransferAuthorizationFields{
    PaymentID:       payment.ID,
    Nonce:           nonce, // data.authorization.nonce from the webhook
    Amount:          payment.Amount, // the API's string verbatim, e.g. "2500.0"
    CurrencyCode:    payment.CurrencyCode,
    AccountID:       payment.AccountID,
    Payee:           payee,
    ClientReference: clientReference, // "" when the transfer has none
})
signature := manza.Sign(signingSecret, input) // lowercase hex HMAC-SHA256

resp, err := authorizerClient.Payments.Authorize(ctx, payment.ID, authorizationID, signature)
// or: authorizerClient.Payments.Decline(ctx, payment.ID, authorizationID, "reason") // "" omits reason
```

`Authorize` refuses a blank signature locally (`*manza.ArgumentError`)
because the API counts a missing signature as a failed attempt. A wrong
signature returns a `KindValidation` error of `Type` `invalid_signature`;
five on one challenge send the payment to your in-app approvers, five in a
row suspend the authorizer. `Create` accepts an optional `client_reference`
(at most 128 characters, unique per entity); a duplicate returns a conflict
error carrying the existing payment's `PaymentID`.

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
(cd examples/transfer_authorizer && go test ./...)   # the example is its own module
```

## The SDK family

| SDK | Repository | Install |
|---|---|---|
| Ruby (reference implementation, records the cassettes) | [getmanza/manza-ruby](https://github.com/getmanza/manza-ruby) | `gem "manza"` |
| TypeScript / JavaScript | [getmanza/manza-ts](https://github.com/getmanza/manza-ts) | `npm install @getmanza/sdk` |
| Python | [getmanza/manza-python](https://github.com/getmanza/manza-python) | `pip install manza` |
| Go | [getmanza/manza-go](https://github.com/getmanza/manza-go) (this repo) | `go get github.com/getmanza/manza-go` |
| PHP | [getmanza/manza-php](https://github.com/getmanza/manza-php) | `composer require manza/manza-php` |
| Rust | [getmanza/manza-rust](https://github.com/getmanza/manza-rust) | `cargo add manza` |
| Crystal | [getmanza/manza-crystal](https://github.com/getmanza/manza-crystal) | shard `manza` (`github: getmanza/manza-crystal`) |
| Elixir | [getmanza/manza-elixir](https://github.com/getmanza/manza-elixir) | `{:manza, "~> 1.0"}` |
| CLI | [getmanza/cli](https://github.com/getmanza/cli) | `npm install -g @getzazu/cli` or `brew install getmanza/tap/zazu` |
