# Changelog

All notable changes to `zazu-go` are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
