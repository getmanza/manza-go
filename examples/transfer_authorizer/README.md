# Transfer authorizer example

Two processes and a shared ledger, matching the two-key model of [transfer authorization](https://ma.manza.finance/docs/api/transfer-authorization):

| Path | Process | Key |
|---|---|---|
| `cmd/create_transfer` (`create.go`) | Creates a payment, records it in `ledger.json` from the API's response | A (`transfers:write`) |
| `cmd/authorizer` (`authorizer.go`) | Receives `payment.authorization_requested`, verifies it, checks the ledger, signs and authorizes (or declines) | B (`transfers:authorize`) |
| `cmd/preflight` (`preflight.go`) | Checks every setup step and prints the fix for each that fails | A and B |
| `ledger.go` | Your own record of what you created, keyed by `client_reference`. Swap it for your database | — |

It is its own Go module (`go.mod` here, with a `replace` onto this checkout of the SDK), so it is not part of the published `github.com/getmanza/manza-go` module. It uses the standard library only.

## Setup

Do steps 1–6 of [Setup, step by step](../../README.md#setup-step-by-step) in the SDK README first: feature flags, keys A and B, a stable tunnel URL, the webhook endpoint, enrolment, and new payees allowed or a trusted payee.

```bash
cd examples/transfer_authorizer
cp .env.example .env         # fill in both keys, the signing secret, the account and the beneficiary
go run ./cmd/preflight       # or: go run ./cmd/preflight 250.00, the amount you will send
```

Every command reads `.env` from the current directory and lets it override what your shell exported.

## Preflight

`cmd/preflight` runs each applicable check in order (a missing key or id, or the same key twice, stops it before any request; when key B cannot read an enrolled authorizer, the checks that need it are skipped) and prints one line per check: `✓` passed, `✗` failed (followed by the fix; the exit status is 1), `!` a warning the API cannot settle.

| Check | Fails when |
|---|---|
| Keys | any of `MANZA_CREATOR_API_KEY`, `MANZA_AUTHORIZER_API_KEY`, `MANZA_ACCOUNT_ID`, `MANZA_BENEFICIARY_ID` is empty, or the two keys are the same |
| Key A reaches the API | `Entity.Get` with key A is refused: a wrong key, a missing scope, or this machine outside its IP allowlist |
| Transfer authorizer | `TransferAuthorizer.Get` with key B is not `active`: prints the `suspension_reason` and its fix, or that nothing is enrolled |
| Currency | `MANZA_ACCOUNT_ID`'s currency is not the enrolled currency |
| Secret expiry | `secret_expires_at` is less than 7 days away |
| Signing secret | `MANZA_AUTHORIZER_SECRET` is empty |
| Webhook endpoint | the enrolled endpoint (`webhook_endpoint_id`) is not active. On `ngrok-free.app` or `trycloudflare.com` it warns: that URL changes when the tunnel restarts |
| Amount | over the limit per transfer, or over what is left of the limit per 24 hours (`daily_limit` − `daily_authorized_total`) |
| Payee | the beneficiary has no default bank account. When that account is not `trusted` it warns: the transfer qualifies only with *Approve transfers to new payees* on (not visible through the API), key A bound to an IP allowlist and the amount within the limit per transfer to a new payee, and only for bank details never trusted: revoked or edited details always go to your approvers |

## Run

Terminal 1, the authorizer (it listens on `PORT` from `.env`, 4599 by default; point your tunnel at the same port):

```bash
go run ./cmd/authorizer
```

With the default `ALLOW_NEW_PAYEES=true` it authorizes transfers to new payees (and so trusts them). The API only sends it one when *Approve transfers to new payees* is on, key A has an IP allowlist and the amount is within the limit per transfer to a new payee; otherwise the payment goes to your approvers. Set `ALLOW_NEW_PAYEES=false` in `.env` to decline those instead and pay only payees an approver trusted.

Terminal 2, the creator:

```bash
go run ./cmd/create_transfer          # 10.00
go run ./cmd/create_transfer 250.00   # any amount within your limits
```

## What each log line means

`cmd/create_transfer`:

| Line | Meaning |
|---|---|
| `created payment … (client_reference …)` | The payment exists and is in `ledger.json` |
| `challenge sent, waiting for the authorizer` | The payment is inside your envelope: the webhook is on its way to terminal 1 |
| `sent to your approvers, not the authorizer: refused (<reason>), <words>` | The payment is outside the envelope. The `refusal_reason` and what it means: `untrusted_payee`, `unrestricted_api_key` (a new payee, and key A has no IP allowlist), `over_new_payee_limit`, `over_transfer_limit`, `over_daily_limit`, `currency_mismatch`, `suspended`, `secret_expired`. Run the preflight |
| `no transfer authorizer was enrolled` | The payment has no `authorization`: nothing was enrolled, or the feature is off |
| `create failed for client_reference …: … (type …, request_id …)` | The API refused the payment, or did not answer. After a timeout the payment may exist anyway. If it reached the authorizer, terminal 1 shows it declined (not in the ledger) and a rerun is safe. Otherwise look for that `client_reference` among your approvers' payments before rerunning, or the rerun creates a second transfer. Quote the `request_id` to support |

The authorizer (`cmd/authorizer`):

| Line | Meaning |
|---|---|
| `received payment.authorization_requested (…), signature verified` | The delivery's `X-Manza-Signature` matches and its timestamp is fresh |
| `rejected a delivery: …` | Bad signature, stale timestamp, missing header, or a body that is not a JSON object: answered 401 |
| `ignoring …` | Any other event: answered 200, nothing done |
| `ledger match for …` | The payment's id, amount, currency, account and payee match `ledger.json` |
| `new payee ext:…, will be trusted on authorize` | The webhook's `payment.payee_trusted` is `false`: *Approve transfers to new payees* is on and these bank details were never trusted. The authorize trusts them |
| `signature input differs from the webhook's: …` | The input built from the ledger is not the webhook's `signature_input`, so a signature would fail and count toward suspension. The challenge is left unanswered and goes to your approvers when it expires; compare the two strings |
| `… retrying in Ns` | The API timed out, answered 5xx, rate-limited, or answered 409 `authorization_busy` (the payee's bank details were being edited, trusted or revoked). Authorize and decline replay idempotently; after three attempts the `API error` line follows and the challenge goes to your approvers when it expires |
| `signed the challenge for payment …` | HMAC-SHA256 of the ledger's signature input under the signing secret |
| `authorized payment … (status "processing")` | Accepted: execution has started. Poll `Payments.Get` (processing → completed / failed) or subscribe to the `transfer.executed` webhook for the outcome |
| `declined payment …: <reason>` | The payment is not in the ledger, or differs from it, or (with `ALLOW_NEW_PAYEES=false`) it pays a new payee. The payment is deleted |
| `API error <kind> (<status>): … (type …, request_id …)` | Authorize or decline failed. `unrestricted_api_key` (403): key B has no IP allowlist. Look any other `type` up in the docs' errors table |

The authorizer answers the webhook with 200 straight away and does the ledger check, signing and authorize on a goroutine: the delivery has a timeout, and a retried webhook is safe because authorize replays idempotently. It waits up to 10 seconds for the ledger entry, because the webhook can arrive before `cmd/create_transfer` has written it.

## Tests

```bash
go test ./...   # the authorizer, preflight and creator against a stubbed API; no network
```

## Troubleshooting

See the errors table and "Which transfers qualify" in the [transfer authorization docs](https://ma.manza.finance/docs/api/transfer-authorization).
