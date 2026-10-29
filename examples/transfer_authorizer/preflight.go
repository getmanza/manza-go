package transferauthorizer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	manza "github.com/getmanza/manza-go"
)

const secretMinDays = 7

// unstableTunnelHosts hand out a new URL on every restart. An enrolled
// endpoint's URL is locked, so the next restart breaks the authorizer.
var unstableTunnelHosts = []string{"ngrok-free.app", "trycloudflare.com"}

var suspensionFixes = map[string]string{
	"member":             "an approver suspended it: resume it in Settings → Developer → Transfer approval",
	"signature_failures": "five invalid signatures in a row: fix the signing, then an approver resumes it in the app",
	"secret_expired": "an approver rotates the secret in the app (and resumes it if it stays suspended); " +
		"set the new one as MANZA_AUTHORIZER_SECRET",
	"endpoint_disabled": "re-enable the webhook endpoint (WebhookEndpoints.Enable), or enrol a new one if it was " +
		"deleted; if it stays suspended, an approver resumes it in the app",
}

// Plain decimals only: create_transfer sends the string as is, so "1/2" or
// "1e2" would parse as a big.Rat but not mean the same to the API.
var plainDecimal = regexp.MustCompile(`^\d+(\.\d+)?$`)

// Preflight checks every setup step before you create a transfer, and says
// how to fix the ones that fail. It prints one line per step: ✓ passed,
// ✗ failed (followed by the fix), ! a warning the API cannot settle.
type Preflight struct {
	env       func(string) string
	rawAmount string
	amount    *big.Rat // nil when rawAmount is not a positive decimal
	out       io.Writer
	now       time.Time
	ok        bool

	creator, authorizerClient *manza.Client
}

// NewPreflight reads its settings through env (os.Getenv in cmd/preflight)
// and checks a transfer of amount (e.g. "10.00").
func NewPreflight(env func(string) string, amount string, out io.Writer, now time.Time) *Preflight {
	p := &Preflight{env: env, rawAmount: amount, out: out, now: now, ok: true}
	if plainDecimal.MatchString(amount) {
		if r, ok := new(big.Rat).SetString(amount); ok && r.Sign() > 0 {
			p.amount = r
		}
	}
	return p
}

// Run prints one line per step and reports whether no step failed.
func (p *Preflight) Run(ctx context.Context) bool {
	if p.amount == nil {
		p.fail(fmt.Sprintf("amount %q is not a positive decimal", p.rawAmount), "pass an amount like 10.00 or 250.00")
	}
	if !p.keys() {
		return p.ok
	}

	p.creatorReachesEntity(ctx)
	authorizer := p.enrolledAuthorizer(ctx)
	if authorizer != nil {
		p.currencyMatches(ctx, authorizer)
		// An expired secret already failed the authorizer line, with its fix.
		if str(authorizer["suspension_reason"]) != "secret_expired" {
			p.secretFresh(authorizer)
		}
	}
	p.secretSet()
	if authorizer != nil {
		p.endpointEnabled(ctx, authorizer)
		if p.amount != nil {
			p.amountWithinLimits(authorizer)
		}
	}
	p.payeeTrusted(ctx)
	return p.ok
}

// 1. Both keys (and the ids the other steps read) are set, the keys different.
func (p *Preflight) keys() bool {
	var missing []string
	for _, name := range []string{"MANZA_CREATOR_API_KEY", "MANZA_AUTHORIZER_API_KEY", "MANZA_ACCOUNT_ID", "MANZA_BENEFICIARY_ID"} {
		if p.env(name) == "" {
			missing = append(missing, name)
		}
	}
	switch {
	case len(missing) > 0:
		p.fail(strings.Join(missing, ", ")+" not set", "set them in .env")
	case p.env("MANZA_CREATOR_API_KEY") == p.env("MANZA_AUTHORIZER_API_KEY"):
		p.fail("key A and key B are the same key",
			"use two different keys: the API refuses to let the key that created a transfer authorize it")
	default:
		if err := p.buildClients(); err != nil {
			p.fail("cannot build the clients: "+err.Error(), "check MANZA_BASE_URL")
			return false
		}
		p.pass("key A and key B are set and different")
		return true
	}
	return false
}

func (p *Preflight) buildClients() error {
	var err error
	baseURL := manza.WithBaseURL(p.env("MANZA_BASE_URL")) // empty: https://ma.manza.finance
	if p.creator, err = manza.New(manza.WithAPIKey(p.env("MANZA_CREATOR_API_KEY")), baseURL); err != nil {
		return err
	}
	p.authorizerClient, err = manza.New(manza.WithAPIKey(p.env("MANZA_AUTHORIZER_API_KEY")), baseURL)
	return err
}

// 2. Key A reaches the API.
func (p *Preflight) creatorReachesEntity(ctx context.Context) {
	resp, err := p.creator.Entity.Get(ctx)
	if err != nil {
		p.fail("key A cannot read the entity: "+describe(err), keyFix(err, "MANZA_CREATOR_API_KEY", "entity:read", ""))
		return
	}
	p.pass(fmt.Sprintf("key A reaches the API (entity %s)", inspect(resp.Body["name"])))
}

// 3. Key B reads the authorizer, which must be active.
func (p *Preflight) enrolledAuthorizer(ctx context.Context) map[string]any {
	resp, err := p.authorizerClient.TransferAuthorizer.Get(ctx)
	var apiErr *manza.Error
	switch {
	case errors.As(err, &apiErr) && apiErr.Kind == manza.KindNotFound:
		p.fail("no transfer authorizer is enrolled", "enrol your webhook endpoint in Settings → Developer → Transfer approval")
		return nil
	case err != nil:
		p.fail("key B cannot read the transfer authorizer: "+describe(err),
			keyFix(err, "MANZA_AUTHORIZER_API_KEY", "transfers:authorize", ""))
		return nil
	}
	authorizer := resp.Body
	if authorizer["status"] == "active" {
		p.pass("transfer authorizer is active")
		return authorizer
	}
	reason := str(authorizer["suspension_reason"])
	fix, ok := suspensionFixes[reason]
	if !ok {
		fix = "see suspension_reason in the transfer authorization docs"
	}
	p.fail(fmt.Sprintf("transfer authorizer is %v (%s)", authorizer["status"], reason), fix)
	return authorizer
}

func (p *Preflight) currencyMatches(ctx context.Context, authorizer map[string]any) {
	accountID := p.env("MANZA_ACCOUNT_ID")
	resp, err := p.creator.Accounts.Get(ctx, accountID)
	if err != nil {
		p.fail(fmt.Sprintf("key A cannot read account %q: %s", accountID, describe(err)),
			keyFix(err, "MANZA_CREATOR_API_KEY", "accounts:read", "MANZA_ACCOUNT_ID"))
		return
	}
	currency, enrolled := str(resp.Body["currency_code"]), str(authorizer["currency_code"])
	if currency == enrolled {
		p.pass(fmt.Sprintf("account currency %s is the enrolled currency", currency))
		return
	}
	p.fail(fmt.Sprintf("account currency %s is not the enrolled currency %s", currency, enrolled),
		fmt.Sprintf("pay from a %s account (MANZA_ACCOUNT_ID), or enrol %s", enrolled, currency))
}

func (p *Preflight) secretFresh(authorizer map[string]any) {
	raw := str(authorizer["secret_expires_at"])
	expiresAt, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		p.fail(fmt.Sprintf("secret_expires_at %q is not a time", raw), "check MANZA_BASE_URL points at the Manza API")
		return
	}
	days := int(math.Floor(expiresAt.Sub(p.now).Hours() / 24))
	if days >= secretMinDays {
		p.pass(fmt.Sprintf("signing secret expires in %d days", days))
		return
	}
	state := "expired"
	if days >= 0 {
		state = fmt.Sprintf("expires in %d days", days)
	}
	p.fail(fmt.Sprintf("signing secret %s (%s)", state, raw),
		"an approver should rotate the secret in Settings → Developer → Transfer approval; set the new one as "+
			"MANZA_AUTHORIZER_SECRET")
}

// 4. The signing secret is set.
func (p *Preflight) secretSet() {
	if p.env("MANZA_AUTHORIZER_SECRET") != "" {
		p.pass("MANZA_AUTHORIZER_SECRET is set")
		return
	}
	p.fail("MANZA_AUTHORIZER_SECRET is not set", "set it to the signing secret shown at enrolment or at the last rotation")
}

// 5. The enrolled webhook endpoint is enabled, on a URL that lasts.
func (p *Preflight) endpointEnabled(ctx context.Context, authorizer map[string]any) {
	endpointID := str(authorizer["webhook_endpoint_id"])
	resp, err := p.creator.WebhookEndpoints.Get(ctx, endpointID)
	var apiErr *manza.Error
	switch {
	case errors.As(err, &apiErr) && apiErr.Kind == manza.KindNotFound:
		p.fail(fmt.Sprintf("the enrolled webhook endpoint %s was deleted", endpointID),
			"create an endpoint at a stable URL and enrol it in Settings → Developer → Transfer approval")
		return
	case err != nil:
		p.fail("key A cannot read the enrolled webhook endpoint: "+describe(err),
			keyFix(err, "MANZA_CREATOR_API_KEY", "webhook_endpoints:read", ""))
		return
	}
	endpoint := resp.Body
	endpointURL := str(endpoint["url"])
	if endpoint["status"] == "active" {
		p.pass(fmt.Sprintf("webhook endpoint %s is enabled", endpointURL))
	} else {
		p.fail(fmt.Sprintf("webhook endpoint %s is %v", endpointURL, endpoint["status"]),
			fmt.Sprintf("enable it (WebhookEndpoints.Enable(ctx, %q)); if the authorizer stays suspended, "+
				"an approver resumes it in the app", str(endpoint["id"])))
	}
	parsed, err := url.Parse(endpointURL)
	if err != nil {
		return
	}
	for _, domain := range unstableTunnelHosts {
		if strings.HasSuffix(parsed.Hostname(), domain) {
			p.warn(fmt.Sprintf("webhook endpoint is on %s, whose URL changes when the tunnel restarts", domain),
				"use a reserved domain or a named tunnel, then enrol an endpoint at that URL")
			return
		}
	}
}

// 6a. The test amount fits both limits.
func (p *Preflight) amountWithinLimits(authorizer map[string]any) {
	perTransfer, ok1 := new(big.Rat).SetString(str(authorizer["per_transfer_limit"]))
	daily, ok2 := new(big.Rat).SetString(str(authorizer["daily_limit"]))
	total, ok3 := new(big.Rat).SetString(str(authorizer["daily_authorized_total"]))
	if !ok1 || !ok2 || !ok3 {
		p.fail("the authorizer's limits are not decimals", "check MANZA_BASE_URL points at the Manza API")
		return
	}
	room := new(big.Rat).Sub(daily, total)
	switch {
	case p.amount.Cmp(perTransfer) > 0:
		p.fail(fmt.Sprintf("%s is over the limit per transfer (%s)", formatAmount(p.amount), formatAmount(perTransfer)),
			fmt.Sprintf("send at most %s, or have an approver raise the limit", formatAmount(perTransfer)))
	case p.amount.Cmp(room) > 0:
		p.fail(fmt.Sprintf("%s does not fit the last 24 hours' room (%s left)", formatAmount(p.amount), formatAmount(room)),
			"wait for older authorizations to leave the 24-hour window, or have an approver raise the limit")
	default:
		p.pass(fmt.Sprintf("%s is within the limit per transfer and the 24-hour room", formatAmount(p.amount)))
	}
}

// 6b. The payee is trusted, or may be trusted on authorize.
func (p *Preflight) payeeTrusted(ctx context.Context) {
	beneficiaryID := p.env("MANZA_BENEFICIARY_ID")
	resp, err := p.creator.Beneficiaries.Get(ctx, beneficiaryID)
	if err != nil {
		p.fail(fmt.Sprintf("key A cannot read beneficiary %q: %s", beneficiaryID, describe(err)),
			keyFix(err, "MANZA_CREATOR_API_KEY", "beneficiaries:read", "MANZA_BENEFICIARY_ID"))
		return
	}
	accounts, _ := resp.Body["external_accounts"].([]any)
	var account map[string]any
	for _, a := range accounts {
		if candidate, ok := a.(map[string]any); ok && candidate["default"] == true {
			account = candidate
			break
		}
	}
	if account == nil {
		p.fail("beneficiary has no default bank account", "add one, or set a beneficiary that has one")
		return
	}
	if account["trusted"] == true {
		p.pass(fmt.Sprintf("payee %s is trusted", str(account["id"])))
		return
	}
	p.warn(fmt.Sprintf("payee %s is not trusted yet", str(account["id"])),
		"it qualifies only with \"Approve transfers to new payees\" on in Settings → Developer → Transfer approval "+
			"(not visible through the API), key A bound to an IP allowlist, and the amount within the limit per "+
			"transfer to a new payee, and only if its bank details were never trusted (revoked or edited "+
			"details always go to your approvers); otherwise have an approver trust it")
}

func describe(err error) string {
	var apiErr *manza.Error
	if !errors.As(err, &apiErr) {
		return err.Error()
	}
	detail := apiErr.Type
	if detail == "" {
		detail = strconv.Itoa(apiErr.Status)
	}
	return fmt.Sprintf("%s %s: %s", apiErr.Kind, detail, apiErr.Message)
}

func keyFix(err error, keyVar, scope, idVar string) string {
	var apiErr *manza.Error
	if !errors.As(err, &apiErr) {
		return "check MANZA_BASE_URL and the network, then rerun"
	}
	switch apiErr.Kind {
	case manza.KindAuthentication:
		return "check " + keyVar + ": the key is wrong or revoked"
	case manza.KindForbidden:
		return "give the key the " + scope + " scope, or add this machine's address to its IP allowlist"
	case manza.KindNotFound:
		if idVar == "" {
			idVar = "MANZA_BASE_URL"
		}
		return "check " + idVar
	}
	return "check MANZA_BASE_URL and the network, then rerun"
}

func formatAmount(r *big.Rat) string { return r.FloatString(2) }

func (p *Preflight) pass(message string) { fmt.Fprintf(p.out, "✓ %s\n", message) }

func (p *Preflight) fail(message, fix string) {
	p.ok = false
	fmt.Fprintf(p.out, "✗ %s: %s\n", message, fix)
}

func (p *Preflight) warn(message, note string) { fmt.Fprintf(p.out, "! %s: %s\n", message, note) }
