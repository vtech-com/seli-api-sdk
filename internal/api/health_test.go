package api

import (
	"context"
	"net/http"
	"testing"
)

func TestGetHealthSuccess(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("path: got %q, want %q", r.URL.Path, "/health")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"ok":true,"timestamp":"2026-07-31T09:30:00.000Z"}}`))
	})

	health, err := c.GetHealth(context.Background(), "acme")
	if err != nil {
		t.Fatalf("GetHealth: %v", err)
	}
	if !health.OK {
		t.Error("OK: got false, want true")
	}
	if health.Timestamp != "2026-07-31T09:30:00.000Z" {
		t.Errorf("Timestamp: got %q", health.Timestamp)
	}
}

func TestGetHealthUnauthorized(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"E0003_UNAUTHORIZED","message":"Unauthorized"}}`))
	})

	_, err := c.GetHealth(context.Background(), "acme")
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if apiErr.HTTPStatus != http.StatusUnauthorized {
		t.Errorf("HTTPStatus: got %d, want %d", apiErr.HTTPStatus, http.StatusUnauthorized)
	}
}
