package api

// Local, CLI-detected error codes — the server was never called.
const (
	ErrTenantRequired  = "TENANT_REQUIRED"   // no tenant resolved for an endpoint that requires one
	ErrInvalidMemberID = "INVALID_MEMBER_ID" // <id> positional arg is not a well-formed UUID
	ErrNetwork         = "NETWORK_ERROR"     // transport-level failure (DNS, timeout, connection refused)
)

// Per-endpoint tenant requirements, read from api/openapi.json's
// components.parameters.XTenantCode (required: true on both operations).
// Encoded here so adding a genuinely optional endpoint later doesn't require
// touching command code.
const (
	HealthRequiresTenant  = true
	MembersRequiresTenant = true
)

// Error is a server-returned or transport-level failure from the API client.
// cmd/ call sites convert this into a *cmd.CLIError.
type Error struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *Error) Error() string { return e.Message }
