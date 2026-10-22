package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/vtech-com/seli-api-sdk/internal/version"
)

const (
	// requestTimeout bounds one whole request, dial to last body byte. Without
	// it a hung server hangs the CLI forever.
	requestTimeout = 30 * time.Second

	// maxResponseBytes caps what the client will read from one response. The
	// largest legitimate answer is a page of 100 records; anything past this is
	// a misbehaving server, not data, and reading it unbounded would let one
	// reach the CLI's memory.
	maxResponseBytes = 16 << 20
)

// Response wraps the raw HTTP response.
type Response struct {
	StatusCode int
	Body       []byte
	// RequestID is the server's X-Request-Id if it sent one, else the id the
	// client generated for this request.
	RequestID string
}

// Client is an HTTP client for the Seli Public API.
type Client struct {
	http    *http.Client
	baseURL string
	apiKey  string
	// verboseW, when non-nil, receives a redacted trace of every request and
	// response. Enabled via EnableVerbose (the --verbose flag).
	verboseW io.Writer
}

// EnableVerbose turns on request/response tracing to w. The Authorization
// header is always redacted. Pass nil to disable.
func (c *Client) EnableVerbose(w io.Writer) {
	c.verboseW = w
}

// NewClient creates a Client. baseURL is the host only, e.g. https://<host>: no
// path beyond an optional "/", no query, fragment or credentials (the /api
// prefix and each endpoint's path are added per request). It must use HTTPS
// unless its host is localhost or 127.0.0.1. Every failure here is diagnosed
// locally: nothing has been sent.
func NewClient(baseURL, apiKey string) (*Client, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, &APIError{
			Code:       ErrAPIURLMissing,
			Message:    "no API base URL configured: pass --api-url, set SELI_API_URL, or run `seli config set api_url <url>`; this build bakes in no default (see `seli version`)",
			HTTPStatus: 400,
		}
	}

	// None of the messages below repeat the value: a base URL pasted wrongly can
	// carry a password or a token, and these messages are printed and pasted.
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return nil, &APIError{
			Code:       ErrValidation,
			Message:    "invalid API base URL: want the host only, e.g. https://<host>",
			HTTPStatus: 400,
		}
	}
	// The base URL is the host. The /api prefix and each endpoint's path belong to
	// the code that knows the endpoint, so anything else here would land in the
	// middle of every request URL (https://h/x?y=1 + /api/health).
	var extras []string
	if u.User != nil {
		extras = append(extras, "credentials")
	}
	if u.Path != "" && u.Path != "/" {
		extras = append(extras, "a path")
	}
	if strings.Contains(baseURL, "?") {
		extras = append(extras, "a query string")
	}
	if strings.Contains(baseURL, "#") {
		extras = append(extras, "a fragment")
	}
	if len(extras) > 0 {
		return nil, &APIError{
			Code:       ErrValidation,
			Message:    "the API base URL must be host only, e.g. https://<host>, but it has " + strings.Join(extras, " and "),
			HTTPStatus: 400,
		}
	}
	host := u.Hostname()
	isLoopback := host == "localhost" || host == "127.0.0.1"
	secure := u.Scheme == "https" || (u.Scheme == "http" && isLoopback)
	if !secure {
		return nil, &APIError{
			Code:       ErrInsecureBaseURL,
			Message:    fmt.Sprintf("refusing to send the API key to %q: the base URL must use https (got scheme %q); plain http is allowed only for localhost and 127.0.0.1", host, u.Scheme),
			HTTPStatus: 400,
		}
	}

	// A key with whitespace or control characters is not a key, and as a header
	// value it would fail inside the transport as a "network" error.
	if !ValidKeyChars(apiKey) {
		return nil, &APIError{
			Code:       ErrValidation,
			Message:    "the API key contains whitespace or control characters; re-run `seli auth login --key <ssk_...>` with the key exactly as issued",
			HTTPStatus: 400,
		}
	}

	return &Client{
		http:    &http.Client{Timeout: requestTimeout, CheckRedirect: refuseRedirect},
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
	}, nil
}

// refuseRedirect makes the HTTP client hand a 3xx back instead of following it.
//
// Go forwards the Authorization header on a redirect to the same host whatever
// the scheme or port, so following one would send the API key in clear text on
// an https -> http redirect, or to another listener on the same machine. The
// API answers with data or an error, never a redirect; send() turns one into
// UNEXPECTED_REDIRECT.
func refuseRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

// ValidKeyChars reports whether key is free of whitespace and control
// characters, which no issued key contains and no header value may carry.
func ValidKeyChars(key string) bool {
	for _, r := range key {
		if r <= ' ' || r == 0x7f {
			return false
		}
	}
	return true
}

// RedactedAuthorization returns the Authorization header value with the key redacted.
func RedactedAuthorization(apiKey string) string {
	if len(apiKey) <= 8 {
		return "Bearer [REDACTED]"
	}
	return "Bearer " + apiKey[:4] + "...[REDACTED]"
}

// Do executes an HTTP request against path (e.g. "/api/health", query string
// included). body is JSON-encoded when non-nil. tenant is the X-Tenant-Code
// value; when empty the header is omitted entirely.
func (c *Client) Do(ctx context.Context, method, path string, body any, tenant string) (*Response, error) {
	return c.DoWithHeaders(ctx, method, path, body, tenant, nil)
}

// DoWithHeaders is Do plus the per-request headers an endpoint's contract asks
// for beyond tenancy.
//
// The caller's headers are applied first and the client's own (Authorization,
// User-Agent, X-Request-Id, X-Tenant-Code) second, so a caller cannot
// accidentally overwrite the credential or the tenant it is talking as.
func (c *Client) DoWithHeaders(ctx context.Context, method, path string, body any, tenant string, headers map[string]string) (*Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.send(req, headers, tenant)
}

// send applies the client's own headers, performs the request, and maps a
// 4xx/5xx to an APIError.
//
// Every method that talks to the API funnels through here, so the credential,
// the tenant header, verbose tracing, the timeout and the error envelope behave
// identically for every endpoint. The caller's headers are applied first and
// the client's second: a per-request header can never shadow the key.
func (c *Client) send(req *http.Request, headers map[string]string, tenant string) (*Response, error) {
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	requestID := newRequestID()
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("User-Agent", version.UserAgent())
	req.Header.Set("X-Request-Id", requestID)
	if tenant != "" {
		req.Header.Set("X-Tenant-Code", tenant)
	}

	c.traceRequest(req)

	resp, err := c.http.Do(req)
	if err != nil {
		c.tracef("< error: %v\n", err)
		return nil, &APIError{Code: ErrNetwork, Message: err.Error(), RequestID: requestID, cause: err}
	}
	defer func() { _ = resp.Body.Close() }()

	// One byte past the cap tells "exactly at the limit" from "over it".
	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		c.tracef("< error: %v\n", err)
		return nil, &APIError{Code: ErrNetwork, Message: "read response body: " + err.Error(), RequestID: requestID, cause: err}
	}
	if len(rawBody) > maxResponseBytes {
		c.tracef("< HTTP %d (body over %d bytes, not read)\n", resp.StatusCode, maxResponseBytes)
		return nil, &APIError{
			Code:       ErrNetwork,
			Message:    fmt.Sprintf("response body exceeds %d bytes", maxResponseBytes),
			RequestID:  requestID,
			HTTPStatus: resp.StatusCode,
		}
	}

	// The server's id wins if it ever sends one; today it does not, so the id
	// the client generated is the only handle that correlates a failure with
	// the server's logs.
	if serverID := resp.Header.Get("X-Request-Id"); serverID != "" {
		requestID = serverID
	}
	r := &Response{StatusCode: resp.StatusCode, Body: rawBody, RequestID: requestID}

	c.tracef("< HTTP %d\n", resp.StatusCode)
	if len(rawBody) > 0 {
		c.tracef("< %s\n", rawBody)
	}

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return r, unexpectedRedirect(resp, rawBody, requestID)
	}
	if resp.StatusCode >= 400 {
		return r, parseAPIError(rawBody, resp.StatusCode, requestID)
	}
	return r, nil
}

// unexpectedRedirect is the error for a 3xx, which the client never follows
// (see refuseRedirect). It names where the server pointed, with any credentials
// in that address redacted: the message is printed and pasted into tickets.
func unexpectedRedirect(resp *http.Response, body []byte, requestID string) *APIError {
	where := "nowhere (no Location header)"
	if loc := resp.Header.Get("Location"); loc != "" {
		where = "an unparseable Location"
		if u, err := resp.Request.URL.Parse(loc); err == nil {
			where = u.Redacted()
		}
		if len(where) > 200 {
			where = where[:200] + "..."
		}
	}
	return &APIError{
		Code: ErrUnexpectedRedirect,
		Message: fmt.Sprintf("the server answered %d, redirecting to %s. The CLI never follows redirects, because the API key would travel with them; check that the base URL is the API's current address",
			resp.StatusCode, where),
		RequestID:  requestID,
		HTTPStatus: resp.StatusCode,
		RawBody:    body,
	}
}

// traceRequest writes the request line and the headers that go on the wire to
// the verbose writer. It reads the headers off the request itself rather than
// from what the caller passed, so nothing the transport will send can be
// missing from the trace, and Authorization is redacted by name whoever set it.
func (c *Client) traceRequest(req *http.Request) {
	if c.verboseW == nil {
		return
	}
	// Redacted, not String: a base URL may carry credentials in its userinfo,
	// and a trace is something people paste into tickets.
	c.tracef("> %s %s\n", req.Method, req.URL.Redacted())

	names := make([]string, 0, len(req.Header))
	for name := range req.Header {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if http.CanonicalHeaderKey(name) == "Authorization" {
			c.tracef("> Authorization: %s\n", RedactedAuthorization(c.apiKey))
			continue
		}
		for _, value := range req.Header[name] {
			c.tracef("> %s: %s\n", name, value)
		}
	}
}

// tracef writes one line of the verbose trace; it is a no-op unless
// EnableVerbose was called.
func (c *Client) tracef(format string, args ...any) {
	if c.verboseW == nil {
		return
	}
	_, _ = fmt.Fprintf(c.verboseW, format, args...)
}

// parseAPIError attempts to extract the structured error from the response body.
func parseAPIError(body []byte, status int, requestID string) *APIError {
	var envelope struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error.Code != "" {
		rid := envelope.Error.RequestID
		if rid == "" {
			rid = requestID
		}
		return &APIError{
			Code:       envelope.Error.Code,
			Message:    envelope.Error.Message,
			RequestID:  rid,
			HTTPStatus: status,
			RawBody:    body,
		}
	}
	return &APIError{
		Code:       fmt.Sprintf("HTTP_%d", status),
		Message:    fmt.Sprintf("unexpected response (status %d)", status),
		RequestID:  requestID,
		HTTPStatus: status,
		RawBody:    body,
	}
}

// newRequestID generates a client-side UUIDv4 sent as X-Request-Id. The server
// does not echo it today, so it is the handle a failure is reported under. No
// external dependency needed.
func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
