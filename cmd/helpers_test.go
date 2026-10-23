package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vtech-com/seli-api-sdk/internal/api"
	"github.com/vtech-com/seli-api-sdk/internal/config"
	"github.com/vtech-com/seli-api-sdk/internal/version"
)

// withConfig points the config store at a temp file for the duration of a test,
// writing contents when it is non-empty (an empty string means "no config file
// at all" — the state of a machine where auth login has never run), and clears
// every layer that would otherwise leak in from the developer's own environment.
func withConfig(t *testing.T, contents string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if contents != "" {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}
	orig := config.ConfigPath
	config.ConfigPath = func() (string, error) { return path, nil }
	t.Cleanup(func() { config.ConfigPath = orig })

	for _, k := range []string{"SELI_API_KEY", "SELI_API_URL", "SELI_TENANT"} {
		t.Setenv(k, "")
	}

	origAPIURL, origTenant, origVerbose, origDefault := apiURL, tenant, verbose, version.DefaultAPIURL
	apiURL, tenant, verbose, version.DefaultAPIURL = "", "", false, ""
	t.Cleanup(func() {
		apiURL, tenant, verbose, version.DefaultAPIURL = origAPIURL, origTenant, origVerbose, origDefault
	})
}

// captureOutput runs fn with os.Stdout and os.Stderr redirected and returns what
// it wrote to each. The outputs under test are small, well under a pipe buffer.
func captureOutput(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	defer func() { os.Stdout, os.Stderr = origOut, origErr }()

	fn()

	_ = outW.Close()
	_ = errW.Close()
	os.Stdout, os.Stderr = origOut, origErr
	o, _ := io.ReadAll(outR)
	e, _ := io.ReadAll(errR)
	return string(o), string(e)
}

// -----------------------------------------------------------------------------
// buildClient
// -----------------------------------------------------------------------------

// The first run of a fresh install must name the situation the caller is in —
// no key — rather than the shape of a config file they have never seen.
func TestBuildClient_NoKeyIsALocalDiagnosis(t *testing.T) {
	cases := map[string]string{
		"no config file at all":       "",
		"profile exists, key cleared": `{"active_profile":"default","profiles":{"default":{"api_url":"https://api.example.test"}}}`,
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			withConfig(t, contents)

			client, _, err := buildClient()

			if err == nil || client != nil {
				t.Fatalf("buildClient succeeded with no API key configured (client %v, err %v)", client, err)
			}
			var apiErr *api.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error is %T, want *api.APIError", err)
			}
			if apiErr.Code != "AUTH_MISSING_HEADER" {
				t.Errorf("code = %q, want AUTH_MISSING_HEADER", apiErr.Code)
			}
			if !apiErr.LocalDiagnosis {
				t.Error("LocalDiagnosis = false; the catalog interpretation would be withheld")
			}
			for _, want := range []string{"SELI_API_KEY", "seli auth login"} {
				if !strings.Contains(apiErr.Message, want) {
					t.Errorf("message %q does not name %q", apiErr.Message, want)
				}
			}
			if got := api.ExitCodeFor(err); got != 2 {
				t.Errorf("exit code = %d, want 2 (auth)", got)
			}
		})
	}
}

// A key in the environment is a complete credential on its own: a machine that
// has never run auth login must still work when SELI_API_KEY is set.
func TestBuildClient_EnvKeyNeedsNoConfigFile(t *testing.T) {
	withConfig(t, "")
	t.Setenv("SELI_API_KEY", "ssk_from_env")
	t.Setenv("SELI_API_URL", "http://127.0.0.1:1")

	client, cfg, err := buildClient()

	if err != nil {
		t.Fatalf("buildClient: %v", err)
	}
	if client == nil || cfg == nil {
		t.Fatal("client or config is nil")
	}
}

func TestBuildClient_ProfileKeyAndURLAreUsed(t *testing.T) {
	withConfig(t, `{"active_profile":"default","profiles":{"default":{"api_key":"ssk_stored","api_url":"http://127.0.0.1:1"}}}`)

	if _, _, err := buildClient(); err != nil {
		t.Fatalf("buildClient: %v", err)
	}
}

func TestBuildClient_EveryURLLayerEmptyIsAPIURLMissing(t *testing.T) {
	withConfig(t, "")
	t.Setenv("SELI_API_KEY", "ssk_from_env")

	_, _, err := buildClient()

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != api.ErrAPIURLMissing {
		t.Fatalf("err = %v, want API_URL_MISSING", err)
	}
	if !strings.Contains(apiErr.Message, "SELI_API_URL") {
		t.Errorf("message does not name SELI_API_URL: %q", apiErr.Message)
	}
	if got := api.ExitCodeFor(err); got != 5 {
		t.Errorf("exit code = %d, want 5", got)
	}
}

// The build-time default is the last resort, not an override.
func TestBuildClient_BuildDefaultFillsTheGapOnly(t *testing.T) {
	withConfig(t, "")
	t.Setenv("SELI_API_KEY", "ssk_from_env")
	version.DefaultAPIURL = "http://127.0.0.1:1"

	if _, _, err := buildClient(); err != nil {
		t.Fatalf("buildClient with a baked-in default: %v", err)
	}

	// A higher layer that is itself refused must not fall through to the default.
	t.Setenv("SELI_API_URL", "http://api.example.test")
	_, _, err := buildClient()
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != api.ErrInsecureBaseURL {
		t.Errorf("err = %v, want INSECURE_BASE_URL from the env layer", err)
	}
}

func TestBuildClient_RefusesPlainHTTPToARemoteHost(t *testing.T) {
	withConfig(t, "")
	t.Setenv("SELI_API_KEY", "ssk_from_env")
	t.Setenv("SELI_API_URL", "http://api.example.test")

	_, _, err := buildClient()

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != api.ErrInsecureBaseURL {
		t.Fatalf("err = %v, want INSECURE_BASE_URL", err)
	}
	if got := api.ExitCodeFor(err); got != 5 {
		t.Errorf("exit code = %d, want 5", got)
	}
}

// An active_profile naming a profile that was never written is a real config
// fault, and keeps its own message — it is not the same thing as "no key".
func TestBuildClient_MissingNamedProfileKeepsItsOwnError(t *testing.T) {
	withConfig(t, `{"active_profile":"staging","profiles":{"default":{"api_key":"ssk_x"}}}`)

	_, _, err := buildClient()

	if err == nil || !strings.Contains(err.Error(), `profile "staging" not found`) {
		t.Errorf("error = %v, want it to name the missing profile", err)
	}
	if got := api.ExitCodeFor(err); got != 1 {
		t.Errorf("exit code = %d, want 1", got)
	}
}

func TestBuildClient_UnreadableConfigIsAConfigError(t *testing.T) {
	withConfig(t, `{not json`)

	_, _, err := buildClient()

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "CONFIG_LOAD_ERROR" {
		t.Errorf("err = %v, want CONFIG_LOAD_ERROR", err)
	}
}

// -----------------------------------------------------------------------------
// resolveTenant
// -----------------------------------------------------------------------------

func TestResolveTenant(t *testing.T) {
	cfg := &config.Config{Profiles: map[string]config.Profile{"default": {DefaultTenant: "from-config"}}}

	withConfig(t, "")
	if code, source := resolveTenant(cfg); code != "from-config" || source != "config" {
		t.Errorf("config layer = (%q, %q)", code, source)
	}

	t.Setenv("SELI_TENANT", "from-env")
	if code, source := resolveTenant(cfg); code != "from-env" || source != "env" {
		t.Errorf("env layer = (%q, %q)", code, source)
	}

	tenant = "from-flag"
	if code, source := resolveTenant(cfg); code != "from-flag" || source != "flag" {
		t.Errorf("flag layer = (%q, %q)", code, source)
	}

	tenant = ""
	t.Setenv("SELI_TENANT", "")
	if code, source := resolveTenant(&config.Config{}); code != "" || source != "" {
		t.Errorf("nothing configured = (%q, %q), want empty", code, source)
	}
}

// -----------------------------------------------------------------------------
// Error rendering and the catalog
// -----------------------------------------------------------------------------

func withCatalog(t *testing.T, entries map[string]api.ErrorInfo) {
	t.Helper()
	orig := lookupError
	lookupError = func(code string) (api.ErrorInfo, bool) {
		info, ok := entries[code]
		return info, ok
	}
	t.Cleanup(func() { lookupError = orig })
}

type renderedError struct {
	Error map[string]any `json:"error"`
}

func renderToDoc(t *testing.T, err error) (renderedError, string) {
	t.Helper()
	stdout, stderr := captureOutput(t, func() { renderCLIError(err) })
	var doc renderedError
	if jerr := json.Unmarshal([]byte(stdout), &doc); jerr != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", jerr, stdout)
	}
	return doc, stderr
}

// The catalog interprets what the server said. It is withheld from a failure the
// server never answered — a cobra usage error carries no server truth — unless
// the CLI says the condition IS the catalog condition (LocalDiagnosis).
func TestRenderCLIError_CatalogGating(t *testing.T) {
	withCatalog(t, map[string]api.ErrorInfo{
		"E_TEST": {Meaning: "what it means", Next: "what to do", CapabilityNote: true},
	})

	t.Run("server answered: enriched", func(t *testing.T) {
		doc, _ := renderToDoc(t, &api.APIError{Code: "E_TEST", Message: "m", HTTPStatus: 400, RawBody: []byte(`{"error":{}}`)})
		if doc.Error["next"] != "what to do" || doc.Error["meaning"] != "what it means" {
			t.Errorf("error = %v, want the catalog's next and meaning", doc.Error)
		}
		if note, _ := doc.Error["capability_note"].(string); !strings.Contains(note, "does NOT mean") {
			t.Errorf("capability_note = %v", doc.Error["capability_note"])
		}
	})

	t.Run("wrapped server error: enriched", func(t *testing.T) {
		wrapped := fmt.Errorf("outer: %w", &api.APIError{Code: "E_TEST", Message: "m", HTTPStatus: 400, RawBody: []byte(`x`)})
		doc, _ := renderToDoc(t, wrapped)
		if doc.Error["code"] != "E_TEST" || doc.Error["next"] != "what to do" {
			t.Errorf("error = %v", doc.Error)
		}
	})

	t.Run("local error: withheld", func(t *testing.T) {
		doc, _ := renderToDoc(t, &api.APIError{Code: "E_TEST", Message: "m", HTTPStatus: 400})
		for _, key := range []string{"next", "meaning", "capability_note"} {
			if _, present := doc.Error[key]; present {
				t.Errorf("a failure the server never answered carries %q: %v", key, doc.Error)
			}
		}
	})

	t.Run("local diagnosis: enriched", func(t *testing.T) {
		doc, _ := renderToDoc(t, &api.APIError{Code: "E_TEST", Message: "m", HTTPStatus: 401, LocalDiagnosis: true})
		if doc.Error["next"] != "what to do" {
			t.Errorf("error = %v, want the catalog's next", doc.Error)
		}
	})

	t.Run("unknown code: nothing invented", func(t *testing.T) {
		doc, _ := renderToDoc(t, &api.APIError{Code: "NOT_IN_CATALOG", Message: "m", HTTPStatus: 400, RawBody: []byte(`x`)})
		if _, present := doc.Error["next"]; present {
			t.Errorf("error = %v", doc.Error)
		}
	})
}

// With the real catalog — empty until the backend's code list supplies entries —
// nothing is ever enriched, whatever the code.
func TestRenderCLIError_RealCatalogAddsNothing(t *testing.T) {
	doc, _ := renderToDoc(t, &api.APIError{Code: "AUTH_MISSING_HEADER", Message: "m", HTTPStatus: 401, LocalDiagnosis: true, RawBody: []byte(`x`)})
	for _, key := range []string{"next", "meaning", "capability_note"} {
		if _, present := doc.Error[key]; present {
			t.Errorf("the empty catalog produced %q: %v", key, doc.Error)
		}
	}
}

func TestRenderCLIError_CarriesEveryDiagnosticField(t *testing.T) {
	doc, stderr := renderToDoc(t, &api.APIError{
		Code: "E0003_UNAUTHORIZED", Message: "Unauthorized", RequestID: "rid-1", HTTPStatus: 401,
		RawBody: []byte(`{"error":{"code":"E0003_UNAUTHORIZED","message":"Unauthorized"}}`),
	})

	if doc.Error["code"] != "E0003_UNAUTHORIZED" || doc.Error["message"] != "Unauthorized" ||
		doc.Error["request_id"] != "rid-1" || doc.Error["http_status"] != float64(401) {
		t.Errorf("error = %v", doc.Error)
	}
	if !strings.Contains(fmt.Sprint(doc.Error["raw"]), `"code":"E0003_UNAUTHORIZED"`) {
		t.Errorf("raw = %v, want the verbatim body", doc.Error["raw"])
	}
	if !strings.Contains(stderr, "Error: Unauthorized") || !strings.Contains(stderr, "request_id=rid-1") {
		t.Errorf("stderr = %q", stderr)
	}
}

// Anything that is not an *api.APIError still renders as an error object.
func TestRenderCLIError_PlainErrors(t *testing.T) {
	doc, _ := renderToDoc(t, errors.New("something odd"))
	if doc.Error["code"] != "ERROR" || doc.Error["message"] != "something odd" {
		t.Errorf("error = %v", doc.Error)
	}
}

// -----------------------------------------------------------------------------
// meta
// -----------------------------------------------------------------------------

func marshalMeta(t *testing.T, tenant, source string, raw json.RawMessage) string {
	t.Helper()
	b, err := json.Marshal(listMeta(tenant, source, raw))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// An absent meta must not become a meta of zeros: "no meta" and "a total of 0"
// are different answers, and the CLI invents neither.
func TestListMeta_AbsentOrNullMetaAddsNothing(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage(``), json.RawMessage(`null`), json.RawMessage(`{}`), json.RawMessage(`[1]`), json.RawMessage(`not json`)} {
		if got := marshalMeta(t, "", "", raw); got != `{}` {
			t.Errorf("listMeta(%q) = %s, want {}", raw, got)
		}
	}
}

func TestListMeta_PassesTheAPIsMetaThroughUntouched(t *testing.T) {
	const raw = `{"pagination":{"page":1,"pageSize":20,"total":0,"totalPages":0},"list_count":5}`
	got := marshalMeta(t, "acme", "config", json.RawMessage(raw))
	// A real zero survives; unknown keys survive; the API's field names stay.
	const want = `{"tenant":"acme","tenant_source":"config","list_count":5,"pagination":{"page":1,"pageSize":20,"total":0,"totalPages":0}}`
	if got != want {
		t.Errorf("meta =\n%s\nwant\n%s", got, want)
	}
}

func TestItemMeta(t *testing.T) {
	b, err := json.Marshal(itemMeta("acme", "flag"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != `{"tenant":"acme","tenant_source":"flag"}` {
		t.Errorf("itemMeta = %s", b)
	}
}
