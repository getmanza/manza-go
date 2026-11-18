# manza-go

Go SDK for the Manza API.

```bash
go get github.com/getmanza/manza-go/v2
```

```go
import manza "github.com/getmanza/manza-go/v2"

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
    "transfer_method":   manza.TransferMethodStandard, // optional rail: eft/rtc (ZA), standard/instant (MA); omitted → the default
})

// An invoice is paid into one of your accounts: account_id is required.
invoice, err := client.Invoices.Create(ctx, manza.Attributes{
    "customer_id":           customerID,
    "account_id":            accountID,
    "currency_code":         "MAD",
    "issue_date":            issueDate, // YYYY-MM-DD, not in a month before your newest numbered invoice
    "due_date":              dueDate,
    "generate_payment_link": true, // a card payment link, made when approved (payment_links:write)
    "items":                 []map[string]any{{"description": "Consulting", "quantity": 10, "unit_price": "150.00"}},
})
```

## Response shape

Response bodies are returned as-is from the API — `snake_case` keys in a
`map[string]any` (`resp.Body`). The same shape ships across every Manza SDK
(Ruby, TypeScript, Python, Go, ...) so the cassette contract is one-to-one.

For typed access, decode the raw body onto a model from `types.go`
(`TransferDraft`, `Authorization`, `TransferAuthorizer`, `Beneficiary`,
`ExternalAccount`, `PayeeTrustRequest`, `CheckoutSession`, `PaymentLink`,
`Customer`, `Invoice` with its embedded `InvoiceAccount`):

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

## Versioning the API contract

```go
client, err := manza.New(manza.WithAPIKey("..."), manza.WithAPIVersion("2026-11-13"))
```

Or via env: `MANZA_API_VERSION=2026-11-13`. Unpinned, the API answers with
its current version. manza-go 2.x signs transfer authorizations v2, which the
API verifies for unpinned requests and requests pinned to `2026-11-13` or
later; a client pinned to `2026-03-27` must stay on manza-go 1.x. The value
is sent as the `Manza-Version` header on every request.

## Resources

| Service | Methods |
|---|---|
| `Accounts` | `List`, `Get`, `ListTransactions`, `GetTransaction` |
| `Beneficiaries` | `List`, `Get`, `Create`, `ListExternalAccounts`, `GetExternalAccount`, `CreateExternalAccount` |
| `CheckoutSessions` | `Create`, `Get` |
| `Customers` | `List`, `Get`, `Create`, `Update`, `Delete` |
| `Entity` | `Get` |
| `Invoices` | `List`, `Get`, `Create`, `Update`, `Send`, `MarkAsPaid`, `Cancel`, `CreditNote`, `Delete` (`Create`/`Update` take `account_id` and `generate_payment_link`) |
| `PayeeTrustRequests` | `Create(ctx, externalAccountIDs)`, `Get` |
| `PaymentLinks` | `List`, `Get`, `Create`, `Cancel` |
| `Payments` | `Create` (optional `transfer_method`), `Get` (`authorization`: status, `refusal_reason`), `Authorize`, `Decline` |
| `TransferAuthorizer` | `Get` (status, `suspension_reason`, limits, `daily_authorized_total`, `trust_new_payees`, `new_payee_limit`; a not-found `*manza.Error` when none is enrolled) |
| `TransferDrafts` | Deprecated alias of `Payments` (the same service; it calls `/api/payments`) |
| `WebhookDeliveries` | `List(ctx, endpointID, params)` (`Status` filter: `failed`, `pending`, `succeeded`), `Get` (adds `payload`, `response_body`), `Retry` |
| `WebhookEndpoints` | `List`, `Get`, `Create`, `Update`, `Delete`, `Test`, `RegenerateSecret`, `Enable`, `Disable`, `RetryFailed` |
| `WebhookListeners` | `Create`, `Delete`, `Stream` (see [Listening for webhooks locally](#listening-for-webhooks-locally)) |

## Listening for webhooks locally

A listen session streams a copy of every subscribed webhook to your machine,
with no public endpoint or tunnel: the engine behind `manza listen`. The
server sends each body unsigned; sign it at forward time with the session's
`SigningSecret` (the API key's listen secret), exactly as a platform delivery
is signed, so your handler's `ConstructWebhookEvent` accepts it.

```go
resp, err := client.WebhookListeners.Create(ctx, manza.WebhookListenerParams{
    Events:     []string{"transfer.executed"}, // leave out for every subscribable event
    Authorizer: true,                          // mirror payment.authorization_requested; needs transfers:authorize on this key
})
if err != nil {
    return err
}
var listener manza.WebhookListener
if err := resp.Decode(&listener); err != nil {
    return err
}
defer client.WebhookListeners.Delete(context.Background(), listener.ID)

for event, err := range client.WebhookListeners.Stream(ctx, listener, manza.WebhookListenerStreamOptions{}) {
    if err != nil {
        return err // the stream has ended; a not-found *manza.Error means the session expired
    }
    header := manza.WebhookHeaders(listener.SigningSecret, event.EventType, event.EventID, event.ID, event.Body, time.Now())
    // POST event.Body (the exact bytes, never re-serialized) with header to your local server
}
```

`Stream` reopens a dropped connection with `Last-Event-ID`, backing off from
500ms to 10s (at least a 429's `Retry-After` in seconds), so no event the
server still keeps (about five minutes) is lost or repeated; a longer outage
ends with an expired session. It ends on the server's `listener_closed`, when
`ctx` is done, or with one error: a redirect (never followed), a 4xx other
than 429, a first connect that fails, a 2xx that is not `text/event-stream`,
a webhook frame missing its fields, or a `stream_url` that is not
on the client's API host (the API key is never sent elsewhere). It ignores the HTTP client's `Timeout`; `ctx` bounds the stream.

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

   `Trusted` on the `ExternalAccount` from `client.Beneficiaries.GetExternalAccount` says which it is; `TrustNewPayees` (and `NewPayeeLimit`) on the `TransferAuthorizer` from `client.TransferAuthorizer.Get` says whether the option is on.
7. **Run the preflight, then the example** (from `examples/transfer_authorizer/`, after filling in `.env`): `go run ./cmd/preflight` checks what the API shows of steps 1–6 and prints a fix for each that fails. What it cannot see, check by hand: that key B is bound to an IP allowlist, and, for a payee not trusted yet, that *Approve transfers to new payees* is on (`trust_new_payees` from `client.TransferAuthorizer.Get`) and key A is bound to an IP allowlist too (it warns, it does not fail). Then `go run ./cmd/authorizer` in one terminal, `go run ./cmd/create_transfer` in another.
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
    w.WriteHeader(http.StatusOK) // respond first: the delivery has a timeout
    if event["event"] == "payment.authorization_requested" {
        go answer(event) // check your ledger, sign and authorize (see below)
    }
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
`signature_input_v2`, which is there only to compare against) with the
endpoint's signing secret, and answer with key B. The SDK builds the v2
input (`manza.transfer-authorization.v2`), which ends with the payment's
`transfer_method` (an empty segment when it is `null`). The webhook's
`signature_input` is the v1 string, verified only for clients pinned to
`Manza-Version: 2026-03-27`: those stay on manza-go 1.x.

```go
// payment is a manza.TransferDraft (decoded via resp.Decode) or your own record.
// Its ExternalAccountID, DestinationAccountID, ClientReference and
// TransferMethod are *string: exactly one of the first two is set,
// ClientReference is nil for a transfer without a client_reference, and
// TransferMethod is nil where the market offers no rail choice.
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
clientReference, transferMethod := "", ""
if payment.ClientReference != nil {
    clientReference = *payment.ClientReference
}
if payment.TransferMethod != nil {
    transferMethod = *payment.TransferMethod
}
input := manza.SignatureInput(manza.TransferAuthorizationFields{
    PaymentID:       payment.ID,
    Nonce:           nonce, // data.authorization.nonce from the webhook
    Amount:          payment.Amount, // the API's string verbatim, e.g. "2500.0"
    CurrencyCode:    payment.CurrencyCode,
    AccountID:       payment.AccountID,
    Payee:           payee,
    ClientReference: clientReference, // "" when the transfer has none
    TransferMethod:  transferMethod,  // always from your record; "" only when the API returned null
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
error carrying the existing payment's `PaymentID`. A valid signature on a
transfer over its rail's limit returns a `KindValidation` error of `Type`
`state_error`: nothing moved, the authorization is `superseded` with
`refusal_reason` `rail_limit_exceeded`, and the payment went to your
approvers.

## Errors

Non-2xx responses come back as `*manza.Error` with `Status`, `Kind`
(`manza.KindAuthentication`, `KindForbidden`, `KindNotFound`,
`KindValidation` for 400 and any other 422, `KindPlanLimitReached` for a 422 whose
`Type` is `plan_limit_reached`, `KindConflict` for 409, `KindRateLimit`,
`KindServer`, `KindAPI`), the API's `Type`/`Message`/`Param`, and the
`RequestID`. A conflict also carries `PaymentID`; a plan limit carries
`LimitKind` (what is capped, e.g. `invoices`), `LimitCurrent` and `LimitMax`,
and retrying it before the plan changes or its period rolls over does not
help. Discriminate on `Kind`, not on status codes:

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
| Go | [getmanza/manza-go](https://github.com/getmanza/manza-go) (this repo) | `go get github.com/getmanza/manza-go/v2` |
| PHP | [getmanza/manza-php](https://github.com/getmanza/manza-php) | `composer require manza/manza-php` |
| Rust | [getmanza/manza-rust](https://github.com/getmanza/manza-rust) | `cargo add manza` |
| Crystal | [getmanza/manza-crystal](https://github.com/getmanza/manza-crystal) | shard `manza` (`github: getmanza/manza-crystal`) |
| Elixir | [getmanza/manza-elixir](https://github.com/getmanza/manza-elixir) | `{:manza, "~> 1.0"}` |
| CLI | [getmanza/cli](https://github.com/getmanza/cli) | `npm install -g @getzazu/cli` or `brew install getmanza/tap/zazu` |
