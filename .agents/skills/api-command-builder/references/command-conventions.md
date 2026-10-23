# Command conventions for the `seli` CLI

Everything here is drawn from the code in the tree (`cmd/root.go`, `cmd/helpers.go`,
`cmd/meta.go`, `cmd/health.go`, `cmd/members.go`, `cmd/auth.go`, `cmd/config.go`,
`internal/api/`, `internal/output/`, `internal/config/store.go`). Mirror the closest
existing command rather than inventing a shape: `cmd/members.go` for a tenant-scoped list/get
pair, `cmd/health.go` for a single-record call. If the tree contradicts this file, **the tree
wins** — re-read it and update the plan accordingly.

## 1. Naming and structure

- One file per command group: `cmd/<resource>.go` (e.g. `cmd/roles.go`).
- Group noun is plural and matches the API resource: `members`, `roles`.
- Subcommands are verbs the user already knows: `list`, `get`, `create`, `update`,
  `delete`. Map spec methods onto them — `GET /api/x` → `list`, `GET /api/x/{id}` →
  `get <id>`, `POST /api/x` → `create`, `PATCH|PUT /api/x/{id}` → `update <id>`.
- Register with `rootCmd.AddCommand(...)` in the file's own `init()`; `Execute()` runs
  after all `init()`s (see `cmd/root.go`).
- `SilenceUsage`/`SilenceErrors` are already set on `rootCmd` — don't re-declare per command.
- Logic lives in `internal/` (the HTTP calls and response types in `internal/api/`), not in
  `cmd/`: a command parses flags, calls the client, and prints.

## 2. Output — one envelope, always

`internal/output` owns the only output shape:

```go
output.Write(os.Stdout, data, meta)                          // { "data": …, "meta": … }
output.WritePartial(stdout, stderr, data, meta, errDetail)   // the same, plus an "error" key above it
```

Commands do not call these two for failures directly. A success calls `output.Write`; a
failure goes through the helpers in §3. Output is indented JSON on stdout. There is no
`--output` flag and no second shape (see `rootCmd.Long`); a nil slice prints `[]`, never `null`.

`meta` is an `output.Meta`, built by the helpers in `cmd/meta.go` — use them, do not hand-roll
it:

```go
itemMeta(tenant, source)           // single record (get, health): tenant + tenant_source
listMeta(tenant, source, rawMeta)  // list: the same, plus the API's own meta passed through
```

`tenant` and `source` come from `resolveTenant(cfg)` (§4); `meta.tenant_source` is `flag`,
`env` or `config`. A command that is not tenant-scoped uses an empty `output.Meta{}` and prints
`"meta": {}`.

**Pass the API's `meta` through; do not rename it.** Seli's list responses carry
`meta.pagination` as `{page, pageSize, total, totalPages}` — camelCase, as the API sends it.
There is no `limit`, no `has_more`. Keep the raw `meta` as a `json.RawMessage` on the page type
(`MembersPage.Meta`) and hand it to `listMeta`, so a key the API adds later reaches the caller
without a release. Decode `meta.pagination` separately, only for the paging loop. Likewise,
marshal the spec's response structs as they are: JSON tags match the spec's field names exactly
(see `api.Member`); there is no display-model layer.

Human-facing notes go to **stderr**, never stdout.

## 3. Errors and exit codes

There is one error type, `*api.APIError` (`code`, `message`, `request_id`, `HTTPStatus`, the
server's raw body). Use the existing helpers; do not `fmt.Errorf` + `os.Exit` at call sites:

```go
return handleErr(err)                                   // an error from buildClient or the client: render + exit
requireTenant(t)                                         // no tenant resolved → TENANT_REQUIRED, exit 5
failValidation("--status must be active or inactive")   // a local problem the CLI can see → exit 5
fail(api.ErrInvalidMemberID, "…", 400)                  // a local error with its own code
return failWithData(err, rows, listMeta(t, src, raw))   // fetched real rows, then failed: error key above the rows, non-zero exit
```

`handleErr` and `fail` print `{"error": {…}}` on stdout and a one-line summary on stderr, then
exit with `api.ExitCodeFor(err)`. The error object carries `code`, `message`, `http_status`,
`request_id`, `raw` (the server's body, verbatim) and, only if the catalog knows the code,
`meaning` / `next` / `capability_note`.

`api.ExitCodeFor` (`internal/api/errors.go`) resolves the code with `errors.As`, so a wrapped
error keeps its code:

| Exit | When |
|---|---|
| 0 | success |
| 1 | anything else, including HTTP 5xx |
| 2 | HTTP 401 (also `AUTH_MISSING_HEADER`, which `buildClient` raises locally) |
| 3 | HTTP 403 |
| 4 | HTTP 404 |
| 5 | HTTP 400 or 422 (also every local validation failure) |
| 6 | a network error: `NETWORK_ERROR`, a deadline, any `net.Error` |
| 7 | HTTP 429 |
| 8 | HTTP 409 |

A new command needs no change here. Add a code constant to `internal/api/errors.go` only for a
new **local** condition; entries in `internal/api/error_catalog.go` come only from the Seli
backend's own error code list — never guess a code or its meaning.

A bulk read (`--all`) that fails after the first page must keep what it fetched: the client
returns the partial page together with the error, and the command calls `failWithData`. Print
nothing before the outcome is known, so stdout stays exactly one JSON document.

## 4. Client, base URL, key and tenant

Every command starts the same way:

```go
client, cfg, err := buildClient()   // key + base URL; fails locally if either is missing
if err != nil { return handleErr(err) }
t, source := resolveTenant(cfg)     // --tenant > SELI_TENANT > default_tenant
if api.XRequiresTenant { requireTenant(t) }
```

- **Base URL** is the host only, resolved from `--api-url` > `SELI_API_URL` > the profile's
  `api_url` > the default baked in at build time (`internal/version.DefaultAPIURL`). All empty →
  `API_URL_MISSING` (exit 5); a non-HTTPS URL other than `localhost` / `127.0.0.1` →
  `INSECURE_BASE_URL` (exit 5). No key (`SELI_API_KEY`, else the profile's) →
  `AUTH_MISSING_HEADER` (exit 2). All three are diagnosed before a request is sent. `buildClient`
  does this; commands never re-implement it, and no source file, test or doc names a real host.
- **Paths are the spec's, verbatim, `/api` prefix included** (`/api/members`), and
  `*api.Client` methods in `internal/api/` pass them to `Do` / `DoWithHeaders`. Commands never
  build an `http.Request`. Escape a path parameter with `url.PathEscape`.
- **Tenant rule** is read from the spec per operation (`references/spec-reading.md §5`) and encoded
  as a named constant in `internal/api/errors.go` (`HealthRequiresTenant`,
  `MembersRequiresTenant`), so a later change doesn't touch command logic:
  - header param `required: true` → `requireTenant(t)` **before** the HTTP call; never let the
    server return 400 for a condition the CLI can see;
  - header param `required: false` → send `X-Tenant-Code` only when resolved; `Do` already omits
    the header for an empty tenant;
  - tenant in the request **body** → inject it there, not in the header.
  Today every operation in the spec requires the header.

## 5. Flags

- Path params are **positional args** (`seli members get <id>`), validated with
  `cobra.ExactArgs(1)` and a UUID check when the spec says `format: uuid` (a malformed id exits 5
  without calling the server).
- Query params are flags named for the CLI, not copied from the API: Seli's pagination is
  `page` and `pageSize`, exposed as `--page` (default 1) and `--page-size` (1–100, default 20;
  values above 100 are capped before the request), plus `--all` on list commands to
  auto-paginate until `page == meta.pagination.totalPages`. `--all` asks for pages of 100 unless
  `--page-size` is given, and ignores `--page`.
- Simple request bodies get one flag per field, kebab-cased from the JSON name
  (`displayName` → `--display-name`). Required spec fields → `MarkFlagRequired`.
- Complex or nested bodies get `--from-json <file|->` (stdin when `-`), which takes the
  whole request body. When both are supplied, `--from-json` wins and item flags are
  ignored — say so in the command's help text. This is the `gh`/`stripe`/`kubectl` pattern.
- Never add a flag for a parameter the spec doesn't declare.

## 6. HTTP client

`internal/api/client.go` already owns: the injected headers (`Authorization: Bearer`,
`X-Tenant-Code`, `User-Agent`, `X-Request-Id`), the 30-second timeout, the 16 MiB response cap,
`--verbose` tracing with the key always redacted, and mapping a 4xx/5xx to an `*APIError`. Read
the server's response headers off `*api.Response` if an endpoint's contract needs one; keep
`Response.RequestID` (the server's `X-Request-Id`, else the id the client generated), because it
is the only handle that correlates a failure with the server's logs.

A new endpoint adds a method on `*api.Client` in `internal/api/` (decode with `json.Unmarshal`;
a body that does not match the spec is `malformedBody(resp, err)`), and a request/response
struct whose JSON tags match the spec exactly. Optional spec fields are pointers; a field the
spec marks `required` but nullable is a pointer **without** `omitempty`.

## 7. Help text

Every command needs `Short`, and `Long` with PURPOSE, USAGE, FLAGS and OUTPUT sections — match
the density of `cmd/members.go` and `cmd/health.go`. AI agents read `--help`; treat it as part of
the deliverable, not decoration. State the tenant rule, the exit codes the command can produce,
and show the output envelope with an example (placeholders only: `acme`, `ssk_…`, `<host>`).

## 8. Tests

`internal/api/<resource>_test.go` — table-driven `httptest` tests per HTTP method: happy path,
4xx mapping, tenant header presence/absence, pagination. Command-level tests go in `cmd/`
(`cliRun` in `main_test.go` runs the real binary logic in a child process against a `newFakeAPI` and reads its real stdout, stderr and
exit code) and should cover the path requested, the tenant header, the envelope, and at least
one failure. `make test` runs with `-race`. Tests use `httptest` URLs only; no test names a real
host. See the `golang-pro` skill's `references/testing.md`.

## 9. Verification

```bash
make build && make test && make lint
```

All three must pass before a §8 row is marked DONE. `make lint` is `golangci-lint run ./...`.
