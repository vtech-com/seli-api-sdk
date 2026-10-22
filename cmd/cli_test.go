package cmd

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// okEnv is the environment of a configured caller: a key and where to send it.
func okEnv(api *fakeAPI) map[string]string {
	return map[string]string{"SELI_API_KEY": testAPIKey, "SELI_API_URL": api.url()}
}

func healthOK(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(healthBody)) }

// -----------------------------------------------------------------------------
// Paths and headers
// -----------------------------------------------------------------------------

// The base URL is the host only and the paths are the spec's, prefix included.
// The old base ended in /api/v1, which 404s on every call.
func TestHealth_RequestsAPIHealth(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, healthOK)

	res := cliRun(t, "", okEnv(api), "health", "--tenant", "acme")

	if res.exit != 0 {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
	}
	reqs := api.requests()
	if len(reqs) != 1 {
		t.Fatalf("%d requests, want 1", len(reqs))
	}
	if reqs[0].method != http.MethodGet || reqs[0].path != "/api/health" {
		t.Errorf("request = %s %s, want GET /api/health", reqs[0].method, reqs[0].path)
	}
	if got := reqs[0].header.Get("Authorization"); got != "Bearer "+testAPIKey {
		t.Errorf("Authorization = %q", got)
	}
	if got := reqs[0].header.Get("X-Tenant-Code"); got != "acme" {
		t.Errorf("X-Tenant-Code = %q", got)
	}

	doc := res.doc(t)
	if data := obj(t, doc, "data"); data["ok"] != true {
		t.Errorf("data = %v", data)
	}
	if _, present := doc["error"]; present {
		t.Error("a successful command printed an error key")
	}
}

func TestMembersList_RequestsAPIMembersAndPassesPaginationThrough(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"a1b2c3d4-e5f6-7890-abcd-ef1234567890","userId":"u","level":"member","status":"active","createdAt":"t","displayName":null,"avatarUrl":null,"contactEmail":null,"contactPhone":null,"jobTitle":null,"department":null,"memberCode":null}],` +
			`"meta":{"pagination":{"page":1,"pageSize":2,"total":41,"totalPages":21}}}`))
	})

	res := cliRun(t, "", okEnv(api), "members", "list", "--tenant", "acme", "--page-size", "2", "--status", "active")

	if res.exit != 0 {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
	}
	reqs := api.requests()
	if len(reqs) != 1 || reqs[0].path != "/api/members" {
		t.Fatalf("requests = %+v, want one GET /api/members", reqs)
	}
	if q := reqs[0].rawQuery; !strings.Contains(q, "pageSize=2") || !strings.Contains(q, "status=active") || !strings.Contains(q, "page=1") {
		t.Errorf("query = %q", q)
	}

	meta := obj(t, res.doc(t), "meta")
	// Seli's own pagination object, with Seli's own field names — not
	// renamed into some other API's page / limit / has_more.
	pg := obj(t, meta, "pagination")
	if pg["page"] != float64(1) || pg["pageSize"] != float64(2) || pg["total"] != float64(41) || pg["totalPages"] != float64(21) {
		t.Errorf("meta.pagination = %v", pg)
	}
	for _, renamed := range []string{"limit", "has_more", "page", "total"} {
		if _, present := meta[renamed]; present {
			t.Errorf("meta.%s was invented beside the API's own pagination object", renamed)
		}
	}
	if meta["tenant"] != "acme" || meta["tenant_source"] != "flag" {
		t.Errorf("meta = %v", meta)
	}
}

func TestMembersGet_RequestsTheMemberPath(t *testing.T) {
	t.Parallel()
	const id = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"` + id + `","userId":"u","level":"owner","status":"active","createdAt":"t","displayName":null,"avatarUrl":null,"contactEmail":null,"contactPhone":null,"jobTitle":null,"department":null,"memberCode":null}}`))
	})

	res := cliRun(t, "", okEnv(api), "members", "get", id, "--tenant", "acme")

	if res.exit != 0 {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
	}
	if reqs := api.requests(); len(reqs) != 1 || reqs[0].path != "/api/members/"+id {
		t.Errorf("requests = %+v", reqs)
	}
	doc := res.doc(t)
	if obj(t, doc, "data")["id"] != id {
		t.Errorf("data = %v", doc["data"])
	}
	if meta := obj(t, doc, "meta"); meta["tenant"] != "acme" || meta["tenant_source"] != "flag" {
		t.Errorf("meta = %v", meta)
	}
}

func TestMembersGet_MalformedIDExitsFiveWithoutARequest(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, healthOK)

	res := cliRun(t, "", okEnv(api), "members", "get", "not-a-uuid", "--tenant", "acme")

	if res.exit != 5 {
		t.Errorf("exit = %d, want 5", res.exit)
	}
	if n := len(api.requests()); n != 0 {
		t.Errorf("%d requests sent for an id the CLI can see is malformed", n)
	}
	if code := obj(t, res.doc(t), "error")["code"]; code != "INVALID_MEMBER_ID" {
		t.Errorf("error.code = %v", code)
	}
}

// -----------------------------------------------------------------------------
// Tenant in meta
// -----------------------------------------------------------------------------

func TestMeta_NamesTheTenantAndWhereItCameFrom(t *testing.T) {
	t.Parallel()
	const cfg = `{"version":1,"active_profile":"default","profiles":{"default":{"api_key":"","api_url":"","default_tenant":"from-config"}}}`

	tests := []struct {
		name       string
		env        map[string]string
		args       []string
		wantTenant string
		wantSource string
	}{
		{"flag", nil, []string{"--tenant", "from-flag"}, "from-flag", "flag"},
		{"flag beats env and config", map[string]string{"SELI_TENANT": "from-env"}, []string{"--tenant", "from-flag"}, "from-flag", "flag"},
		{"env", map[string]string{"SELI_TENANT": "from-env"}, nil, "from-env", "env"},
		{"config", nil, nil, "from-config", "config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, healthOK)
			home := t.TempDir()
			writeConfigFile(t, home, cfg)
			env := okEnv(api)
			for k, v := range tt.env {
				env[k] = v
			}

			res := cliRun(t, home, env, append([]string{"health"}, tt.args...)...)

			if res.exit != 0 {
				t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
			}
			meta := obj(t, res.doc(t), "meta")
			if meta["tenant"] != tt.wantTenant || meta["tenant_source"] != tt.wantSource {
				t.Errorf("meta = %v, want tenant %q from %q", meta, tt.wantTenant, tt.wantSource)
			}
			if got := api.requests()[0].header.Get("X-Tenant-Code"); got != tt.wantTenant {
				t.Errorf("X-Tenant-Code = %q, want %q", got, tt.wantTenant)
			}
		})
	}
}

func TestNoTenantResolvedExitsFiveWithoutARequest(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, healthOK)

	for _, args := range [][]string{{"health"}, {"members", "list"}, {"members", "get", "a1b2c3d4-e5f6-7890-abcd-ef1234567890"}} {
		res := cliRun(t, "", okEnv(api), args...)
		if res.exit != 5 {
			t.Errorf("%v: exit = %d, want 5", args, res.exit)
		}
		if code := obj(t, res.doc(t), "error")["code"]; code != "TENANT_REQUIRED" {
			t.Errorf("%v: error.code = %v", args, code)
		}
	}
	if n := len(api.requests()); n != 0 {
		t.Errorf("%d requests sent with no tenant", n)
	}
}

// -----------------------------------------------------------------------------
// Local diagnoses: nothing is sent
// -----------------------------------------------------------------------------

func TestMissingKey_ExitsTwoAndSendsNothing(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, healthOK)

	// SELI_API_KEY empty, no config file: the state of a fresh install.
	res := cliRun(t, "", map[string]string{"SELI_API_URL": api.url(), "SELI_API_KEY": ""}, "health", "--tenant", "acme")

	if res.exit != 2 {
		t.Fatalf("exit = %d, want 2\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
	}
	if n := len(api.requests()); n != 0 {
		t.Errorf("%d requests reached the server; a missing key must be diagnosed locally", n)
	}
	e := obj(t, res.doc(t), "error")
	if e["code"] != "AUTH_MISSING_HEADER" {
		t.Errorf("error.code = %v, want AUTH_MISSING_HEADER", e["code"])
	}
	msg, _ := e["message"].(string)
	if !strings.Contains(msg, "SELI_API_KEY") || !strings.Contains(msg, "seli auth login") {
		t.Errorf("message does not name SELI_API_KEY and `seli auth login`: %q", msg)
	}
	if _, present := e["request_id"]; present {
		t.Error("a local diagnosis never left the machine and has no request id")
	}
	if !strings.Contains(res.stderr, "Error:") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestMissingKey_AlsoForAProfileWithoutOne(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, healthOK)
	home := t.TempDir()
	writeConfigFile(t, home, `{"version":1,"active_profile":"default","profiles":{"default":{"api_key":"","default_tenant":"acme"}}}`)

	res := cliRun(t, home, map[string]string{"SELI_API_URL": api.url()}, "members", "list")

	if res.exit != 2 || len(api.requests()) != 0 {
		t.Errorf("exit = %d, requests = %d; want 2 and 0", res.exit, len(api.requests()))
	}
}

func TestPlainHTTPToARemoteHostIsRefused(t *testing.T) {
	t.Parallel()
	res := cliRun(t, "", map[string]string{"SELI_API_KEY": testAPIKey, "SELI_API_URL": "http://api.example.test"}, "health", "--tenant", "acme")

	if res.exit != 5 {
		t.Fatalf("exit = %d, want 5\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
	}
	if code := obj(t, res.doc(t), "error")["code"]; code != "INSECURE_BASE_URL" {
		t.Errorf("error.code = %v, want INSECURE_BASE_URL", code)
	}
	if strings.Contains(res.stdout+res.stderr, testAPIKey) {
		t.Error("the refusal printed the API key")
	}
}

func TestPlainHTTPIsRefusedFromTheFlagToo(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, healthOK)
	res := cliRun(t, "", map[string]string{"SELI_API_KEY": testAPIKey, "SELI_API_URL": api.url()},
		"health", "--tenant", "acme", "--api-url", "http://api.example.test")

	if res.exit != 5 || len(api.requests()) != 0 {
		t.Errorf("exit = %d, requests = %d; --api-url must win, and be refused", res.exit, len(api.requests()))
	}
}

func TestEveryLayerEmptyIsAPIURLMissing(t *testing.T) {
	t.Parallel()
	res := cliRun(t, "", map[string]string{"SELI_API_KEY": testAPIKey}, "health", "--tenant", "acme")

	if res.exit != 5 {
		t.Fatalf("exit = %d, want 5\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
	}
	e := obj(t, res.doc(t), "error")
	if e["code"] != "API_URL_MISSING" {
		t.Errorf("error.code = %v, want API_URL_MISSING", e["code"])
	}
	if msg, _ := e["message"].(string); !strings.Contains(msg, "SELI_API_URL") {
		t.Errorf("message does not name SELI_API_URL: %q", msg)
	}
}

// --api-url > SELI_API_URL > profile api_url > the default baked into the build.
func TestBaseURLPrecedence(t *testing.T) {
	t.Parallel()
	flagAPI, envAPI, profileAPI, buildAPI := newFakeAPI(t, healthOK), newFakeAPI(t, healthOK), newFakeAPI(t, healthOK), newFakeAPI(t, healthOK)

	profileCfg := func() string {
		home := t.TempDir()
		writeConfigFile(t, home, `{"version":1,"active_profile":"default","profiles":{"default":{"api_key":"","api_url":"`+profileAPI.url()+`"}}}`)
		return home
	}

	run := func(env map[string]string, extra ...string) {
		env["SELI_API_KEY"] = testAPIKey
		env[childDefaultEnv] = buildAPI.url()
		res := cliRun(t, profileCfg(), env, append([]string{"health", "--tenant", "acme"}, extra...)...)
		if res.exit != 0 {
			t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
		}
	}

	run(map[string]string{"SELI_API_URL": envAPI.url()}, "--api-url", flagAPI.url())
	run(map[string]string{"SELI_API_URL": envAPI.url()})
	run(map[string]string{})
	// The build default is the last resort: with no profile api_url it is used.
	res := cliRun(t, "", map[string]string{"SELI_API_KEY": testAPIKey, childDefaultEnv: buildAPI.url()}, "health", "--tenant", "acme")
	if res.exit != 0 {
		t.Fatalf("build default: exit = %d\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
	}

	for name, c := range map[string]struct {
		api  *fakeAPI
		want int
	}{"flag": {flagAPI, 1}, "env": {envAPI, 1}, "profile": {profileAPI, 1}, "build default": {buildAPI, 1}} {
		if got := len(c.api.requests()); got != c.want {
			t.Errorf("%s host got %d requests, want %d", name, got, c.want)
		}
	}
}

func TestVersionPrintsTheBakedInDefaultAPIURL(t *testing.T) {
	t.Parallel()
	res := cliRun(t, "", map[string]string{childDefaultEnv: "https://seli.example.test"}, "version")
	if res.exit != 0 {
		t.Fatalf("exit = %d: %s", res.exit, res.stderr)
	}
	if got := obj(t, res.doc(t), "data")["default_api_url"]; got != "https://seli.example.test" {
		t.Errorf("data.default_api_url = %v", got)
	}

	res = cliRun(t, "", nil, "version")
	if got, present := obj(t, res.doc(t), "data")["default_api_url"]; !present || got != "" {
		t.Errorf("a build with no default must print default_api_url: \"\", got %v (present %v)", got, present)
	}
}

// -----------------------------------------------------------------------------
// Errors from the server
// -----------------------------------------------------------------------------

func TestServerErrorsMapToExitCodesAndCarryTheRequestID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status   int
		code     string
		wantExit int
	}{
		{401, "E0003_UNAUTHORIZED", 2},
		{403, "E0004_FORBIDDEN", 3},
		{404, "E0404_NOT_FOUND", 4},
		{400, "E0400_BAD_REQUEST", 5},
		{422, "E0422_INVALID", 5},
		{429, "E0429_RATE_LIMITED", 7},
		{409, "E0409_CONFLICT", 8},
		{500, "E0000_UNKNOWN", 1},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.status), func(t *testing.T) {
			api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = fmt.Fprintf(w, `{"error":{"code":%q,"message":"m"}}`, tt.code)
			})

			res := cliRun(t, "", okEnv(api), "health", "--tenant", "acme")

			if res.exit != tt.wantExit {
				t.Errorf("exit = %d, want %d", res.exit, tt.wantExit)
			}
			sentID := api.requests()[0].header.Get("X-Request-Id")
			e := obj(t, res.doc(t), "error")
			if e["code"] != tt.code || e["http_status"] != float64(tt.status) {
				t.Errorf("error = %v", e)
			}
			// The server echoes no id, so the client's is the only handle.
			if rid, _ := e["request_id"].(string); !uuidV4.MatchString(rid) || rid != sentID {
				t.Errorf("error.request_id = %q, want the id sent on the wire (%q)", rid, sentID)
			}
			if raw, _ := e["raw"].(string); !strings.Contains(raw, tt.code) {
				t.Errorf("error.raw = %q, want the verbatim server body", raw)
			}
			if !strings.Contains(res.stderr, "request_id="+sentID) {
				t.Errorf("stderr does not quote the request id: %q", res.stderr)
			}
		})
	}
}

func TestNetworkFailureExitsSix(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, healthOK)
	env := okEnv(api)
	api.srv.Close() // nothing listens at the address any more

	res := cliRun(t, "", env, "health", "--tenant", "acme")

	if res.exit != 6 {
		t.Fatalf("exit = %d, want 6\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
	}
	e := obj(t, res.doc(t), "error")
	if e["code"] != "NETWORK_ERROR" {
		t.Errorf("error.code = %v", e["code"])
	}
	if rid, _ := e["request_id"].(string); !uuidV4.MatchString(rid) {
		t.Errorf("a transport failure must still name the request id, got %q", rid)
	}
	if strings.Contains(res.stdout+res.stderr, testAPIKey) {
		t.Error("the error printed the API key")
	}
}

// -----------------------------------------------------------------------------
// --verbose
// -----------------------------------------------------------------------------

func TestVerboseTracesToStderrAndRedactsTheKey(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, healthOK)

	res := cliRun(t, "", okEnv(api), "-v", "health", "--tenant", "acme")

	if res.exit != 0 {
		t.Fatalf("exit = %d: %s", res.exit, res.stderr)
	}
	sentID := api.requests()[0].header.Get("X-Request-Id")
	for _, want := range []string{"> GET " + api.url() + "/api/health", "> Authorization: Bearer ssk_...[REDACTED]", "> X-Tenant-Code: acme", "> X-Request-Id: " + sentID, "< HTTP 200"} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("stderr is missing %q:\n%s", want, res.stderr)
		}
	}
	if strings.Contains(res.stderr, testAPIKey) || strings.Contains(res.stderr, "SECRET") {
		t.Errorf("the trace contains the API key:\n%s", res.stderr)
	}
	// The trace is stderr's; stdout stays exactly one parseable document.
	res.doc(t)

	quiet := cliRun(t, "", okEnv(api), "health", "--tenant", "acme")
	if strings.Contains(quiet.stderr, "Authorization") {
		t.Errorf("a trace was printed without --verbose:\n%s", quiet.stderr)
	}
}

// -----------------------------------------------------------------------------
// members list --all
// -----------------------------------------------------------------------------

// pagedMembers serves `pages` pages of `perPage` members and fails page failOn
// (0: never) with failStatus. The member ids carry the page, so a test can tell
// which rows came back.
func pagedMembers(t *testing.T, pages, perPage, failOn, failStatus int) *fakeAPI {
	t.Helper()
	return newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == failOn {
			w.WriteHeader(failStatus)
			_, _ = w.Write([]byte(`{"error":{"code":"E0000_UNKNOWN","message":"boom"}}`))
			return
		}
		rows := make([]string, 0, perPage)
		for i := 0; i < perPage; i++ {
			rows = append(rows, fmt.Sprintf(`{"id":"p%d-%d","userId":"u","level":"member","status":"active","createdAt":"t","displayName":null,"avatarUrl":null,"contactEmail":null,"contactPhone":null,"jobTitle":null,"department":null,"memberCode":null}`, page, i))
		}
		_, _ = fmt.Fprintf(w, `{"data":[%s],"meta":{"pagination":{"page":%d,"pageSize":%d,"total":%d,"totalPages":%d}}}`,
			strings.Join(rows, ","), page, perPage, pages*perPage, pages)
	})
}

func TestMembersListAll_FetchesEveryPage(t *testing.T) {
	t.Parallel()
	api := pagedMembers(t, 3, 2, 0, 0)

	res := cliRun(t, "", okEnv(api), "members", "list", "--all", "--tenant", "acme", "--page-size", "2")

	if res.exit != 0 {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
	}
	doc := res.doc(t)
	if rows, _ := doc["data"].([]any); len(rows) != 6 {
		t.Errorf("data has %d rows, want 6", len(rows))
	}
	if _, present := doc["error"]; present {
		t.Error("a complete sweep carries an error key")
	}
	if pg := obj(t, doc, "meta", "pagination"); pg["page"] != float64(3) || pg["totalPages"] != float64(3) {
		t.Errorf("meta.pagination = %v, want the last page's", pg)
	}
	if n := len(api.requests()); n != 3 {
		t.Errorf("%d requests, want 3: one per page", n)
	}
}

// Fewer round trips: --all asks for the API's largest page unless told otherwise.
func TestMembersListAll_DefaultsToTheLargestPage(t *testing.T) {
	t.Parallel()
	api := pagedMembers(t, 1, 1, 0, 0)

	if res := cliRun(t, "", okEnv(api), "members", "list", "--all", "--tenant", "acme"); res.exit != 0 {
		t.Fatalf("exit = %d: %s", res.exit, res.stderr)
	}
	if q := api.requests()[0].rawQuery; !strings.Contains(q, "pageSize=100") {
		t.Errorf("query = %q, want pageSize=100", q)
	}

	api2 := pagedMembers(t, 1, 1, 0, 0)
	if res := cliRun(t, "", okEnv(api2), "members", "list", "--tenant", "acme"); res.exit != 0 {
		t.Fatalf("exit = %d: %s", res.exit, res.stderr)
	}
	if q := api2.requests()[0].rawQuery; !strings.Contains(q, "pageSize=20") {
		t.Errorf("without --all: query = %q, want pageSize=20", q)
	}

	api3 := pagedMembers(t, 1, 1, 0, 0)
	if res := cliRun(t, "", okEnv(api3), "members", "list", "--tenant", "acme", "--page-size", "500"); res.exit != 0 {
		t.Fatalf("exit = %d: %s", res.exit, res.stderr)
	}
	if q := api3.requests()[0].rawQuery; !strings.Contains(q, "pageSize=100") {
		t.Errorf("--page-size 500: query = %q, want it capped at 100", q)
	}
}

// A sweep that dies on page 3 still holds two real pages. They are printed with
// the error above them, in one document, and the exit code is non-zero.
func TestMembersListAll_PartialResultOnAFailureAfterPageOne(t *testing.T) {
	t.Parallel()
	api := pagedMembers(t, 4, 2, 3, http.StatusTooManyRequests)

	res := cliRun(t, "", okEnv(api), "members", "list", "--all", "--tenant", "acme", "--page-size", "2")

	if res.exit != 7 {
		t.Fatalf("exit = %d, want 7 (rate limit)\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
	}
	doc := res.doc(t) // exactly one document on stdout
	e := obj(t, doc, "error")
	if e["code"] != "E0000_UNKNOWN" || e["http_status"] != float64(429) {
		t.Errorf("error = %v", e)
	}
	if rid, _ := e["request_id"].(string); !uuidV4.MatchString(rid) {
		t.Errorf("error.request_id = %q", rid)
	}
	rows, _ := doc["data"].([]any)
	if len(rows) != 4 {
		t.Fatalf("data has %d rows, want the 4 fetched before the failure", len(rows))
	}
	if first := rows[0].(map[string]any)["id"]; first != "p1-0" {
		t.Errorf("first row = %v", first)
	}
	meta := obj(t, doc, "meta")
	if meta["tenant"] != "acme" || meta["tenant_source"] != "flag" {
		t.Errorf("meta = %v", meta)
	}
	if pg := obj(t, meta, "pagination"); pg["page"] != float64(2) {
		t.Errorf("meta.pagination = %v, want the last page that succeeded", pg)
	}
	if !strings.Contains(res.stderr, "Error:") {
		t.Errorf("stderr = %q", res.stderr)
	}
	if n := len(api.requests()); n != 3 {
		t.Errorf("%d requests, want 3: nothing is asked for after the failure", n)
	}
}

func TestMembersListAll_FailureOnPageOneIsAPlainError(t *testing.T) {
	t.Parallel()
	api := pagedMembers(t, 4, 2, 1, http.StatusForbidden)

	res := cliRun(t, "", okEnv(api), "members", "list", "--all", "--tenant", "acme")

	if res.exit != 3 {
		t.Fatalf("exit = %d, want 3", res.exit)
	}
	doc := res.doc(t)
	if _, present := doc["data"]; present {
		t.Errorf("nothing was fetched, but the document has a data key: %v", doc)
	}
	if obj(t, doc, "error")["code"] != "E0000_UNKNOWN" {
		t.Errorf("error = %v", doc["error"])
	}
}

// -----------------------------------------------------------------------------
// Help text
// -----------------------------------------------------------------------------

func TestRootHelpHasGettingStarted(t *testing.T) {
	t.Parallel()
	res := cliRun(t, "", nil, "--help")
	for _, want := range []string{"GETTING STARTED", "seli auth login --key ssk_", "seli health", "AUTH_MISSING_HEADER", "SELI_API_KEY", "API_URL_MISSING"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("root help is missing %q:\n%s", want, res.stdout)
		}
	}
}

// config set used to point at a command that did not exist and at a base URL
// with a stale path.
func TestConfigSetHelpExampleIsAHostOnlyPlaceholder(t *testing.T) {
	t.Parallel()
	res := cliRun(t, "", nil, "config", "set", "--help")
	if strings.Contains(res.stdout, "/v1") || strings.Contains(res.stdout, "seli.app") {
		t.Errorf("config set help carries a stale URL:\n%s", res.stdout)
	}
	if !strings.Contains(res.stdout, "seli auth login") {
		t.Errorf("config set help does not point at `seli auth login`:\n%s", res.stdout)
	}
}
