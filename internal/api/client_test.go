package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

// userAgentPattern matches: seli-api-sdk/<version> (<os>; <arch>)
var userAgentPattern = regexp.MustCompile(`^seli-api-sdk/.+ \(.+; .+\)$`)

// uuidV4Pattern matches the client-generated request id.
var uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

const testKey = "ssk_testkey123456789"

// newTestClient starts a plain-HTTP server on 127.0.0.1 — the one non-HTTPS
// host the client accepts — and returns a client pointed at it.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL, testKey)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func asAPIError(t *testing.T, err error) *APIError {
	t.Helper()
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	return apiErr
}

func TestNewClient_RejectsHTTPNonLoopback(t *testing.T) {
	_, err := NewClient("http://api.example.test", testKey)
	if err == nil {
		t.Fatal("expected error for http:// non-loopback URL")
	}
	apiErr := asAPIError(t, err)
	if apiErr.Code != ErrInsecureBaseURL {
		t.Errorf("Code = %q, want %q", apiErr.Code, ErrInsecureBaseURL)
	}
	if got := ExitCodeFor(err); got != 5 {
		t.Errorf("exit code = %d, want 5", got)
	}
}

func TestNewClient_AllowsHTTPLoopback(t *testing.T) {
	for _, u := range []string{"http://localhost:3999", "http://127.0.0.1:3999"} {
		if _, err := NewClient(u, testKey); err != nil {
			t.Errorf("NewClient(%q): %v", u, err)
		}
	}
}

func TestNewClient_AllowsHTTPS(t *testing.T) {
	if _, err := NewClient("https://api.example.test", testKey); err != nil {
		t.Fatalf("unexpected error for https URL: %v", err)
	}
}

// Loopback exempts a host from the https rule, not a scheme from the rest of
// the checks: ftp://localhost is no more a place to send a key than http://x.
func TestNewClient_RejectsOtherSchemesEvenOnLoopback(t *testing.T) {
	for _, u := range []string{"ftp://localhost", "file://127.0.0.1", "ws://localhost:80"} {
		_, err := NewClient(u, testKey)
		if err == nil {
			t.Errorf("NewClient(%q) succeeded", u)
			continue
		}
		if code := asAPIError(t, err).Code; code != ErrInsecureBaseURL {
			t.Errorf("NewClient(%q) code = %q, want %q", u, code, ErrInsecureBaseURL)
		}
	}
}

// A host that merely starts with "localhost" is not localhost.
func TestNewClient_RejectsLookalikeLoopbackHosts(t *testing.T) {
	for _, u := range []string{"http://localhost.example.test", "http://127.0.0.1.example.test", "http://notlocalhost"} {
		if _, err := NewClient(u, testKey); err == nil {
			t.Errorf("NewClient(%q) succeeded", u)
		}
	}
}

func TestNewClient_EmptyURLIsAPIURLMissing(t *testing.T) {
	for _, u := range []string{"", "   "} {
		_, err := NewClient(u, testKey)
		if err == nil {
			t.Fatalf("NewClient(%q) succeeded", u)
		}
		apiErr := asAPIError(t, err)
		if apiErr.Code != ErrAPIURLMissing {
			t.Errorf("Code = %q, want %q", apiErr.Code, ErrAPIURLMissing)
		}
		if !strings.Contains(apiErr.Message, "SELI_API_URL") {
			t.Errorf("message does not name SELI_API_URL: %q", apiErr.Message)
		}
		if got := ExitCodeFor(err); got != 5 {
			t.Errorf("exit code = %d, want 5", got)
		}
	}
}

func TestNewClient_RejectsMalformedURL(t *testing.T) {
	for _, u := range []string{"api.example.test", "https://", "http://%zz", "://x"} {
		_, err := NewClient(u, testKey)
		if err == nil {
			t.Errorf("NewClient(%q) succeeded", u)
			continue
		}
		if got := ExitCodeFor(err); got != 5 {
			t.Errorf("NewClient(%q) exit code = %d, want 5", u, got)
		}
	}
}

// The base URL is the host; the /api prefix and every path belong to the code
// that knows the endpoint. A path, query or fragment in the base would end up
// in the middle of every request URL (https://h/x?y=1 + /api/health =
// https://h/x?y=1/api/health), and userinfo is a credential in a URL.
func TestNewClient_RejectsABaseURLThatIsNotHostOnly(t *testing.T) {
	cases := map[string]string{
		"path":                  "https://api.example.test/x",
		"the api prefix":        "https://api.example.test/api",
		"a trailing path":       "https://api.example.test/api/",
		"repeated slashes":      "https://api.example.test//",
		"query":                 "https://api.example.test?y=SECRETQUERY",
		"path and query":        "https://api.example.test/x?y=SECRETQUERY",
		"empty query":           "https://api.example.test?",
		"fragment":              "https://api.example.test#SECRETFRAG",
		"empty fragment":        "https://api.example.test/#",
		"userinfo":              "https://someone:SECRETPASS@api.example.test",
		"username only":         "https://SECRETUSER@api.example.test",
		"loopback with a path":  "http://127.0.0.1:8080/api",
		"loopback with a query": "http://localhost:8080?x=SECRETQUERY",
	}
	for name, u := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewClient(u, testKey)
			if err == nil {
				t.Fatalf("NewClient(%q) succeeded", u)
			}
			apiErr := asAPIError(t, err)
			if apiErr.Code != ErrValidation {
				t.Errorf("Code = %q, want %q", apiErr.Code, ErrValidation)
			}
			if got := ExitCodeFor(err); got != 5 {
				t.Errorf("exit code = %d, want 5", got)
			}
			if !strings.Contains(apiErr.Message, "host only") || !strings.Contains(apiErr.Message, "https://<host>") {
				t.Errorf("message does not say what is wanted: %q", apiErr.Message)
			}
			// The value may carry a credential; the message must not repeat it.
			if strings.Contains(apiErr.Message, "SECRET") {
				t.Errorf("message repeats part of the URL: %q", apiErr.Message)
			}
		})
	}
}

func TestNewClient_AcceptsAHostOnlyBaseURL(t *testing.T) {
	for _, u := range []string{
		"https://api.example.test", "https://api.example.test/", "https://api.example.test:8443",
		"https://api.example.test:8443/", "http://localhost:3999/", "http://127.0.0.1:3999",
	} {
		if _, err := NewClient(u, testKey); err != nil {
			t.Errorf("NewClient(%q): %v", u, err)
		}
	}
}

// A malformed URL is reported without being echoed: it may have a password in it.
func TestNewClient_MalformedURLErrorDoesNotEchoTheValue(t *testing.T) {
	_, err := NewClient("https://someone:SECRETPASS@api.example.test/%zz", testKey)
	if err == nil {
		t.Fatal("expected an error")
	}
	if apiErr := asAPIError(t, err); strings.Contains(apiErr.Message, "SECRETPASS") || strings.Contains(apiErr.Message, "someone") {
		t.Errorf("message repeats the credentials: %q", apiErr.Message)
	}
}

func TestNewClient_RejectsKeyWithWhitespace(t *testing.T) {
	for _, key := range []string{"ssk_a b", "ssk_a\nb", "ssk_a\tb", "ssk_a\x7fb"} {
		_, err := NewClient("https://api.example.test", key)
		if err == nil {
			t.Errorf("NewClient accepted key %q", key)
			continue
		}
		apiErr := asAPIError(t, err)
		if strings.Contains(apiErr.Message, key) {
			t.Errorf("message repeats the key: %q", apiErr.Message)
		}
		if got := ExitCodeFor(err); got != 5 {
			t.Errorf("exit code = %d, want 5", got)
		}
	}
}

func TestNewClient_SetsTimeout(t *testing.T) {
	c, err := NewClient("https://api.example.test", testKey)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.http.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", c.http.Timeout)
	}
}

func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL+"/", testKey)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Do(context.Background(), http.MethodGet, "/api/health", nil, ""); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if gotPath != "/api/health" {
		t.Errorf("path = %q, want /api/health", gotPath)
	}
}

// A hung server must not hang the CLI. The real timeout is 30s; shrink it for
// the test, after checking above that NewClient really sets 30s.
func TestDo_TimesOut(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		case <-time.After(10 * time.Second):
		}
	}))
	defer srv.Close()
	defer close(release)

	c, err := NewClient(srv.URL, testKey)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.http.Timeout = 50 * time.Millisecond

	start := time.Now()
	_, err = c.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme")
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("returned after %v; the timeout did not fire", elapsed)
	}

	apiErr := asAPIError(t, err)
	if apiErr.Code != ErrNetwork {
		t.Errorf("Code = %q, want %q", apiErr.Code, ErrNetwork)
	}
	if !uuidV4Pattern.MatchString(apiErr.RequestID) {
		t.Errorf("RequestID = %q, want the client-generated id", apiErr.RequestID)
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Errorf("the timeout is not reachable through errors.As(net.Error): %v", err)
	}
	if got := ExitCodeFor(err); got != 6 {
		t.Errorf("exit code = %d, want 6", got)
	}
}

func TestDo_CancelledContextIsNetworkError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.Do(ctx, http.MethodGet, "/api/health", nil, "acme")
	if err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
	if got := ExitCodeFor(err); got != 6 {
		t.Errorf("exit code = %d, want 6", got)
	}
}

func TestDo_RequestHeaders(t *testing.T) {
	var got http.Header
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte(`{}`))
	})

	if _, err := c.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme"); err != nil {
		t.Fatalf("Do: %v", err)
	}

	if a := got.Get("Authorization"); a != "Bearer "+testKey {
		t.Errorf("Authorization = %q, want %q", a, "Bearer "+testKey)
	}
	if got.Get("X-Tenant-Code") != "acme" {
		t.Errorf("X-Tenant-Code = %q, want acme", got.Get("X-Tenant-Code"))
	}
	if ua := got.Get("User-Agent"); !userAgentPattern.MatchString(ua) {
		t.Errorf("User-Agent = %q, does not match expected pattern", ua)
	}
	if rid := got.Get("X-Request-Id"); !uuidV4Pattern.MatchString(rid) {
		t.Errorf("X-Request-Id = %q is not a UUIDv4", rid)
	}
}

func TestDo_OmitsTenantHeaderWhenEmpty(t *testing.T) {
	var sawHeader bool
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, sawHeader = r.Header["X-Tenant-Code"]
		_, _ = w.Write([]byte(`{}`))
	})

	if _, err := c.Do(context.Background(), http.MethodGet, "/api/health", nil, ""); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if sawHeader {
		t.Error("X-Tenant-Code header should be omitted, not sent empty")
	}
}

func TestDo_EachRequestGetsItsOwnID(t *testing.T) {
	seen := map[string]bool{}
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen[r.Header.Get("X-Request-Id")] = true
		_, _ = w.Write([]byte(`{}`))
	})
	for i := 0; i < 3; i++ {
		if _, err := c.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme"); err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	if len(seen) != 3 {
		t.Errorf("3 requests produced %d distinct ids", len(seen))
	}
}

func TestDo_SendsQueryAndPath(t *testing.T) {
	var gotPath, gotRawQuery string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotRawQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte(`{}`))
	})

	if _, err := c.Do(context.Background(), http.MethodGet, "/api/members?page=2", nil, "acme"); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if gotPath != "/api/members" || gotRawQuery != "page=2" {
		t.Errorf("got %q ? %q", gotPath, gotRawQuery)
	}
}

func TestDo_JSONBody(t *testing.T) {
	var gotCT string
	var gotBody []byte
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	})

	resp, err := c.Do(context.Background(), http.MethodPost, "/api/x", map[string]string{"a": "b"}, "acme")
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("StatusCode = %d, want 201", resp.StatusCode)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if string(gotBody) != `{"a":"b"}` {
		t.Errorf("body = %q", gotBody)
	}
}

// The credential and the tenant are the client's to set. A header a caller
// passes can add to the request but never replace either.
func TestDoWithHeaders_ClientHeadersWin(t *testing.T) {
	var got http.Header
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte(`{}`))
	})

	_, err := c.DoWithHeaders(context.Background(), http.MethodGet, "/api/health", nil, "acme", map[string]string{
		"Authorization": "Bearer attacker-key",
		"X-Tenant-Code": "other-tenant",
		"User-Agent":    "spoofed/1.0",
		"X-Request-Id":  "spoofed-id",
		"X-Custom":      "kept",
	})
	if err != nil {
		t.Fatalf("DoWithHeaders: %v", err)
	}

	if a := got.Get("Authorization"); a != "Bearer "+testKey {
		t.Errorf("Authorization = %q: a caller header overwrote the credential", a)
	}
	if got.Get("X-Tenant-Code") != "acme" {
		t.Errorf("X-Tenant-Code = %q: a caller header overwrote the tenant", got.Get("X-Tenant-Code"))
	}
	if !userAgentPattern.MatchString(got.Get("User-Agent")) {
		t.Errorf("User-Agent = %q", got.Get("User-Agent"))
	}
	if !uuidV4Pattern.MatchString(got.Get("X-Request-Id")) {
		t.Errorf("X-Request-Id = %q", got.Get("X-Request-Id"))
	}
	if got.Get("X-Custom") != "kept" {
		t.Errorf("X-Custom = %q, want the caller's header to be sent", got.Get("X-Custom"))
	}
}

func TestDo_ParsesErrorEnvelope(t *testing.T) {
	const body = `{"error":{"code":"E0003_UNAUTHORIZED","message":"Unauthorized"}}`
	var sentID string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		sentID = r.Header.Get("X-Request-Id")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(body))
	})

	_, err := c.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme")
	apiErr := asAPIError(t, err)

	if apiErr.Code != "E0003_UNAUTHORIZED" || apiErr.Message != "Unauthorized" {
		t.Errorf("code/message = %q / %q", apiErr.Code, apiErr.Message)
	}
	if apiErr.HTTPStatus != http.StatusUnauthorized {
		t.Errorf("HTTPStatus = %d, want 401", apiErr.HTTPStatus)
	}
	if string(apiErr.RawBody) != body {
		t.Errorf("RawBody = %q, want the verbatim server body", apiErr.RawBody)
	}
	// The server echoes no request id and its error body carries none, so the
	// id the client generated is the only handle on this failure.
	if apiErr.RequestID == "" || apiErr.RequestID != sentID {
		t.Errorf("RequestID = %q, want the id sent on the wire (%q)", apiErr.RequestID, sentID)
	}
	if apiErr.LocalDiagnosis {
		t.Error("a server answer is not a local diagnosis")
	}
}

func TestDo_PrefersTheServersRequestID(t *testing.T) {
	t.Run("header", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Request-Id", "srv-header-1")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"E0404_NOT_FOUND","message":"nope"}}`))
		})
		_, err := c.Do(context.Background(), http.MethodGet, "/api/members/x", nil, "acme")
		if got := asAPIError(t, err).RequestID; got != "srv-header-1" {
			t.Errorf("RequestID = %q, want the server's header value", got)
		}
	})

	t.Run("success response", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Request-Id", "srv-header-2")
			_, _ = w.Write([]byte(`{}`))
		})
		resp, err := c.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme")
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		if resp.RequestID != "srv-header-2" {
			t.Errorf("RequestID = %q, want the server's header value", resp.RequestID)
		}
	})

	t.Run("error body beats header", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Request-Id", "srv-header-3")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"code":"E0000_UNKNOWN","message":"boom","request_id":"srv-body-3"}}`))
		})
		_, err := c.Do(context.Background(), http.MethodGet, "/api/members", nil, "acme")
		if got := asAPIError(t, err).RequestID; got != "srv-body-3" {
			t.Errorf("RequestID = %q, want the id in the error body", got)
		}
	})

	t.Run("success response without header falls back to the client id", func(t *testing.T) {
		var sentID string
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			sentID = r.Header.Get("X-Request-Id")
			_, _ = w.Write([]byte(`{}`))
		})
		resp, err := c.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme")
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		if resp.RequestID != sentID || sentID == "" {
			t.Errorf("RequestID = %q, want the client id %q", resp.RequestID, sentID)
		}
	})
}

// An edge or proxy answers in HTML or plain text. The code and message stay
// ours; the body the server really sent is kept verbatim for the caller.
func TestDo_NonEnvelopedErrorKeepsTheRawBody(t *testing.T) {
	const body = "<html><body>Bad Gateway</body></html>"
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(body))
	})

	_, err := c.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme")
	apiErr := asAPIError(t, err)
	if apiErr.Code != "HTTP_502" {
		t.Errorf("Code = %q, want HTTP_502", apiErr.Code)
	}
	if apiErr.HTTPStatus != http.StatusBadGateway {
		t.Errorf("HTTPStatus = %d, want 502", apiErr.HTTPStatus)
	}
	if string(apiErr.RawBody) != body {
		t.Errorf("RawBody = %q, want the verbatim body", apiErr.RawBody)
	}
	if apiErr.RequestID == "" {
		t.Error("RequestID is empty")
	}
}

// Go forwards the Authorization header on a redirect to the same host whatever
// the scheme or port, so following one would hand the API key to a plain-http
// listener (https -> http) or to another service on the same machine. The client
// never follows: a 3xx is an error that names where it pointed.
func TestDo_NeverFollowsARedirectAndNeverForwardsTheKey(t *testing.T) {
	for _, status := range []int{
		http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var seenByTarget []string
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seenByTarget = append(seenByTarget, r.Header.Get("Authorization"))
				_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
			}))
			defer target.Close()
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.URL+"/api/health", status)
			}))
			defer origin.Close()

			c, err := NewClient(origin.URL, testKey)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}

			resp, err := c.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme")

			if len(seenByTarget) != 0 {
				t.Fatalf("the redirect was followed; the target saw Authorization %q", seenByTarget)
			}
			if err == nil {
				t.Fatalf("a %d was returned as success: %+v", status, resp)
			}
			apiErr := asAPIError(t, err)
			if apiErr.Code != "UNEXPECTED_REDIRECT" {
				t.Errorf("Code = %q, want UNEXPECTED_REDIRECT", apiErr.Code)
			}
			if apiErr.HTTPStatus != status {
				t.Errorf("HTTPStatus = %d, want %d", apiErr.HTTPStatus, status)
			}
			if !strings.Contains(apiErr.Message, "redirect") || !strings.Contains(apiErr.Message, target.URL) {
				t.Errorf("message does not say where it pointed: %q", apiErr.Message)
			}
			if strings.Contains(apiErr.Message, testKey) {
				t.Error("the message contains the API key")
			}
			if apiErr.RequestID == "" {
				t.Error("RequestID is empty")
			}
			if got := ExitCodeFor(err); got != 1 {
				t.Errorf("exit code = %d, want 1", got)
			}
		})
	}
}

// The case the finding names: an https service redirecting to plain http.
func TestDo_DoesNotFollowAnHTTPSToHTTPRedirect(t *testing.T) {
	var targetHits int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits++
		_, _ = w.Write([]byte(`{}`))
	}))
	defer target.Close()
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/api/health", http.StatusFound)
	}))
	defer origin.Close()

	c, err := NewClient(origin.URL, testKey)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.http.Transport = origin.Client().Transport // trust the test server's certificate

	_, err = c.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme")

	if targetHits != 0 {
		t.Fatalf("the https origin's redirect to plain http was followed (%d request(s) arrived)", targetHits)
	}
	if apiErr := asAPIError(t, err); apiErr.Code != "UNEXPECTED_REDIRECT" {
		t.Errorf("Code = %q, want UNEXPECTED_REDIRECT", apiErr.Code)
	}
}

// A Location can carry credentials in its userinfo; the message is printed and
// pasted into tickets.
func TestDo_RedirectMessageRedactsCredentialsInTheLocation(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://someone:hunter2@elsewhere.example.test/x", http.StatusFound)
	})

	_, err := c.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme")
	apiErr := asAPIError(t, err)
	if strings.Contains(apiErr.Message, "hunter2") {
		t.Errorf("message prints the Location's password: %q", apiErr.Message)
	}
	if !strings.Contains(apiErr.Message, "elsewhere.example.test") {
		t.Errorf("message does not name the host it pointed at: %q", apiErr.Message)
	}
}

func TestDo_ARedirectWithoutALocationIsStillAnError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusMultipleChoices) })

	_, err := c.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme")
	if apiErr := asAPIError(t, err); apiErr.Code != "UNEXPECTED_REDIRECT" || apiErr.HTTPStatus != 300 {
		t.Errorf("err = %+v", apiErr)
	}
}

func TestDo_ExitCodeMapping(t *testing.T) {
	tests := []struct {
		status   int
		wantExit int
	}{
		{200, 0},
		{400, 5},
		{401, 2},
		{403, 3},
		{404, 4},
		{409, 8},
		{422, 5},
		{429, 7},
		{500, 1},
		{502, 1},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"error":{"code":"X","message":"x"}}`))
			})
			_, err := c.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme")
			if got := ExitCodeFor(err); got != tt.wantExit {
				t.Errorf("HTTP %d: ExitCodeFor = %d, want %d (err=%v)", tt.status, got, tt.wantExit, err)
			}
		})
	}
}

func TestDo_ConnectionRefusedIsNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	c, err := NewClient(srv.URL, testKey)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	srv.Close() // nothing listens at the address any more

	_, err = c.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme")
	if err == nil {
		t.Fatal("expected a network error")
	}
	apiErr := asAPIError(t, err)
	if apiErr.Code != ErrNetwork {
		t.Errorf("Code = %q, want %q", apiErr.Code, ErrNetwork)
	}
	if apiErr.RequestID == "" {
		t.Error("a transport failure must still carry the request id")
	}
	if got := ExitCodeFor(err); got != 6 {
		t.Errorf("exit code = %d, want 6", got)
	}
}

func TestDo_RefusesAnOversizedResponse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		chunk := bytes.Repeat([]byte("x"), 1<<20)
		for i := 0; i <= maxResponseBytes>>20; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	})

	_, err := c.Do(context.Background(), http.MethodGet, "/api/members", nil, "acme")
	if err == nil {
		t.Fatal("expected an error for a response over the size cap")
	}
	if asAPIError(t, err).Code != ErrNetwork {
		t.Errorf("Code = %q, want %q", asAPIError(t, err).Code, ErrNetwork)
	}
}

// -----------------------------------------------------------------------------
// Verbose tracing
// -----------------------------------------------------------------------------

func TestVerbose_TracesAndRedactsTheKey(t *testing.T) {
	var sentID string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		sentID = r.Header.Get("X-Request-Id")
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	})
	var trace bytes.Buffer
	c.EnableVerbose(&trace)

	if _, err := c.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme"); err != nil {
		t.Fatalf("Do: %v", err)
	}
	out := trace.String()

	if strings.Contains(out, testKey) {
		t.Errorf("the trace contains the API key:\n%s", out)
	}
	if strings.Contains(out, "supersecret") || strings.Contains(out, testKey[4:]) {
		t.Errorf("the trace contains part of the secret:\n%s", out)
	}
	for _, want := range []string{
		"> GET " + c.baseURL + "/api/health",
		"> Authorization: Bearer ssk_...[REDACTED]",
		"> X-Tenant-Code: acme",
		"> X-Request-Id: " + sentID,
		"< HTTP 200",
		`< {"data":{"ok":true}}`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the trace is missing %q:\n%s", want, out)
		}
	}
}

// Authorization is redacted by name, whoever set it: a caller header that tried
// to shadow the key neither reaches the wire nor the trace.
func TestVerbose_NeverPrintsACallerSuppliedAuthorization(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) })
	var trace bytes.Buffer
	c.EnableVerbose(&trace)

	_, err := c.DoWithHeaders(context.Background(), http.MethodGet, "/api/health", nil, "acme", map[string]string{
		"authorization": "Bearer attacker-secret-value",
	})
	if err != nil {
		t.Fatalf("DoWithHeaders: %v", err)
	}
	if strings.Contains(trace.String(), "attacker-secret-value") {
		t.Errorf("the trace contains a caller-supplied Authorization value:\n%s", trace.String())
	}
	if n := strings.Count(trace.String(), "> Authorization:"); n != 1 {
		t.Errorf("the trace has %d Authorization lines, want 1:\n%s", n, trace.String())
	}
}

// NewClient refuses a base URL with userinfo, so this client cannot be built
// through it; the trace still redacts the URL, as a second line of defence for
// the day a URL reaches send() some other way. A trace is pasted into tickets.
func TestVerbose_RedactsCredentialsInTheRequestURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer srv.Close()

	c := &Client{
		http:    srv.Client(),
		baseURL: strings.Replace(srv.URL, "http://", "http://someone:hunter2@", 1),
		apiKey:  testKey,
	}
	var trace bytes.Buffer
	c.EnableVerbose(&trace)

	if _, err := c.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme"); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if strings.Contains(trace.String(), "hunter2") {
		t.Errorf("the trace prints the base URL's password:\n%s", trace.String())
	}
}

func TestVerbose_TracesAnErrorResponseAndATransportFailure(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"E0004_FORBIDDEN","message":"Forbidden"}}`))
	})
	var trace bytes.Buffer
	c.EnableVerbose(&trace)

	if _, err := c.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme"); err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(trace.String(), "< HTTP 403") || !strings.Contains(trace.String(), "E0004_FORBIDDEN") {
		t.Errorf("the trace of a 403 is missing the status or body:\n%s", trace.String())
	}

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	cd, err := NewClient(dead.URL, testKey)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	dead.Close()
	trace.Reset()
	cd.EnableVerbose(&trace)
	if _, err := cd.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme"); err == nil {
		t.Fatal("expected a network error")
	}
	if !strings.Contains(trace.String(), "< error:") {
		t.Errorf("the trace of a transport failure is missing:\n%s", trace.String())
	}
	if strings.Contains(trace.String(), testKey) {
		t.Errorf("the trace contains the API key:\n%s", trace.String())
	}
}

func TestVerbose_OffWritesNothingAndCanBeDisabled(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) })
	var trace bytes.Buffer
	c.EnableVerbose(&trace)
	c.EnableVerbose(nil)

	if _, err := c.Do(context.Background(), http.MethodGet, "/api/health", nil, "acme"); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if trace.Len() != 0 {
		t.Errorf("a disabled trace wrote %q", trace.String())
	}
}

func TestRedactedAuthorization(t *testing.T) {
	tests := map[string]string{
		"":                     "Bearer [REDACTED]",
		"ssk_short":            "Bearer ssk_...[REDACTED]", // 9 chars: long enough to show the prefix
		"ssk_1234":             "Bearer [REDACTED]",        // 8 chars: shown in full would be most of it
		"ssk_0123456789abcdef": "Bearer ssk_...[REDACTED]",
	}
	for key, want := range tests {
		if got := RedactedAuthorization(key); got != want {
			t.Errorf("RedactedAuthorization(%q) = %q, want %q", key, got, want)
		}
	}
}
