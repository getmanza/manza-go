# zazu-go

Go SDK for the Zazu API. This SDK **replays zazu-ruby's cassettes**; it never records. zazu-ruby is the reference implementation of the cross-language SDK family: the same cassettes are replayed by zazu-ts, zazu-python and the rest, which is what keeps the wire format identical everywhere.

## Stack

| Concern | Tool | Notes |
|---|---|---|
| Language | Go 1.24 | `go.mod` (`go 1.24`); CI runs one version, 1.24 (`.github/workflows/ci.yml`) |
| HTTP | `net/http` | `client.go`. Default timeout 30s; swap via `WithHTTPClient` |
| Test runner | `go test` | Flat package: `*_test.go` next to the source in the repo root |
| Cassette replay (tests) | `httptest.Server` + `gopkg.in/yaml.v3` | `cassette_test.go`. Reads zazu-ruby's release tarball from `testdata/cassettes/` (gitignored) |
| Format | `gofmt` | CI fails on any file `gofmt -l .` prints |
| Lint / type-check | `go vet ./...` | No golangci-lint, no separate typechecker (the compiler is it) |
| Package registry | none | The Go module proxy indexes git tags (`proxy.golang.org`, `pkg.go.dev`). Module path is `github.com/getzazu/zazu-go` until the rename plan moves it |
| Release | `bin/release` | zazu SDK release kit (byte-identical across SDK repos; repo-specific bits in `scripts/version` + `scripts/release-check`) |

## Public API surface

```go
client, err := zazu.New(zazu.WithAPIKey("sk_live_...")) // or ZAZU_API_KEY

resp, err := client.Entity.Get(ctx)
page, err := client.Accounts.List(ctx, zazu.AccountListParams{CurrencyCode: "MAD"})
page, err = client.Customers.List(ctx, zazu.CustomerListParams{Query: "Acme"})
_, err = client.PaymentLinks.Cancel(ctx, id)

// Transfer drafts (0.3.0): create, then answer the authorization webhook
resp, err = client.TransferDrafts.Create(ctx, zazu.Attributes{"account_id": accountID, "amount": "10.0", "client_reference": "inv-42"})
input := zazu.SignatureInput(zazu.TransferAuthorizationFields{ /* from YOUR own record */ })
_, err = client.TransferDrafts.Authorize(ctx, draftID, authorizationID, zazu.Sign(secret, input))
_, err = client.TransferDrafts.Decline(ctx, draftID, authorizationID, "wrong amount")

// Beneficiaries, external accounts, payee trust (0.3.0)
client.Beneficiaries.Create(ctx, attrs)
client.Beneficiaries.ListExternalAccounts(ctx, beneficiaryID, zazu.ListParams{})
client.Beneficiaries.GetExternalAccount(ctx, beneficiaryID, id)
client.Beneficiaries.CreateExternalAccount(ctx, beneficiaryID, attrs)
client.PayeeTrustRequests.Create(ctx, []string{externalAccountID})

// Errors: one *zazu.Error with a Kind, discriminate with errors.As
var apiErr *zazu.Error
if errors.As(err, &apiErr) && apiErr.Kind == zazu.KindConflict {
    existing := apiErr.PaymentID // the draft already holding this client_reference
}
```

- Services on `Client`: `Accounts`, `Beneficiaries`, `CheckoutSessions`, `Customers`, `Entity`, `Invoices`, `PayeeTrustRequests`, `PaymentLinks`, `TransferDrafts`, `WebhookEndpoints`.
- `zazu.Page` (`page.go`): cursor-based, hard cap `MaxPerPage` = 100; `Next(ctx)` returns nil on the last page.
- `*zazu.Response` carries `Body` (snake_case `map[string]any`, as-is) and `Raw`. `Response.Decode` maps it onto the typed models in `types.go` (additive; `Body` is unchanged).
- Errors: a single `*Error` with a `Kind` (`KindAuthentication`, `KindForbidden`, `KindNotFound`, `KindValidation` for 400 and 422, `KindConflict` for 409 with `PaymentID`, `KindRateLimit`, `KindServer`, `KindAPI`), plus `*ConfigurationError`, `*ConnectionError` and `*ArgumentError` (a value the SDK refuses to send, no HTTP made, e.g. a blank signature). Never match status codes.
- Signer (`transfer_authorization.go`): `SignatureInput`, `Sign`, `PayeeFor`.
- Snake-case wire format: bodies are returned as-is. **No auto-camelCasing.**

## How to work in this codebase

1. **Tests come first.** Every change to a non-test `.go` file ships with a test. Cassette-replay tests are the contract: they enforce the same wire format across Ruby, TS, Go and the rest.
2. **Use the SDK's primitives.** `Page` via `listPage`, `*Error` and its `Kind`, `encodePath` for every URL path segment, `Client.get/post/patch/delete`, `fixtureID()` in tests. Don't hand-roll `http.NewRequest` calls, concatenate IDs into paths, or switch on status codes.
3. **Snake-case stays.** Response keys and typed-model `json` tags are wire format.
4. **`gofmt` and `go vet` must be clean.** CI gates on both. Don't add `//nolint`-style escapes; fix the issue.

## Critical rules

- **Never call a live Zazu/Manza API.** Not from tests, scripts or Claude sessions. Tests replay zazu-ruby's cassettes against an `httptest.Server` only. Live staging calls create real transfers and approval requests for the team. Only zazu-ruby records cassettes.
- **`gofmt -l .`, `go vet ./...` and `go test ./...` before every commit.** CI runs the same (`scripts/release-check` too).
- **Cassette contract.**
  - Cassettes come from the newest zazu-ruby `v*` release (`cassettes-vX.Y.Z.tar.gz`) via `scripts/fetch-cassettes.sh`, into `testdata/cassettes/`. They are recorded against `https://ma.manza.dev`; the harness fails any cassette whose URI is on another host (`replayHost`).
  - Load **one cassette per test** where two share method + URI: `transfer_drafts/authorize` vs `authorize_same_key`, and `create` vs `create_duplicate`. The server serves the first matching interaction.
  - The three `transfer_drafts/authorize*` cassettes match method + path + query + the JSON body minus `signature` (the recorded value is a scrubbed HMAC replay cannot reproduce).
  - Every other cassette matches method + path + query + **semantic JSON body** (`jsonEqual`: parsed and compared, not byte-for-byte, because Go sorts map keys). Bodies built from structs keep the recorded key order.
  - Cassette responses carry no `Content-Length`; `net/http` computes it. Don't add it.
  - The `fixtureIDs` table in `cassette_test.go` must stay identical to zazu-ruby's `spec/support/fixture_ids.rb` (29 entries). Tests call `fixtureID(t, "ZAZU_FIXTURE_X")`.
- **Hosts.** Default `https://ma.manza.finance`, South Africa `https://za.manza.finance`, staging and cassettes `https://ma.manza.dev`. Env var names stay `ZAZU_*` (`ZAZU_API_KEY`, `ZAZU_BASE_URL`, `ZAZU_API_VERSION`) and the package stays `zazu` until the rename plan (zazu-ruby `docs/plans/2026-10-manza-rename.md`).
- **Error model is shared across the SDK family.** Adding an error class or `Kind` means coordinating zazu-ruby and zazu-ts at minimum. The 10th is the conflict (409). `Kind` string values are part of the contract.
- **Signer.** `SignatureInput` and `Sign` must keep reproducing the two fixed vectors in `transfer_authorization_test.go`, the same vectors as zazu-ruby's `spec/zazu/transfer_authorization_spec.rb`. Never sign the server's `signature_input` blindly: build it from your own record of the transfer, and pass `amount` verbatim (`"2500.0"`).
- **Release.** `bin/release` is byte-identical across the SDK repos and is never edited in place. Repo-specific logic lives in `scripts/version` (the `Version` constant in `client.go`) and `scripts/release-check`. `release.yml` gates on tag == `Version`. There is no registry token or trusted-publishing environment: a Go module's version is its git tag and the proxy indexes it. If a registry is ever added, its trusted-publisher binding must name `getmanza/zazu-go`.
- **The repo lives at `getmanza/zazu-go`** (moved from `getzazu`). Remotes and URLs must say `getmanza`. Still stale and tracked for the rename plan: `scripts/fetch-cassettes.sh` (`REPO="getzazu/zazu-ruby"`), the module path in `go.mod` and `release.yml`, and README links.
- **Never escape backticks in PR bodies.** With `<<'EOF'` (single-quoted heredoc) the shell passes everything through verbatim. Typing `` \` `` produces literal `` \` `` in the rendered PR. See "PR descriptions" below.

## PR descriptions

Write PR description bodies in plain Markdown. **Do not escape backticks** with `` \` `` — GitHub renders `` \` `` literally as a backslash followed by a backtick, producing output like `` \`Page\` `` instead of the monospace `Page` the reader expects.

The usual cause is writing the description inside a bash heredoc (`gh pr create --body "$(cat <<'EOF' ... EOF)"`) and then reflexively escaping every backtick because of shell-quoting muscle memory. With `<<'EOF'` (single-quoted delimiter) the shell does NOT interpret anything inside the heredoc — backticks, dollars, and backslashes all pass through verbatim. So write them exactly as you want them rendered:

```bash
# Good — renders as `Page` in monospace
gh pr create --body "$(cat <<'EOF'
Uses the `Page` helper.
EOF
)"

# Bad — renders as \`Page\` literally in the PR body
gh pr create --body "$(cat <<'EOF'
Uses the \`Page\` helper.
EOF
)"
```

Same rule for code blocks — write triple-backticks unescaped. The single-quoted heredoc delimiter is doing all the shell-escaping work. If you find yourself typing `` \` `` inside a PR body, stop and remove the backslash.

## Striving for excellence

These are the Karpathy guidelines we apply on every change. They reduce common LLM coding mistakes.

### 1. Think before coding

Don't assume. Don't hide confusion. Surface tradeoffs.

- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them — don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

### 2. Simplicity first

Minimum code that solves the problem. Nothing speculative.

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Senior engineer test: would they call this overcomplicated?

### 3. Surgical changes

Touch only what you must. Clean up only your own mess.

- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it — don't delete it.
- Remove imports/variables/functions that *your* changes orphaned. Don't remove pre-existing dead code unless asked.

### 4. Goal-driven execution

Define success criteria. Loop until verified.

- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan with verification at each step.

## Development workflow

The exact steps from `.github/workflows/ci.yml`:

```bash
# One-time setup (Go 1.24+), and again when zazu-ruby ships a new release
scripts/fetch-cassettes.sh            # latest zazu-ruby v* tarball -> testdata/cassettes/
scripts/fetch-cassettes.sh v0.3.0     # or a specific tag

# Daily loop
go test -run TestTransferDrafts ./...  # while iterating
go test ./...                          # full suite (CI)
gofmt -l .                             # must print nothing
gofmt -w .                             # fix formatting
go vet ./...                           # lint

# Release (after PR merge, from a clean, up-to-date main)
bin/release list        # last releases + what patch/minor/major would give
bin/release --dry-run   # version + changes since the last tag, publishes nothing
bin/release minor       # or patch (default), major, an explicit 0.4.0; --force re-creates
# -> bumps the Version constant in client.go, runs scripts/release-check, pushes main, publishes the GH release
# -> release.yml gates on tag == Version, then tests and warms the module proxy index
```

## Models

Sessions run on `opus` (Opus 5.5) with `fable` (Fable 5.1) as the advisor (`.claude/settings.json`). Fable is spent where judgment matters most: ask for a plan on Fable (a `fable` subagent or `/model fable`); plan mode itself runs on Opus and asks the advisor. The advisor is consulted at decision points (before choosing an approach, a schema or public API, a migration, a dependency, anything irreversible, and when a failure repeats). The `fable-validator` agent checks every finished implementation before its pull request opens (`/lfg`, Phase 6.5). Agents pin their tier by alias, never by full model ID: `fable` for plans and validation; `opus` for orchestration, security, full PR review, payments and production debugging; `sonnet` for the implementation specialists and TDD; `haiku` for mechanical scans. Every spawned agent names its `model:`; a subagent whose definition names no model runs on `sonnet` (`CLAUDE_CODE_SUBAGENT_MODEL`), never on the session's model.

## Slash commands

These live in `.claude/commands/` and are available in any Claude Code session:

| Command | When |
|---|---|
| `/lfg <issue or feature>` | Full autonomous workflow with TDD + verification |
| `/github-review-pr <PR#>` | Full PR review pass — failures first, then comments |
| `/github-review-failures <PR#>` | Just fix CI failures on a PR |
| `/github-review-comments <PR#>` | Just respond to reviewer comments on a PR |
| `/coderabbit-review <PR#>` | Specifically address CodeRabbit findings (verify, fix valid, push back on stale/wrong) |

## Cross-SDK contract

zazu-ruby is the source of truth:

- It records cassettes against `https://ma.manza.dev` and ships them as a release tarball (`cassettes-vX.Y.Z.tar.gz`) on each version.
- Every other SDK (zazu-ts, zazu-python, this one, ...) replays them in its own harness.
- Cassettes always come from the **newest** zazu-ruby release, so a new Ruby release turns CI red here until this SDK catches up.

If the contract breaks (new request shape, new error kind), it is a coordinated change across at least two repos: zazu-ruby and zazu-ts.

## Repository links

- This repo: https://github.com/getmanza/zazu-go
- Go package docs (module proxy, no registry): https://pkg.go.dev/github.com/getzazu/zazu-go
- Reference implementation: https://github.com/getmanza/zazu-ruby
- TypeScript SDK: https://github.com/getmanza/zazu-ts
