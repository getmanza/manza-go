---
description: "Use when CI checks are failing on a PR — fetches failure logs, diagnoses root causes, implements fixes, pushes until CI is green."
model: opus
argument-hint: "PR number (e.g., 1690 or #1690)"
allowed-tools: Bash(gh pr view:*), Bash(gh pr checks:*), Bash(gh pr diff:*), Bash(gh api:*), Bash(gh run view:*), Bash(git log:*), Bash(git diff:*), Bash(git push:*), Bash(git commit:*), Bash(git add:*), Bash(go:*), Bash(gofmt:*), Bash(scripts/fetch-cassettes.sh:*), Read, Write, Edit, Glob, Grep, Agent
---

# Fix GitHub CI Failures: $ARGUMENTS

Diagnose and fix CI failures. Work systematically: identify failures → read logs → diagnose root cause → fix locally → verify → push.

## Phase 0: Determine the PR

Number → PR. `#N` → strip `#`. Empty → current branch (`gh pr view --json number`).

## Phase 1: Inventory failures

```bash
gh pr checks <PR>
```

For each failing check, get the run id and load the failed logs:

```bash
gh run view <run-id> --log-failed
```

Categorize:
- **Test failures** — assertion failed, `no cassette interaction matches ...`, timeout
- **Format failures** — `gofmt needed on:` lists the files
- **Vet / compile failures** — `go vet ./...` errors, unused imports or variables
- **Cassette fetch failures** — `scripts/fetch-cassettes.sh` could not resolve the tag or download the tarball
- **Release failures** — `release.yml`: tag does not match the `Version` constant in `client.go`, tests red, GitHub release or module proxy warm-up

## Phase 2: Diagnose

Read the actual error message, not the surrounding noise. The first stacktrace line that points at our code is usually the culprit.

For each failure:

### Reproduce locally

```bash
# Cassettes (manza-ruby's release pinned in the script, as CI does)
scripts/fetch-cassettes.sh

# Test
go test -run TestName ./...

# Format (must print nothing)
gofmt -l .

# Vet
go vet ./...

# Full pipeline
gofmt -l . && go vet ./... && go test ./...
```

If you can't reproduce locally, the failure is environmental (CI-only):
- Different Go version → CI pins `go-version: "1.24"` in `ci.yml` and `release.yml`; `go.mod` says `go 1.24`
- Newer cassettes → CI fetches the manza-ruby release pinned in `scripts/fetch-cassettes.sh` (`PINNED_TAG`), so a Ruby release only reaches CI when the pin is bumped (run `scripts/fetch-cassettes.sh` locally to reproduce)
- Race condition → re-running the job fixes it
- Network → external service (cassette tarball download, module proxy) hiccup; the fetch script already retries

### Find the root cause

Apply the five-whys ladder until you reach a fix point that prevents the same class of failure recurring. Don't:

- Disable or `t.Skip` the failing test
- Loosen the cassette matcher so the replay passes
- Discard an error with `_ =` to quiet `go vet`
- Call a live Manza API to "check" a cassette (never; only manza-ruby records)

These hide the failure; the underlying bug returns elsewhere.

## Phase 3: Fix and verify

### 3.1 Implement the fix

Touch only what the failure cites, plus what the fix requires.

### 3.2 Run the equivalent local check

The CI step that failed has a local equivalent — run it, get green:

| CI step | Local equivalent |
|---|---|
| Fetch cassettes | `scripts/fetch-cassettes.sh` |
| gofmt | `gofmt -l .` (fix with `gofmt -w .`) |
| go vet | `go vet ./...` |
| go test | `go test ./...` |

### 3.3 Run the full pipeline

```bash
gofmt -l . && go vet ./... && go test ./...
```

### 3.4 Commit + push

```bash
git add <files>
git commit -m "fix(ci): <what was failing>

<root cause and how this addresses it>"
git push origin <branch>
```

Use `fix:` for prod fixes, `chore(ci):` for workflow / config changes.

## Phase 4: Watch the next run

```bash
gh pr checks <PR> --watch
# or
gh run watch <run-id> --exit-status
```

Track until green. If the same step fails again with a different error, repeat. If it fails the same way, your fix is wrong — revert and rethink.

## Phase 5: Verify and document

```bash
gh pr checks <PR>            # all green
gh pr view <PR> --json mergeable,reviewDecision
```

If the failure was CI-config drift (workflow YAML out of sync with reality), also update relevant docs:
- `go.mod` `go` directive and the `go-version` in `.github/workflows/ci.yml` and `release.yml`
- `CLAUDE.md` if a convention changed

## Common patterns and fixes

### Replay says "no cassette interaction matches"

The request shape drifted from the recording. Check, in order:
- Two cassettes sharing method + URI were loaded in one test (`transfer_drafts/authorize` vs `authorize_same_key`, `create` vs `create_duplicate`): load one per test.
- A `fixtureIDs` entry in `cassette_test.go` drifted from manza-ruby's `spec/support/fixture_ids.rb`: the URI no longer matches.
- Body mismatch: matching is semantic JSON (the three `transfer_drafts/authorize*` cassettes ignore `signature`). Fix the SDK's request, never the cassette. A genuinely new request shape means re-recording in manza-ruby and shipping a new SDK version.

### Cassette fetch fails

`scripts/fetch-cassettes.sh` downloads `cassettes-<PINNED_TAG>.tar.gz` from the manza-ruby GitHub release. Check that the release exists and carries the tarball. A transient 5xx is already retried 8 times.

### Release says the tag does not match the Version constant

`release.yml` compares the pushed `vX.Y.Z` tag to `Version` in `client.go`. `bin/release` writes it through `scripts/version`; a hand-made tag skips that. Delete the tag and release through `bin/release`. Do not edit `bin/release`.

## Karpathy guidelines

- **Think before coding** — read the actual error, don't pattern-match on the first guess.
- **Goal-driven execution** — the green CI check is the verification.
- **Surgical changes** — fix the failing class of error, not adjacent things.
