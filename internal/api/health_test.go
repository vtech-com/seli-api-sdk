package api

import (
	"context"
	"net/http"
	"testing"
)

func TestGetHealthSuccess(t *testing.T) {
	var gotMethod, gotPath, gotTenant string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotTenant = r.Method, r.URL.Path, r.Header.Get("X-Tenant-Code")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"ok":true,"timestamp":"2026-07-31T09:30:00.000Z"}}`))
	})

	health, err := c.GetHealth(context.Background(), "acme")
	if err != nil {
		t.Fatalf("GetHealth: %v", err)
	}
	// The base URL is the host only; the /api prefix lives with the path. The
	// old /health and /api/v1/health both 404 on the real service.
	if gotMethod != http.MethodGet || gotPath != "/api/health" {
		t.Errorf("request = %s %s, want GET /api/health", gotMethod, gotPath)
	}
	if gotTenant != "acme" {
		t.Errorf("X-Tenant-Code = %q, want acme", gotTenant)
	}
	if !health.OK {
		t.Error("OK: got false, want true")
	}
	if health.Timestamp != "2026-07-31T09:30:00.000Z" {
		t.Errorf("Timestamp: got %q", health.Timestamp)
	}
}

func TestGetHealthUnauthorized(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"E0003_UNAUTHORIZED","message":"Unauthorized"}}`))
	})

	_, err := c.GetHealth(context.Background(), "acme")
	apiErr := asAPIError(t, err)
	if apiErr.HTTPStatus != http.StatusUnauthorized {
		t.Errorf("HTTPStatus: got %d, want %d", apiErr.HTTPStatus, http.StatusUnauthorized)
	}
	if got := ExitCodeFor(err); got != 2 {
		t.Errorf("exit code = %d, want 2", got)
	}
}

func TestGetHealthWrongTenantIsForbidden(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"E0004_FORBIDDEN","message":"Forbidden"}}`))
	})

	_, err := c.GetHealth(context.Background(), "not-my-tenant")
	if apiErr := asAPIError(t, err); apiErr.Code != "E0004_FORBIDDEN" {
		t.Errorf("Code = %q, want E0004_FORBIDDEN", apiErr.Code)
	}
	if got := ExitCodeFor(err); got != 3 {
		t.Errorf("exit code = %d, want 3", got)
	}
}

func TestGetHealthMalformedBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>not json</html>`))
	})

	_, err := c.GetHealth(context.Background(), "acme")
	apiErr := asAPIError(t, err)
	if apiErr.Code != ErrNetwork {
		t.Errorf("Code = %q, want %q", apiErr.Code, ErrNetwork)
	}
	if apiErr.RequestID == "" {
		t.Error("a malformed reply must still carry the request id")
	}
	if string(apiErr.RawBody) != `<html>not json</html>` {
		t.Errorf("RawBody = %q, want the body the server sent", apiErr.RawBody)
	}
}
