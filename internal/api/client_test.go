package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, APIKey: "ssk_test", HTTPClient: srv.Client()}
}

func TestGetSetsHeaders(t *testing.T) {
	var gotAuth, gotUA, gotReqID, gotTenant string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotReqID = r.Header.Get("X-Request-Id")
		gotTenant = r.Header.Get("X-Tenant-Code")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	if _, err := c.Get(context.Background(), "/health", nil, "acme"); err != nil {
		t.Fatalf("Get: %v", err)
	}

	if gotAuth != "Bearer ssk_test" {
		t.Errorf("Authorization: got %q, want %q", gotAuth, "Bearer ssk_test")
	}
	if gotUA == "" {
		t.Error("User-Agent header not set")
	}
	if gotReqID == "" {
		t.Error("X-Request-Id header not set")
	}
	if gotTenant != "acme" {
		t.Errorf("X-Tenant-Code: got %q, want %q", gotTenant, "acme")
	}
}

func TestGetOmitsTenantHeaderWhenUnresolved(t *testing.T) {
	var sawHeader bool
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, sawHeader = r.Header["X-Tenant-Code"]
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	if _, err := c.Get(context.Background(), "/health", nil, ""); err != nil {
		t.Fatalf("Get: %v", err)
	}

	if sawHeader {
		t.Error("X-Tenant-Code header should be omitted, not sent empty")
	}
}

func TestGetSendsQuery(t *testing.T) {
	var gotQuery url.Values
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	q := url.Values{"page": {"2"}}
	if _, err := c.Get(context.Background(), "/members", q, "acme"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotQuery.Get("page") != "2" {
		t.Errorf("query page: got %q, want %q", gotQuery.Get("page"), "2")
	}
}

func TestGetDecodesErrorEnvelope(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"E0003_UNAUTHORIZED","message":"Unauthorized"}}`))
	})

	_, err := c.Get(context.Background(), "/health", nil, "acme")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if apiErr.HTTPStatus != http.StatusUnauthorized {
		t.Errorf("HTTPStatus: got %d, want %d", apiErr.HTTPStatus, http.StatusUnauthorized)
	}
	if apiErr.Code != "E0003_UNAUTHORIZED" {
		t.Errorf("Code: got %q, want %q", apiErr.Code, "E0003_UNAUTHORIZED")
	}
	if apiErr.Message != "Unauthorized" {
		t.Errorf("Message: got %q, want %q", apiErr.Message, "Unauthorized")
	}
}

func TestGetDecodesMalformedErrorBody(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`not json`))
	})

	_, err := c.Get(context.Background(), "/health", nil, "acme")
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if apiErr.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("HTTPStatus: got %d, want %d", apiErr.HTTPStatus, http.StatusInternalServerError)
	}
}

func TestGetNetworkError(t *testing.T) {
	c := &Client{BaseURL: "http://127.0.0.1:0", APIKey: "ssk_test", HTTPClient: http.DefaultClient}

	_, err := c.Get(context.Background(), "/health", nil, "acme")
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if apiErr.Code != ErrNetwork {
		t.Errorf("Code: got %q, want %q", apiErr.Code, ErrNetwork)
	}
}
