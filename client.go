// Package zazu is the Go SDK for the Zazu API.
//
// Response bodies are returned as-is from the API — snake_case keys in a
// map. The same shape ships across every Zazu SDK (Ruby, TypeScript, Python,
// Go, ...) so the cassette contract is one-to-one. Response.Decode maps a
// body onto the typed models in types.go when you want structs.
//
// Deprecated: this module is renamed. Use github.com/getmanza/manza-go
// (package manza) instead; zazu-go gets no further updates.
package zazu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// Version is the SDK version, sent in the User-Agent header.
	Version = "0.3.1"

	defaultBaseURL = "https://ma.manza.finance"
	defaultTimeout = 30 * time.Second
)

// Client is the SDK entry point. Resources hang off it as fields.
//
//	client, err := zazu.New(zazu.WithAPIKey("sk_live_..."))
//	page, err := client.Accounts.List(ctx, zazu.ListParams{})
type Client struct {
	apiKey     string
	baseURL    string
	apiVersion string
	httpClient *http.Client

	Accounts           *AccountsService
	Beneficiaries      *BeneficiariesService
	CheckoutSessions   *CheckoutSessionsService
	Customers          *CustomersService
	Entity             *EntityService
	Invoices           *InvoicesService
	PayeeTrustRequests *PayeeTrustRequestsService
	PaymentLinks       *PaymentLinksService
	TransferDrafts     *TransferDraftsService
	WebhookEndpoints   *WebhookEndpointsService
}

// Option configures a Client.
type Option func(*Client)

// WithAPIKey sets the API key (default: the ZAZU_API_KEY env var).
func WithAPIKey(key string) Option { return func(c *Client) { c.apiKey = key } }

// WithBaseURL sets the API base URL (default: ZAZU_BASE_URL or
// https://ma.manza.finance). South Africa is https://za.manza.finance.
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithAPIVersion pins the Zazu-Version request header (default: ZAZU_API_VERSION).
func WithAPIVersion(v string) Option { return func(c *Client) { c.apiVersion = v } }

// WithHTTPClient swaps the underlying *http.Client (default: 30s timeout).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// New builds a Client. An API key is required — pass WithAPIKey or set
// ZAZU_API_KEY.
func New(opts ...Option) (*Client, error) {
	c := &Client{
		apiKey:     os.Getenv("ZAZU_API_KEY"),
		baseURL:    os.Getenv("ZAZU_BASE_URL"),
		apiVersion: os.Getenv("ZAZU_API_VERSION"),
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.apiKey == "" {
		return nil, &ConfigurationError{Message: "missing API key: pass zazu.WithAPIKey or set ZAZU_API_KEY"}
	}
	if c.baseURL == "" {
		c.baseURL = defaultBaseURL
	}
	c.baseURL = strings.TrimRight(c.baseURL, "/")
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: defaultTimeout}
	}

	c.Accounts = &AccountsService{client: c}
	c.Beneficiaries = &BeneficiariesService{client: c}
	c.CheckoutSessions = &CheckoutSessionsService{client: c}
	c.Customers = &CustomersService{client: c}
	c.Entity = &EntityService{client: c}
	c.Invoices = &InvoicesService{client: c}
	c.PayeeTrustRequests = &PayeeTrustRequestsService{client: c}
	c.PaymentLinks = &PaymentLinksService{client: c}
	c.TransferDrafts = &TransferDraftsService{client: c}
	c.WebhookEndpoints = &WebhookEndpointsService{client: c}
	return c, nil
}

// Response is a successful (2xx) API response.
type Response struct {
	Status    int
	RequestID string
	Body      map[string]any
	Raw       []byte
}

// Decode unmarshals the raw response body into v, typically one of the
// models in types.go (TransferDraft, CheckoutSession, ...).
func (r *Response) Decode(v any) error {
	return json.Unmarshal(r.Raw, v)
}

// Request performs an HTTP request against the API. Non-2xx responses are
// returned as *Error. Body (when non-nil) is JSON-encoded.
func (c *Client) Request(ctx context.Context, method, path string, params url.Values, body any) (*Response, error) {
	u := c.baseURL + "/" + strings.TrimLeft(path, "/")
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("zazu: encode request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, fmt.Errorf("zazu: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("User-Agent", "zazu-go/"+Version)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiVersion != "" {
		req.Header.Set("Zazu-Version", c.apiVersion)
	}

	raw, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &ConnectionError{Message: err.Error()}
	}
	defer raw.Body.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(raw.Body); err != nil {
		return nil, &ConnectionError{Message: "read response body: " + err.Error()}
	}

	parsed := map[string]any{}
	if buf.Len() > 0 {
		// Non-JSON bodies stay raw; parsed stays empty.
		_ = json.Unmarshal(buf.Bytes(), &parsed)
	}

	if raw.StatusCode < 200 || raw.StatusCode >= 300 {
		return nil, newError(raw.StatusCode, raw.Header, parsed)
	}

	return &Response{
		Status:    raw.StatusCode,
		RequestID: raw.Header.Get("X-Request-Id"),
		Body:      parsed,
		Raw:       buf.Bytes(),
	}, nil
}

func (c *Client) get(ctx context.Context, path string, params url.Values) (*Response, error) {
	return c.Request(ctx, http.MethodGet, path, params, nil)
}

func (c *Client) post(ctx context.Context, path string, body any) (*Response, error) {
	return c.Request(ctx, http.MethodPost, path, nil, body)
}

func (c *Client) patch(ctx context.Context, path string, body any) (*Response, error) {
	return c.Request(ctx, http.MethodPatch, path, nil, body)
}

func (c *Client) delete(ctx context.Context, path string) (*Response, error) {
	return c.Request(ctx, http.MethodDelete, path, nil, nil)
}

func encodePath(segments ...string) string {
	escaped := make([]string, len(segments))
	for i, s := range segments {
		escaped[i] = url.PathEscape(s)
	}
	return strings.Join(escaped, "/")
}
