# Changelog

All notable changes to `zazu-go` are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [2.0.0] - 2026-11-14

Ports manza-ruby 2.0.0: synced to the Manza API at getmanza/app `906ec486e5`
(API version `2026-11-13`). Cassettes are pinned to `cassettes-v2.0.0`. A
major release: see Migrating from 1.x below.

### Removed

- **Breaking:** `InvoicesService.CreatePaymentLink` (`POST /api/invoices/:id/payment_link`). The API retired the endpoint: ask for the link with `"generate_payment_link": true` on `Invoices.Create` or `Invoices.Update` instead
- **Breaking:** the v1 transfer-authorization signer. `SignatureInput` builds only the v2 input (below)

### Changed

- **Breaking:** the module path is `github.com/getmanza/manza-go/v2`. Go requires the major-version suffix from v2 on: a `v2.0.0` tag on the 1.x path cannot be fetched. The package name stays `manza`
- **Breaking:** `SignatureInput` signs `manza.transfer-authorization.v2` and appends `TransferAuthorizationFields.TransferMethod` after the client reference (`""` when the payment's `transfer_method` is null): `manza.transfer-authorization.v2|payment_id|nonce|amount|currency_code|account_id|payee|client_reference|transfer_method`. The API verifies v2 for every request that is unpinned or sends `Manza-Version: 2026-11-13`; only `2026-03-27` still verifies v1. Pass the payment's `transfer_method` as the API returned it
- **Breaking:** a 422 of type `plan_limit_reached` is an `*Error` of the new `KindPlanLimitReached`, not `KindValidation` (the API used to answer a plan cap with a 500, `KindServer`)
- The example authorizer (`examples/transfer_authorizer`) records the create response's `transfer_method` in its ledger (`Entry.TransferMethod`), checks it against the webhook's `payment.transfer_method` (a null matches an entry without one), signs v2, and compares its input with the webhook's `signature_input_v2`. The preflight points at `trust_new_payees`
- Invoice docs: `account_id` is required on `Invoices.Create` (without one the API answers 422 `state_error` with no `Param`; an account that is not the entity's, 422 with `Param` `account_id`), and `Invoices.Update` takes it too. An invoice's `status` may be `declined`, and `Invoices.Delete` also takes a `declined` invoice that has no number

### Added

- `KindPlanLimitReached` (the other SDKs' `PlanLimitReachedError`, a validation subclass): a 422 whose `Type` is `plan_limit_reached`. `Error.LimitKind` names what is capped (e.g. `invoices`), `LimitCurrent` the count the entity holds and `LimitMax` the cap. Retrying does not help until the plan changes
- `generate_payment_link` on `Invoices.Create` and `Invoices.Update` (needs the `payment_links:write` scope; taken only while payment links are enabled for the entity). `Invoice.GeneratePaymentLink` and `Invoice.Account` (`InvoiceAccount`: `ID`, `Name`, `CurrencyCode`, `RIB` in Morocco, `AccountNumber` in South Africa) decode them
- `transfer_method` on `Payments.Create`: `TransferMethodEFT` or `TransferMethodRTC` in South Africa, `TransferMethodStandard` or `TransferMethodInstant` in Morocco; one the market does not offer returns a `KindValidation` error with `Param` `transfer_method`. `TransferDraft.TransferMethod` (`*string`) is the rail sent, the provider's default when left out, nil where the market offers no choice
- `TransferAuthorizer.TrustNewPayees` and `NewPayeeLimit` (`*string`, nil while `trust_new_payees` is off)
- Documented: the authorization `refusal_reason` `rail_limit_exceeded` (status `superseded`): an authorized transfer over its rail's limit at execution, sent to the approvers; `Authorize` returns a 422 `KindValidation` error (`state_error`). A non-boolean `generate_payment_link` returns a 400 `KindValidation` error
- Documented: the `payment.authorization_requested` webhook's `signature_input_v2` and `payment.transfer_method` (events stay maps: read them from `data`)

### Fixed


- Typed by-id and action calls (`Payments.Get`/`Authorize`/`Decline`, `Customers.Get`/`Update`/`Delete`, `Invoices.*`, `WebhookEndpoints.*`, `Accounts.Get`, `Beneficiaries.Get`, …) sent `/api%2F<resource>/<id>` because the base path's `/` was escaped. They now send `/api/<resource>/<id>`; ids stay escaped inside their own segment (`a/b` → `a%2Fb`) (#14)
- The cassette replay matcher compares the escaped request path instead of Go's decoded `URL.Path`, so a `%2F` in the request target no longer matches a recorded `/`

### Fixed (API side, no wire change)

- `Accounts.ListTransactions` and `Accounts.GetTransaction` return posted movements only (getmanza/app 818432457a); the SDK is unchanged

### Changed (cassettes)

Cassettes come from `cassettes-v2.0.0` (was `v1.3.0`): every cassette was re-recorded against staging. New replay tests mirror manza-ruby 2.0.0's specs:

| Cassette | Request |
|---|---|
| `invoices/create` | `POST /api/invoices` with `account_id`, `generate_payment_link: true`, `issue_date` `2026-11-14`, `due_date` `2026-12-14` |
| `invoices/create_invalid_generate_payment_link` | `generate_payment_link: "yes please"`: 400 |
| `invoices/create_without_account` | no `account_id`: 422 `state_error` |
| `invoices/update` | `PATCH /api/invoices/fixture-invoice-id` with `account_id`, `notes`, `generate_payment_link: true` |
| `payments/create` | `POST /api/payments` with `transfer_method: "standard"` |
| `payments/create_invalid_transfer_method` | an unoffered `transfer_method`: 422, `Param` `transfer_method` |

The authorize tests sign v2 over `transfer_method` `standard`; replay still matches their body without the scrubbed `signature`.

### Migrating from 1.x

- **New import path.** `go get github.com/getmanza/manza-go/v2@v2.0.0` and change every `import manza "github.com/getmanza/manza-go"` to `import manza "github.com/getmanza/manza-go/v2"`. Identifiers are unchanged apart from the breaks below
- **`CreatePaymentLink` is gone.** Replace `client.Invoices.CreatePaymentLink(ctx, id, accountID)` with `client.Invoices.Update(ctx, id, manza.Attributes{"account_id": accountID, "generate_payment_link": true})`, or pass `"generate_payment_link": true` on create. `Update` returns the invoice, not a link: the link is made from the invoice's account when the invoice is approved and appears in the invoice's `payment_link`. `Update` takes only an invoice that is still `draft`, `pending_approval` or `declined` (any other is a `KindValidation` error, `state_error`); for an invoice already approved or sent there is no API path to attach a link: `client.PaymentLinks.Create` makes a standalone one
- **`account_id` is required on invoice create.** Pass the account the invoice is paid into (`client.Accounts.List`); 1.x calls without it now return a `KindValidation` error
- **v2 signatures only.** Set `TransferMethod` on every `TransferAuthorizationFields` from your record of the payment (the create response's `transfer_method`; `""` only when it is null), and compare with the webhook's `signature_input_v2` instead of `signature_input`. Go cannot require the field: one left out signs an empty segment, which the API refuses for a payment that carries a rail, and five refusals fail the challenge. A client pinned to `Manza-Version: 2026-03-27` (`WithAPIVersion` or `MANZA_API_VERSION`) is verified v1 and must stay on manza-go 1.x; unpin it, or pin `2026-11-13`, to move to 2.0
- **Plan caps.** A check for `KindServer` around creates that may hit a plan cap no longer matches: check `KindPlanLimitReached`, and do not retry it. It is not `KindValidation`, so a `Kind == manza.KindValidation` check does not catch it either

## [1.1.0]

Ports manza-ruby 1.1.0–1.3.0: the webhook verifier, the transfer-authorizer
resource, payments as the canonical name for transfer drafts, and a runnable
authorizer example. Cassettes are pinned to `cassettes-v1.3.0`.

### Added

- Webhook verifier (`webhook.go`): `VerifyWebhook` (a `bool`), `ConstructWebhookEvent` (the parsed event, or a `*manza.ArgumentError`) and `SignWebhook`. Signed content is `"<timestamp>.<raw body>"`, hex HMAC-SHA256, compared in constant time. Reads `X-Manza-*` headers in any case, the CGI `HTTP_X_MANZA_*` form and the legacy `X-Zazu-*` names; checks the timestamp within 5 minutes (`WithWebhookTolerance`, `WithoutWebhookTolerance` for local replay only, `WithWebhookNow`). Passes the shared fixed vector. No new error class
- `Client.Payments` (`PaymentsService`): `Create`, `Get`, `Authorize` and `Decline` on the API's canonical paths, `POST /api/payments`, `GET /api/payments/:id`, `POST /api/payments/:id/authorize` and `POST /api/payments/:id/decline`. Request and response bodies are unchanged, and so is the signature input
- `Client.TransferAuthorizer.Get` (`GET /api/transfer_authorizer`) and the `TransferAuthorizer` model: status, `suspension_reason`, limits, `daily_authorized_total`, `secret_expires_at`, `webhook_endpoint_id`. A not-found `*manza.Error` means none is enrolled
- `Authorization.RefusalReason` (`refusal_reason`, set when a payment went to the approvers) and `ExternalAccount.Trusted` (`trusted`). The webhook's `payment.payee_trusted` passes through in the event map
- `examples/transfer_authorizer/`: a creator, an authorizer webhook server and a preflight on `net/http`, sharing a JSON ledger, with tests against a stubbed API. Its own Go module, so it is not in the published module
- README: "Setup, step by step" for machine-authorized transfers, and the verifier

### Deprecated

- `Client.TransferDrafts` and `TransferDraftsService`. Both keep working for all of 1.x: the field points at the same service as `Client.Payments` and the type is an alias of `PaymentsService`, so the old name now calls `/api/payments` too. The server keeps `/api/transfer_drafts` as a permanent alias

### Changed

- Cassettes come from `cassettes-v1.3.0` (was `v1.0.0`): the payment cassettes are under `payments/`, and `payments/create` and `beneficiaries/create` replay the request bodies manza-ruby 1.2.0 records

## [1.0.0]

### Renamed from zazu-go to manza-go

The SDK is now `manza-go`. This is a source-breaking change for Go: update `go.mod` and every import. A GitHub redirect does not fix the module path. Go 1.0.0 needs no `/v2` suffix.

- Module `github.com/getmanza/manza-go`, package `manza`; error messages are prefixed `manza: `
- Request header `Manza-Version` (was `Zazu-Version`); User-Agent `manza-go/<version>`
- `MANZA_API_KEY`, `MANZA_BASE_URL` and `MANZA_API_VERSION` are read first. The `ZAZU_*` names still work as a fallback and print a one-time deprecation warning per variable to stderr, for all of 1.x
- Cassettes are fetched from `getmanza/manza-ruby`, pinned to `v1.0.0`; fixture env vars are `MANZA_FIXTURE_*` (no fallback, dev-only)

#### Migration

| Old | New |
|---|---|
| `go get github.com/getzazu/zazu-go` | `go get github.com/getmanza/manza-go` |
| `import zazu "github.com/getzazu/zazu-go"` | `import manza "github.com/getmanza/manza-go"` |
| `package zazu`, `zazu.New`, `zazu.Client`, `zazu.Error`, ... | `manza.New`, `manza.Client`, `manza.Error`, ... (same identifiers under the new package name; none contained `Zazu`) |
| `ZAZU_API_KEY` / `ZAZU_BASE_URL` / `ZAZU_API_VERSION` | `MANZA_API_KEY` / `MANZA_BASE_URL` / `MANZA_API_VERSION` |
| `Zazu-Version` header | `Manza-Version` |
| `ZAZU_FIXTURE_*` (tests) | `MANZA_FIXTURE_*` |

## [0.3.0]

Syncs the SDK with the API changes since 2026-07-16, matching zazu-ruby 0.3.0.

### Added

- `Kind` "conflict" (`zazu.KindConflict`) for 409, with `Error.PaymentID` read from `error.payment_id` (the 10th error class). 400 now maps to `KindValidation`; the `Kind*` constants are exported
- `zazu.ArgumentError` for values the SDK refuses to send
- `TransferDrafts.Authorize` (refuses a blank signature locally) and `TransferDrafts.Decline` (omits an empty reason); `client_reference` documented on `Create`
- Transfer-authorization signer: `SignatureInput`, `Sign` (lowercase hex HMAC-SHA256) and `PayeeFor`, tested against the shared fixed vectors
- `Beneficiaries.Create`, `ListExternalAccounts`, `GetExternalAccount`, `CreateExternalAccount`
- `PayeeTrustRequests` service (`Create`, `Get`)
- Typed response models (`types.go`) and `Response.Decode`: `client_reference`, `authorization`, `settled_at`, `transaction`, `billing_address`, `collect_billing_address`, `customer_name`, `registration_number`, `vat_number`, the `clearing` status; `tax_id`, `ice_number` and `delivery_date` are optional; `beneficiary_type` is `individual` or `business`
- Replay tests for every new cassette; the three authorize cassettes match the request body with `signature` removed

### Changed

- Default base URL is now `https://ma.manza.finance` (`https://za.manza.finance` for South Africa); the replay cassettes are recorded against `https://ma.manza.dev`
- Beneficiaries are no longer documented as dashboard-only

## [0.2.1]

Version alignment: the whole SDK family now releases in lockstep with zazu-ruby. No functional changes since [0.1.0].

## [0.1.0]

Initial release.

### Added

- `zazu.Client` built on `net/http` (functional options, context-first API)
- Services: `Accounts`, `Beneficiaries`, `CheckoutSessions`, `Customers`, `Entity`, `Invoices`, `PaymentLinks`, `TransferDrafts`, `WebhookEndpoints`
- Cursor-based `Page` with `Next(ctx)` (max 100 records per page)
- `*zazu.Error` mirroring the shared SDK error taxonomy
- Cassette-replay test harness driven by the Ruby SDK's release tarball
