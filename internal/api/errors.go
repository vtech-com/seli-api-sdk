package api

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// Local, CLI-detected error codes — the server was never called, with one
// exception: ErrNetwork also covers a transport failure after the request left.
const (
	ErrTenantRequired    = "TENANT_REQUIRED"     // no tenant resolved for an endpoint that requires one
	ErrInvalidMemberID   = "INVALID_MEMBER_ID"   // <id> positional arg is not a well-formed UUID
	ErrNetwork           = "NETWORK_ERROR"       // transport-level failure (DNS, timeout, connection refused, unreadable body)
	ErrValidation        = "VALIDATION_ERROR"    // the caller asked for something the CLI can see is wrong; exit 5
	ErrAuthMissingHeader = "AUTH_MISSING_HEADER" // no API key configured; exit 2
	ErrInsecureBaseURL   = "INSECURE_BASE_URL"   // non-HTTPS, non-loopback base URL; exit 5
	ErrAPIURLMissing     = "API_URL_MISSING"     // no base URL from any layer, and none baked into the build; exit 5
)

// Per-endpoint tenant requirements, read from api/openapi.json's
// components.parameters.XTenantCode (required: true on both operations).
// Encoded here so adding a genuinely optional endpoint later doesn't require
// touching command code.
const (
	HealthRequiresTenant  = true
	MembersRequiresTenant = true
)

// APIError is the one error type of this CLI: a failure the server returned, a
// transport failure on the way to it, or a condition the CLI diagnosed itself
// before sending anything.
type APIError struct {
	Code       string
	Message    string
	RequestID  string
	HTTPStatus int
	// RawBody is the verbatim response body from the server, when available.
	// It is surfaced on error so callers (especially AI agents) see the real
	// server response without having to re-run with --verbose.
	RawBody []byte
	// LocalDiagnosis marks an error the CLI diagnosed itself, without a round
	// trip, but whose Code is nonetheless a real catalog code the caller should
	// get the full interpretation for. The renderer normally withholds the
	// catalog from errors the server never answered — a cobra arg-validation
	// failure carries no server truth to interpret — so a locally-detected
	// condition that IS the catalog condition has to say so explicitly.
	LocalDiagnosis bool

	// cause is the transport error underneath an ErrNetwork, kept so that
	// errors.Is / errors.As (context.DeadlineExceeded, net.Error) see through
	// this type to the failure that actually happened.
	cause error
}

func (e *APIError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("%s: %s (request_id=%s)", e.Code, e.Message, e.RequestID)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap exposes the transport error behind an ErrNetwork, if there is one.
func (e *APIError) Unwrap() error { return e.cause }

// ExitCodeFor maps an error to the standardized exit codes.
//
// 0 — success (nil error)
// 1 — general / unexpected error
// 2 — auth error (HTTP 401)
// 3 — permission error (HTTP 403)
// 4 — not found (HTTP 404)
// 5 — validation error (HTTP 400 / 422)
// 6 — network error (DNS, timeout, connection refused)
// 7 — rate limit (HTTP 429)
// 8 — conflict (HTTP 409)
//
// It uses errors.As, so an error wrapped with %w keeps its code.
func ExitCodeFor(err error) int {
	if err == nil {
		return 0
	}

	// Network errors: context deadline or net.Error (timeout, DNS, etc.)
	if errors.Is(err, context.DeadlineExceeded) {
		return 6
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return 6
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if apiErr.Code == ErrNetwork {
			return 6
		}
		switch apiErr.HTTPStatus {
		case 401:
			return 2
		case 403:
			return 3
		case 404:
			return 4
		case 400, 422:
			return 5
		case 409:
			return 8
		case 429:
			return 7
		}
	}

	return 1
}
