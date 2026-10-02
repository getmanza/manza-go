package zazu_test

// Reads VCR YAML cassettes (recorded by zazu-ruby) and serves them from an
// httptest.Server so identical interactions replay against this SDK. The
// contract is enforced cross-language: every SDK that consumes the cassette
// tarball must replay the exact request shape.
//
// Matching is method + path+query + semantic JSON body (Go's map marshaling
// sorts keys, so byte-equality against Ruby's insertion-ordered bodies would
// never hold). The transfer_drafts/authorize* cassettes match the body with
// `signature` removed: the recorded value is an HMAC over a real nonce under a
// real secret (scrubbed to <SIGNATURE>) that replay cannot reproduce.
//
// Pass ONE cassette per test when two share method + URI (authorize vs
// authorize_same_key, create vs create_duplicate): the server serves the first
// matching interaction.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// replayHost is the host the cassettes were recorded against.
const replayHost = "ma.manza.dev"

// Mirror of spec/support/fixture_ids.rb in zazu-ruby. The placeholders must
// exactly match what VCR scrubbed the real staging UUIDs to when the
// cassettes were recorded — otherwise the request URI won't match.
var fixtureIDs = map[string]string{
	"ZAZU_FIXTURE_ACCOUNT_ID":                  "fixture-account-id",
	"ZAZU_FIXTURE_TRANSACTION_ID":              "fixture-transaction-id",
	"ZAZU_FIXTURE_CUSTOMER_ID":                 "fixture-customer-id",
	"ZAZU_FIXTURE_DELETABLE_CUSTOMER_ID":       "fixture-deletable-customer-id",
	"ZAZU_FIXTURE_INVOICE_ID":                  "fixture-invoice-id",
	"ZAZU_FIXTURE_DELETABLE_INVOICE_ID":        "fixture-deletable-invoice-id",
	"ZAZU_FIXTURE_PAYMENT_LINK_ID":             "fixture-payment-link-id",
	"ZAZU_FIXTURE_CANCELLABLE_PAYMENT_LINK_ID": "fixture-cancellable-payment-link-id",
	"ZAZU_FIXTURE_WEBHOOK_ID":                  "fixture-webhook-id",
	"ZAZU_FIXTURE_ENABLED_WEBHOOK_ID":          "fixture-enabled-webhook-id",
	"ZAZU_FIXTURE_DISABLED_WEBHOOK_ID":         "fixture-disabled-webhook-id",
	"ZAZU_FIXTURE_DELETABLE_WEBHOOK_ID":        "fixture-deletable-webhook-id",
	"ZAZU_FIXTURE_CHECKOUT_SESSION_ID":         "fixture-checkout-session-id",
	"ZAZU_FIXTURE_BENEFICIARY_ID":              "fixture-beneficiary-id",
	"ZAZU_FIXTURE_TRANSFER_DRAFT_ID":           "fixture-transfer-draft-id",
	// The discovered beneficiary's default bank account, marked a trusted
	// payee once by hand so machine-authorized drafts can target it.
	"ZAZU_FIXTURE_TRUSTED_EXTERNAL_ACCOUNT_ID": "fixture-trusted-external-account-id",
	"ZAZU_FIXTURE_CREATED_BENEFICIARY_ID":      "fixture-created-beneficiary-id",
	"ZAZU_FIXTURE_EXTERNAL_ACCOUNT_ID":         "fixture-external-account-id",
	// Not an ID, but fresh per seed (bank account numbers are unique per
	// entity) and scrubbed the same way.
	"ZAZU_FIXTURE_NEW_ACCOUNT_NUMBER":     "fixture-new-account-number",
	"ZAZU_FIXTURE_PAYEE_TRUST_REQUEST_ID": "fixture-payee-trust-request-id",
	// client_reference is unique per entity, so these are fresh per seed too.
	"ZAZU_FIXTURE_CLIENT_REFERENCE":               "fixture-client-reference",
	"ZAZU_FIXTURE_AUTHORIZABLE_CLIENT_REFERENCE":  "fixture-authorizable-client-reference",
	"ZAZU_FIXTURE_AUTHORIZABLE_DRAFT_ID":          "fixture-authorizable-draft-id",
	"ZAZU_FIXTURE_DECLINABLE_DRAFT_ID":            "fixture-declinable-draft-id",
	"ZAZU_FIXTURE_BAD_SIGNATURE_DRAFT_ID":         "fixture-bad-signature-draft-id",
	"ZAZU_FIXTURE_AUTHORIZABLE_AUTHORIZATION_ID":  "fixture-authorizable-authorization-id",
	"ZAZU_FIXTURE_DECLINABLE_AUTHORIZATION_ID":    "fixture-declinable-authorization-id",
	"ZAZU_FIXTURE_BAD_SIGNATURE_AUTHORIZATION_ID": "fixture-bad-signature-authorization-id",
	// One-time nonce from the authorization webhook. Only needed while
	// recording; it never appears in a request or response.
	"ZAZU_FIXTURE_AUTHORIZABLE_NONCE": "fixture-authorizable-nonce",
}

func fixtureID(t *testing.T, envVar string) string {
	t.Helper()
	placeholder, ok := fixtureIDs[envVar]
	if !ok {
		t.Fatalf("unknown fixture env var %q — add it to fixtureIDs", envVar)
	}
	return placeholder
}

// cassetteBody handles VCR's body encodings. Ruby's Psych writes non-UTF-8
// bodies as base64 with the PRIMARY `!binary` tag (not the canonical
// `!!binary`), which yaml.v3 leaves verbatim — decode it ourselves.
type cassetteBody struct {
	String string `yaml:"string"`
}

func (b *cassetteBody) UnmarshalYAML(node *yaml.Node) error {
	var raw struct {
		String yaml.Node `yaml:"string"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}
	value := raw.String.Value
	if raw.String.Tag == "!binary" || raw.String.Tag == "!!binary" {
		decoded, err := base64.StdEncoding.DecodeString(compactWhitespace(value))
		if err != nil {
			return fmt.Errorf("decode !binary cassette body: %w", err)
		}
		value = string(decoded)
	}
	b.String = value
	return nil
}

func compactWhitespace(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\n', '\r', '\t':
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}

type cassetteInteraction struct {
	ignoreSignature bool // set by the loader, not read from YAML

	Request struct {
		Method string       `yaml:"method"`
		URI    string       `yaml:"uri"`
		Body   cassetteBody `yaml:"body"`
	} `yaml:"request"`
	Response struct {
		Status struct {
			Code int `yaml:"code"`
		} `yaml:"status"`
		Body cassetteBody `yaml:"body"`
	} `yaml:"response"`
}

type cassette struct {
	HTTPInteractions []cassetteInteraction `yaml:"http_interactions"`
}

// startReplayServer loads the named cassettes (e.g. "payment_links/list")
// and serves their interactions. Unmatched requests fail the test.
func startReplayServer(t *testing.T, names ...string) *httptest.Server {
	t.Helper()

	var interactions []cassetteInteraction
	for _, name := range names {
		path := filepath.Join("testdata", "cassettes", name+".yml")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read cassette %s: %v (run scripts/fetch-cassettes.sh first)", path, err)
		}
		var c cassette
		if err := yaml.Unmarshal(raw, &c); err != nil {
			t.Fatalf("parse cassette %s: %v", path, err)
		}
		for _, interaction := range c.HTTPInteractions {
			recorded, err := url.Parse(interaction.Request.URI)
			if err != nil || recorded.Host != replayHost {
				t.Fatalf("cassette %s: expected a %s request URI, got %q", name, replayHost, interaction.Request.URI)
			}
			interaction.ignoreSignature = strings.HasPrefix(name, "transfer_drafts/authorize")
			interactions = append(interactions, interaction)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		for _, interaction := range interactions {
			if matches(interaction, r, body) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(interaction.Response.Status.Code)
				fmt.Fprint(w, interaction.Response.Body.String)
				return
			}
		}
		t.Errorf("no cassette interaction matches %s %s (body %q)", r.Method, r.URL.RequestURI(), body)
		w.WriteHeader(http.StatusNotImplemented)
	}))
	t.Cleanup(server.Close)
	return server
}

func matches(interaction cassetteInteraction, r *http.Request, body []byte) bool {
	if interaction.Request.Method != "" && !equalFold(interaction.Request.Method, r.Method) {
		return false
	}

	recorded, err := url.Parse(interaction.Request.URI)
	if err != nil {
		return false
	}
	if recorded.Path != r.URL.Path {
		return false
	}
	if !reflect.DeepEqual(recorded.Query(), r.URL.Query()) {
		return false
	}

	if interaction.ignoreSignature {
		return jsonEqualWithoutSignature(interaction.Request.Body.String, string(body))
	}
	return jsonEqual(interaction.Request.Body.String, string(body))
}

// jsonEqualWithoutSignature compares two JSON object bodies ignoring their
// `signature` key. Anything that is not a JSON object never matches.
func jsonEqualWithoutSignature(recorded, actual string) bool {
	var a, b map[string]any
	if json.Unmarshal([]byte(recorded), &a) != nil || json.Unmarshal([]byte(actual), &b) != nil {
		return false
	}
	delete(a, "signature")
	delete(b, "signature")
	return reflect.DeepEqual(a, b)
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'a' <= ca && ca <= 'z' {
			ca -= 'a' - 'A'
		}
		if 'a' <= cb && cb <= 'z' {
			cb -= 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// jsonEqual compares two bodies semantically when both parse as JSON, and
// byte-for-byte otherwise (empty matches empty).
func jsonEqual(recorded, actual string) bool {
	if recorded == actual {
		return true
	}
	var a, b any
	if json.Unmarshal([]byte(recorded), &a) != nil || json.Unmarshal([]byte(actual), &b) != nil {
		return false
	}
	return reflect.DeepEqual(a, b)
}
