package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
	"github.com/vtech-com/seli-api-sdk/internal/api"
	"github.com/vtech-com/seli-api-sdk/internal/config"
	"github.com/vtech-com/seli-api-sdk/internal/output"
	"github.com/vtech-com/seli-api-sdk/internal/version"
)

// lookupError is the error catalog the renderer consults. It is a variable so
// a test can stand a code in while the real catalog has no entries.
var lookupError = api.LookupError

// buildClient loads config and constructs an api.Client using the active
// profile. SELI_API_KEY overrides the profile key. The base URL is the first of
// --api-url, SELI_API_URL, the profile's api_url, and the default baked into
// the build; if none of them is set the CLI fails locally (API_URL_MISSING) and
// sends nothing.
func buildClient() (*api.Client, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, &api.APIError{Code: "CONFIG_LOAD_ERROR", Message: err.Error()}
	}

	profile, err := config.ActiveProfile(cfg)
	if err != nil {
		if len(cfg.Profiles) > 0 {
			// active_profile names a profile that was never written. That is a
			// real config fault, and its own message says so.
			return nil, nil, &api.APIError{Code: "CONFIG_LOAD_ERROR", Message: fmt.Sprintf("get active profile: %v", err)}
		}
		// No profiles at all: auth login has never run here. "profile default
		// not found" describes the config file's insides; the caller's actual
		// situation is that there is no key, which the check below reports in
		// those terms. An env-supplied key still works from a bare machine.
		profile = &config.Profile{}
	}

	apiKey := strings.TrimSpace(profile.APIKey)
	if k := strings.TrimSpace(viper.GetString("api_key")); k != "" {
		apiKey = k
	}
	if apiKey == "" {
		// Sending "Authorization: Bearer " with nothing after it costs a round
		// trip to learn what is already known here, and comes back as a 401 that
		// points at the wrong repair. Diagnose it locally, as the condition it is.
		return nil, nil, &api.APIError{
			Code:           api.ErrAuthMissingHeader,
			Message:        "no API key configured: SELI_API_KEY is unset and the active profile in ~/.seli/config.json has none. Run `seli auth login --key <ssk_...>`, or set SELI_API_KEY",
			HTTPStatus:     401,
			LocalDiagnosis: true,
		}
	}

	baseURL := api.ResolveBaseURL(apiURL, viper.GetString("api_url"), profile.APIURL, version.DefaultAPIURL)
	client, err := api.NewClient(baseURL, apiKey)
	if err != nil {
		return nil, nil, err
	}

	if verbose {
		client.EnableVerbose(os.Stderr)
	}

	return client, cfg, nil
}

// resolveTenant applies the documented resolution order: --tenant flag, then
// SELI_TENANT env (both merged by viper via BindPFlag/BindEnv), then the active
// profile's default_tenant. It returns the code and the layer it came from,
// both "" if none resolve.
func resolveTenant(cfg *config.Config) (code, source string) {
	var defaultTenant string
	if p, err := config.ActiveProfile(cfg); err == nil {
		defaultTenant = p.DefaultTenant
	}
	return api.ResolveTenant(tenant, viper.GetString("tenant"), defaultTenant)
}

// requireTenant exits 5 when a tenant-scoped command resolved no tenant. A
// request without a tenant header is not a narrower request; it is a request
// the server cannot place.
func requireTenant(code string) {
	if code == "" {
		fail(api.ErrTenantRequired, "no tenant resolved: pass --tenant, set SELI_TENANT, or run `seli config set-default-tenant`", 400)
	}
}

// errorDetail turns err into the self-diagnosing error the renderer prints: the
// raw API fields, enriched with the catalog interpretation when the code is
// known.
func errorDetail(err error) output.ErrorDetail {
	detail := output.ErrorDetail{Code: "ERROR", Message: err.Error()}
	serverResponded := false
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		detail.Code = apiErr.Code
		detail.Message = apiErr.Message
		detail.RequestID = apiErr.RequestID
		detail.HTTPStatus = apiErr.HTTPStatus
		detail.RawBody = string(apiErr.RawBody)
		serverResponded = len(apiErr.RawBody) > 0
	}
	// Only enrich with the catalog interpretation (meaning, next step, capability
	// brake) when the server actually responded. Locally-constructed and cobra
	// arg-validation errors never round-tripped, so the brake — "a failed write
	// is not a missing capability" — would be misleading noise there.
	if serverResponded || (apiErr != nil && apiErr.LocalDiagnosis) {
		if info, ok := lookupError(detail.Code); ok {
			detail.Meaning = info.Meaning
			detail.Next = info.Next
			detail.CapabilityNote = info.CapabilityNote
		}
	}
	return detail
}

// renderCLIError renders err as a self-diagnosing error: the full diagnostic
// block goes to stdout where an AI agent reads, and a concise line goes to
// stderr. It does not exit.
func renderCLIError(err error) {
	output.RenderError(os.Stdout, os.Stderr, errorDetail(err))
}

// handleErr renders the error and exits with the appropriate exit code.
// Returns nil so RunE callers do not trigger cobra's double-print.
func handleErr(err error) error {
	renderCLIError(err)
	os.Exit(api.ExitCodeFor(err))
	return nil
}

// failWithData ends a command that fetched real rows and then failed. It prints
// one document carrying both — the error key above the rows it qualifies — and
// exits non-zero.
//
// Nothing is printed before this call: the rows are held until the outcome is
// known, so stdout never receives an envelope that a later error would have to
// contradict.
func failWithData(err error, data any, meta output.Meta) error {
	_ = output.WritePartial(os.Stdout, os.Stderr, data, meta, errorDetail(err))
	os.Exit(api.ExitCodeFor(err))
	return nil
}

// fail renders a locally-detected error — a bad flag combination, an
// unreadable config — and exits. The server was never called, so there is no
// request id and no catalog entry to enrich it with.
func fail(code, message string, httpStatus int) {
	e := &api.APIError{Code: code, Message: message, HTTPStatus: httpStatus}
	renderCLIError(e)
	os.Exit(api.ExitCodeFor(e))
}

// failValidation is the common case: the caller asked for something the CLI
// can see is wrong without asking the server. Exit 5.
func failValidation(format string, args ...any) {
	fail(api.ErrValidation, fmt.Sprintf(format, args...), 400)
}
