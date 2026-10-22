package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// timeoutNetError is a net.Error that needs no DNS or socket to construct.
type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "i/o timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

func TestExitCodeFor(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"nil error is success", nil, 0},
		{"generic error is 1", errors.New("boom"), 1},

		{"401 is 2", &APIError{HTTPStatus: 401}, 2},
		{"403 is 3", &APIError{HTTPStatus: 403}, 3},
		{"404 is 4", &APIError{HTTPStatus: 404}, 4},
		{"400 is 5", &APIError{HTTPStatus: 400}, 5},
		{"422 is 5", &APIError{HTTPStatus: 422}, 5},
		{"429 is 7", &APIError{HTTPStatus: 429}, 7},
		{"409 is 8", &APIError{HTTPStatus: 409}, 8},
		{"500 is 1", &APIError{HTTPStatus: 500}, 1},
		{"no status is 1", &APIError{Code: "X"}, 1},

		{"context deadline is 6", context.DeadlineExceeded, 6},
		{"net.Error is 6", timeoutNetError{}, 6},
		{"NETWORK_ERROR code is 6", &APIError{Code: ErrNetwork}, 6},
		{"NETWORK_ERROR beats a status", &APIError{Code: ErrNetwork, HTTPStatus: 200}, 6},

		// The reason ExitCodeFor uses errors.As: a wrapped error keeps its code.
		{"wrapped APIError 401 is 2", fmt.Errorf("outer: %w", &APIError{HTTPStatus: 401}), 2},
		{"wrapped APIError 403 is 3", fmt.Errorf("outer: %w", &APIError{HTTPStatus: 403}), 3},
		{"wrapped APIError 404 is 4", fmt.Errorf("outer: %w", &APIError{HTTPStatus: 404}), 4},
		{"wrapped APIError 409 is 8", fmt.Errorf("a: %w", fmt.Errorf("b: %w", &APIError{HTTPStatus: 409})), 8},
		{"wrapped APIError 429 is 7", fmt.Errorf("outer: %w", &APIError{HTTPStatus: 429}), 7},
		{"wrapped context deadline is 6", fmt.Errorf("wrapped: %w", context.DeadlineExceeded), 6},
		{"wrapped net.Error is 6", fmt.Errorf("wrapped: %w", timeoutNetError{}), 6},
		{"APIError around a net.Error is 6", &APIError{Code: "X", HTTPStatus: 401, cause: timeoutNetError{}}, 6},

		// Locally diagnosed conditions map through the status they are given.
		{"AUTH_MISSING_HEADER is 2", &APIError{Code: ErrAuthMissingHeader, HTTPStatus: 401, LocalDiagnosis: true}, 2},
		{"INSECURE_BASE_URL is 5", &APIError{Code: ErrInsecureBaseURL, HTTPStatus: 400}, 5},
		{"API_URL_MISSING is 5", &APIError{Code: ErrAPIURLMissing, HTTPStatus: 400}, 5},
		{"TENANT_REQUIRED is 5", &APIError{Code: ErrTenantRequired, HTTPStatus: 400}, 5},

		// A 3xx is not followed and is not one of the mapped statuses: exit 1.
		{"UNEXPECTED_REDIRECT is 1", &APIError{Code: ErrUnexpectedRedirect, HTTPStatus: 302}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCodeFor(tt.err); got != tt.wantCode {
				t.Errorf("ExitCodeFor(%v) = %d, want %d", tt.err, got, tt.wantCode)
			}
		})
	}
}

func TestAPIErrorKeepsItsTransportCause(t *testing.T) {
	cause := fmt.Errorf("dial: %w", context.DeadlineExceeded)
	err := fmt.Errorf("outer: %w", &APIError{Code: ErrNetwork, Message: "m", cause: cause})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Error("errors.Is cannot see the deadline through the APIError")
	}

	if (&APIError{Code: "X"}).Unwrap() != nil {
		t.Error("an APIError with no transport failure must unwrap to nil")
	}
}

func TestAPIErrorString(t *testing.T) {
	withID := &APIError{Code: "E0003_UNAUTHORIZED", Message: "Unauthorized", RequestID: "req-1"}
	if got := withID.Error(); got != "E0003_UNAUTHORIZED: Unauthorized (request_id=req-1)" {
		t.Errorf("Error() = %q", got)
	}
	local := &APIError{Code: ErrAuthMissingHeader, Message: "no key"}
	if got := local.Error(); got != "AUTH_MISSING_HEADER: no key" {
		t.Errorf("Error() = %q", got)
	}
	if strings.Contains(local.Error(), "request_id") {
		t.Error("a local error has no request id to quote")
	}
}
