package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/viper"
	"github.com/vtech-com/seli-api-sdk/internal/api"
	"github.com/vtech-com/seli-api-sdk/internal/config"
)

// CLIError is a locally-detected error: a bad flag combination, an unreadable
// config, an unresolved tenant. The server was never called.
//
// This scaffold does not yet ship an API client — when one is added, extend
// this type (or replace it) to carry the server's error code and request id
// the same way, so exit codes and envelopes stay consistent across the CLI.
type CLIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *CLIError) Error() string { return e.Message }

// ExitCodeFor maps an error to a process exit code. Kept intentionally small;
// grow this table as real API error codes are wired in.
func ExitCodeFor(err error) int {
	var cliErr *CLIError
	if e, ok := err.(*CLIError); ok {
		cliErr = e
	}
	if cliErr == nil {
		return 1
	}
	if cliErr.Code == "NETWORK_ERROR" {
		return 6
	}
	switch cliErr.HTTPStatus {
	case 401:
		return 2
	case 403:
		return 3
	case 404:
		return 4
	case 400:
		return 5
	case 429:
		return 7
	default:
		return 1
	}
}

// failAPIError renders an *api.Error (from internal/api) as a *CLIError and
// exits, preserving the server's code/message and HTTP status.
func failAPIError(err error) {
	if apiErr, ok := err.(*api.Error); ok {
		fail(apiErr.Code, apiErr.Message, apiErr.HTTPStatus)
	}
	fail("UNKNOWN_ERROR", err.Error(), 0)
}

// resolveTenant applies the documented resolution order: --tenant flag,
// then SELI_TENANT env (both already merged by viper via BindPFlag/BindEnv),
// then the active profile's default_tenant. Returns "" if none resolve.
func resolveTenant() string {
	if t := viper.GetString("tenant"); t != "" {
		return t
	}
	cfg, err := config.Load()
	if err != nil {
		return ""
	}
	p, err := config.ActiveProfile(cfg)
	if err != nil {
		return ""
	}
	return p.DefaultTenant
}

// envelope is the one output shape every command prints on stdout:
// { "data": …, "meta": … }. There is no other shape to branch on.
type envelope struct {
	Data any `json:"data"`
	Meta any `json:"meta"`
}

// writeEnvelope prints data on stdout in the standard envelope, with an
// empty meta. Used by single-record commands.
func writeEnvelope(data any) error {
	return writeEnvelopeWithMeta(data, struct{}{})
}

// writeEnvelopeWithMeta prints data and meta on stdout in the standard
// envelope. Used by list commands that carry pagination in meta.
func writeEnvelopeWithMeta(data, meta any) error {
	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(envelope{Data: data, Meta: meta})
}

// renderCLIError prints a concise error line to stderr and an error envelope
// to stdout. It does not exit.
func renderCLIError(err error) {
	code := "ERROR"
	if cliErr, ok := err.(*CLIError); ok && cliErr.Code != "" {
		code = cliErr.Code
	}
	fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{
		Error: struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}{Code: code, Message: err.Error()},
	})
}

// fail renders a locally-detected error and exits. Exists so callers don't
// repeat the render-then-exit ritual at every call site.
func fail(code, message string, httpStatus int) {
	e := &CLIError{Code: code, Message: message, HTTPStatus: httpStatus}
	renderCLIError(e)
	os.Exit(ExitCodeFor(e))
}

// failValidation is the common case: the caller asked for something the CLI
// can see is wrong without asking the server. Exit 5.
func failValidation(format string, args ...any) {
	fail("VALIDATION_ERROR", fmt.Sprintf(format, args...), 400)
}
