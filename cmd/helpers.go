package cmd

import (
	"encoding/json"
	"fmt"
	"os"
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
	switch cliErr.HTTPStatus {
	case 404:
		return 4
	case 400:
		return 5
	default:
		return 1
	}
}

// envelope is the one output shape every command prints on stdout:
// { "data": …, "meta": … }. There is no other shape to branch on.
type envelope struct {
	Data any `json:"data"`
	Meta any `json:"meta"`
}

// writeEnvelope prints data on stdout in the standard envelope.
func writeEnvelope(data any) error {
	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(envelope{Data: data, Meta: struct{}{}})
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
