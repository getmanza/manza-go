# Changelog

All notable changes to `zazu-go` are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### 1.0.0: renamed from zazu-go to manza-go

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
