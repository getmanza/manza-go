# Changelog

All notable changes to `zazu-go` are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- Typed by-id and action calls (`Payments.Get`/`Authorize`/`Decline`, `Customers.Get`/`Update`/`Delete`, `Invoices.*`, `WebhookEndpoints.*`, `Accounts.Get`, `Beneficiaries.Get`, …) sent `/api%2F<resource>/<id>` because the base path's `/` was escaped. They now send `/api/<resource>/<id>`; ids stay escaped inside their own segment (`a/b` → `a%2Fb`) (#14)
- The cassette replay matcher compares the escaped request path instead of Go's decoded `URL.Path`, so a `%2F` in the request target no longer matches a recorded `/`

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
