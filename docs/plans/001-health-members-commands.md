---
name: health-members-commands
status: done
goal: Ship `seli health`, `seli members list`, and `seli members get <id>` backed by a new internal/api HTTP client.
description: This builds on the existing cobra/viper scaffold, which has no API client or domain commands yet. It adds the first HTTP-calling client (internal/api), the Health and Member models, a --tenant global flag with resolution, and the three commands that wrap the only endpoints currently in api/openapi.json.
---

# Seli CLI SDK — Implementation Plan 001 (Health & Member commands)

> **Milestone:** 001
> **Mapping:** `project-context.md §5.4` (`seli health`, `seli members list`, `seli members get`)
> **Base:** scaffold-only commit `aa9cf47` on `main` — cobra/viper skeleton and config store exist; no `internal/api` package and no `--tenant` flag exist yet
> **Execution status:** see §8 (Execution tracking) at the bottom of this file.

---

## 1. Context

The repo today (`cmd/root.go`, `cmd/config.go`, `cmd/helpers.go`, `internal/config/store.go`) is a scaffold: cobra command tree, `~/.seli/config.json` profile store, and the `{ "data", "meta" }` envelope helpers. There is no `internal/api` package, no HTTP client, and no `--tenant` flag registered anywhere — `cmd/helpers.go` explicitly says the client doesn't exist yet. No command in this plan can be built without first adding that client.

`api/openapi.json` (`Platform API v v1`) currently declares exactly three operations: `GET /api/v1/health`, `GET /api/v1/members`, `GET /api/v1/members/{id}`. This plan wraps all three — it is the full current spec surface, not a partial slice of it.

`docs/project-context.md` describes a larger, aspirational command set (`auth`, `tenants`, `boards`, `tasks`, `products`) and a `§5.3` tenant table claiming `GET /members` has an **optional** tenant header. Per this skill's hard rule, `api/openapi.json` overrides that: both operations declare `X-Tenant-Code` via `#/components/parameters/XTenantCode` with `required: true`. `docs/api-coverage-gaps.md` itself warns the checked-in spec/docs pairing has drifted before, so this plan follows the spec, not the table, and flags the conflict below (§2.1).

---

## 2. Counter-review findings (pre-implementation audit)

### 2.1 Spec vs. docs conflicts (2 found — not spec defects; the spec is trusted, project-context.md is stale)

| # | Conflict | Resolution |
|---|--------|-----|
| 1 | `docs/project-context.md §5.3` lists `GET /members` tenant as **Optional**. `api/openapi.json` `components.parameters.XTenantCode` (used by both `/health` and `/members*`) sets `required: true`. | Follow the spec: both `seli health` and `seli members *` require a resolved tenant and `failValidation` before the HTTP call if none is resolved. `docs/project-context.md §5.3` is out of date for these two endpoints — not corrected in this plan (out of scope, docs-only). |
| 2 | `docs/project-context.md §5.1` documents the default base URL as `https://platform.seli.app/api/v1`. `api/openapi.json.servers[0].url` is `https://api.seli.vn/api/v1`. | Follow the spec's `servers[0].url` as the hardcoded fallback default (used only when no `--api-url`, `SELI_API_URL`, or profile `api_url` is set). Flagged in the verification summary for explicit user sign-off since it changes where the CLI points by default. |

No defects in the operations themselves (methods, param names, schemas) were found — both are otherwise internally consistent and match their declared response schemas.

### 2.2 Client/transport gap

`internal/api` does not exist. There is no base URL resolution, no header injection (`Authorization`, `X-Tenant-Code`, `User-Agent`, `X-Request-Id`), no response decoding, and no mapping from `ErrorEnvelope` to `CLIError`. All of this is new work in this plan, not an extension of existing code.

Fix: add `internal/api/client.go` (transport), `internal/api/models.go` (domain types), `internal/api/errors.go` (tenant-requirement constants + server-error wrapping).

### 2.3 Output types — none needed

The repo's actual output layer is `cmd/helpers.go`'s single `writeEnvelope(data any)` — there is no `internal/output` package, no `--output` flag, and `cmd/root.go`'s `Long` text explicitly says "There is no output flag, and no second shape to branch on." The template's `internal/output/types.go` / `formatter.go` step does not apply here; per this skill's own rule ("if the tree contradicts, the tree wins"), this plan does not create that package. `internal/api/models.go` structs are marshaled directly by `writeEnvelope`.

### 2.4 Flag design for write commands — not applicable

All three operations in scope are `GET`. There are no request bodies, so `--from-json` is not needed in this plan.

### 2.5 CLI tenant enforcement

Both `/api/v1/health` and `/api/v1/members*` require a resolved tenant (§2.1 #1). `internal/api/errors.go` gets two `bool` constants, `HealthRequiresTenant` and `MembersRequiresTenant` (both `true`), so a future endpoint that is genuinely optional doesn't have to touch command code to encode that — `cmd/health.go` and `cmd/members.go` check the constant and `failValidation` before calling the client if no tenant resolved.

`cmd/root.go` has no `--tenant` flag yet (`docs/project-context.md §4.1` describes it as a global flag, but it was never wired). This plan adds it as a `PersistentFlag` on `rootCmd`, bound through viper the same way `--api-url` already is, so resolution order is flag → `SELI_TENANT` env → `config.default_tenant` (`cmd/helpers.go` gets a new `resolveTenant()` helper implementing that order). `--no-tenant` / global mode is not added — no endpoint in this plan's scope has an optional tenant, so an inert flag would be misleading (§9).

---

## 3. Deliverable scope

### Files modified
- `cmd/root.go` — add `--tenant` persistent flag, bind through viper (mirrors `--api-url`)
- `cmd/helpers.go` — add `resolveTenant()` (flag → `SELI_TENANT` → `config.default_tenant`), extend `ExitCodeFor` for `401→2`, `403→3`, `429→7`, and a `NETWORK_ERROR` code → `6`
- `CHANGELOG.md` — add `[Unreleased]` entries for the new commands

### Files created
- `internal/api/client.go` — `Client` struct, base URL resolution (`--api-url` / `SELI_API_URL` / profile `api_url` / spec `servers[0].url` fallback), header injection, `Response` struct, `ErrorEnvelope` → `CLIError` mapping
- `internal/api/models.go` — `Health`, `Member`, `Pagination`, `MembersMeta` structs
- `internal/api/errors.go` — `HealthRequiresTenant`, `MembersRequiresTenant` constants; `ErrTenantRequired`, `ErrInvalidMemberID` local validation codes
- `internal/api/client_test.go` — httptest coverage for header injection, base URL resolution, error envelope mapping
- `internal/api/members_test.go` — httptest coverage for `ListMembers` / `GetMember` (pagination, status filter, 404)
- `internal/api/health_test.go` — httptest coverage for `GetHealth`
- `cmd/health.go` — `seli health`
- `cmd/members.go` — `seli members list`, `seli members get <id>`

---

## 4. API contract (authoritative — from `api/openapi.json`, `Platform API v v1`)

### Auth (both operations)
- `Authorization: Bearer {api_key}` — always required (`bearerAuth`, scheme `bearer`, `ssk_`-prefixed keys)
- `X-Tenant-Code: {tenant_code}` — **required** (`components.parameters.XTenantCode`, `required: true`); CLI rejects early (`ErrTenantRequired`, exit 5) if unresolved

### GET /api/v1/health
No path or query params beyond the tenant header.
Response 200: `{ "data": { "ok": boolean, "timestamp": string (date-time) } }` — `HealthResponse`, both fields required.
Responses declared: 401, 404, 429, 500 — all `ErrorEnvelope` (`{ "error": { "code": string, "message": string } }`).

### GET /api/v1/members
Query params (all optional, all typed `string` in the spec even though numeric):
- `page` — default `1`
- `pageSize` — default `20`, max `100`
- `status` — `active` or `inactive`

Response 200:
```
{
  "data": [ {
    id (uuid, required), userId (uuid, required),
    level (enum: owner|member, required), status (enum: active|inactive, required),
    createdAt (date-time, required),
    displayName, avatarUrl (uri), contactEmail (email), contactPhone,
    jobTitle, department, memberCode
      — all six above are in `required[]` but typed ["string","null"]: present key, nullable value
  } ],
  "meta": { "pagination": { "page", "pageSize", "total", "totalPages" (all required int) } }
}
```
Responses declared: 401, 404, 429, 500.

### GET /api/v1/members/{id}
Path param: `id` (uuid, required). Both "no such id" and "id in another tenant" return 404 (no existence leak, per spec description — no special CLI handling needed, it's just a normal 404).
Response 200: `{ "data": <Member, same shape as the list item> }`.
Responses declared: 401, 404, 429, 500.

---

## 5. New error codes (to add to `internal/api/errors.go`)

```go
const (
    ErrTenantRequired  = "TENANT_REQUIRED"   // no tenant resolved for an endpoint that requires one — local, exit 5
    ErrInvalidMemberID = "INVALID_MEMBER_ID" // <id> positional arg is not a well-formed UUID — local, exit 5
    ErrNetwork         = "NETWORK_ERROR"     // transport-level failure (DNS, timeout, connection refused) — exit 6
)

const (
    HealthRequiresTenant  = true
    MembersRequiresTenant = true
)
```

Server-returned errors are **not** given CLI-side constants — `ErrorEnvelope.error.code` (e.g. `E0003_UNAUTHORIZED`) and `.message` are passed straight through into the CLI's error envelope; only the HTTP status drives the exit code.

`ExitCodeFor` (`cmd/helpers.go`) needs these additions:
```go
if cliErr.Code == ErrNetwork { return 6 }
switch cliErr.HTTPStatus {
case 401: return 2
case 403: return 3
case 404: return 4
case 400: return 5
case 429: return 7
default: return 1
}
```
This is additive — existing `CONFIG_LOAD_ERROR`/`CONFIG_SAVE_ERROR` callers pass `HTTPStatus: 0` and a code other than `NETWORK_ERROR`, so they keep falling through to `default: return 1` unchanged.

---

## 6. Implementation details

### 6.1 `internal/api/client.go`

```go
package api

type Client struct {
    BaseURL    string // resolved: --api-url / SELI_API_URL / profile api_url / spec servers[0].url fallback
    APIKey     string
    HTTPClient *http.Client
}

// defaultBaseURL mirrors api/openapi.json's servers[0].url.
const defaultBaseURL = "https://api.seli.vn/api/v1"

type Response struct {
    StatusCode int
    Body       []byte
    RequestID  string // X-Request-Id the CLI itself generates and sends, echoed back for correlation
}

// Get issues a GET request. tenant == "" means the header is omitted; callers
// that need to enforce a required tenant do so before calling Get (cmd/ layer).
func (c *Client) Get(ctx context.Context, path string, query url.Values, tenant string) (*Response, error)

// DecodeError turns a non-2xx Response into a *cmd.CLIError-shaped error by
// unmarshaling ErrorEnvelope; returns ErrNetwork-coded error on transport failure.
```
`internal/api` cannot import `cmd` (would cycle); it returns a small `*api.Error{Code, Message, HTTPStatus}` that `cmd/health.go` / `cmd/members.go` convert to `*cmd.CLIError` at the call site — same pattern already used for config errors in `cmd/config.go`.

Headers set on every request: `Authorization: Bearer <APIKey>`, `X-Tenant-Code` (only when `tenant != ""`), `User-Agent: <version.UserAgent()>`, `X-Request-Id: <uuid>` (generated client-side, no new dependency — `crypto/rand` formatted as a UUIDv4 string).

### 6.2 `internal/api/models.go`

JSON tags match the spec's camelCase field names exactly (`avatarUrl`, not `avatar_url`) since `writeEnvelope` marshals these structs directly for CLI output — there is no separate display-model layer in this repo (§2.3).

```go
type Health struct {
    OK        bool   `json:"ok"`
    Timestamp string `json:"timestamp"`
}

type Member struct {
    ID           string  `json:"id"`
    UserID       string  `json:"userId"`
    Level        string  `json:"level"`  // owner | member
    Status       string  `json:"status"` // active | inactive
    CreatedAt    string  `json:"createdAt"`
    DisplayName  *string `json:"displayName"`
    AvatarURL    *string `json:"avatarUrl"`
    ContactEmail *string `json:"contactEmail"`
    ContactPhone *string `json:"contactPhone"`
    JobTitle     *string `json:"jobTitle"`
    Department   *string `json:"department"`
    MemberCode   *string `json:"memberCode"`
}

type Pagination struct {
    Page       int `json:"page"`
    PageSize   int `json:"pageSize"`
    Total      int `json:"total"`
    TotalPages int `json:"totalPages"`
}

type MembersMeta struct {
    Pagination Pagination `json:"pagination"`
}
```
The six nullable-but-required member fields are pointers so a `null` in the response round-trips as `nil`, not `""`.

### 6.3 `cmd/health.go` command surface

```
seli health   [--tenant <code>]
```
- Rejects early (`ErrTenantRequired`, exit 5) if no tenant resolves (`resolveTenant()`).
- On success: `writeEnvelope(health)` → `{ "data": { "ok": true, "timestamp": "..." }, "meta": {} }`.

### 6.4 `cmd/members.go` command surface

```
seli members list  [--tenant <code>] [--page N] [--page-size N] [--status active|inactive] [--all]
seli members get <id> [--tenant <code>]
```
- Both reject early if no tenant resolves.
- `list`: `--page` default 1, `--page-size` default 20 (client-side caps at 100, matching the spec's documented max, before sending), `--status` unvalidated pass-through (spec doesn't declare an enum constraint object, only an example — server validates). `--all` loops `page` from 1 while `page <= meta.pagination.totalPages`, concatenating `data`, and sets `meta` on the final envelope to the last page's pagination block.
- `get <id>`: positional arg, regex-validated as a UUID (`ErrInvalidMemberID`, exit 5) before the request — mirrors "path params are positional, UUID-checked when spec says `format: uuid`" (`references/command-conventions.md §5`). A well-formed-but-nonexistent UUID still reaches the server and surfaces its 404.

---

## 7. Implementation order

1. `cmd/root.go` — add `--tenant` persistent flag, viper binding
2. `cmd/helpers.go` — add `resolveTenant()`, extend `ExitCodeFor`
3. `internal/api/errors.go` — tenant-requirement constants, local error codes
4. `internal/api/client.go` — `Client`, `Response`, header injection, base URL resolution, error decoding
5. `internal/api/models.go` — `Health`, `Member`, `Pagination`, `MembersMeta`
6. `cmd/health.go` — `seli health`
7. `cmd/members.go` — `seli members list`, `seli members get <id>`
8. `internal/api/client_test.go`, `internal/api/health_test.go`, `internal/api/members_test.go` — httptest coverage
9. `CHANGELOG.md` update
10. Verify: `make build`, `make test`, `make lint`

---

## 8. Execution tracking

| # | Step | Status |
|---|------|--------|
| 1 | `cmd/root.go` — `--tenant` flag | DONE |
| 2 | `cmd/helpers.go` — `resolveTenant()` + `ExitCodeFor` | DONE |
| 3 | `internal/api/errors.go` | DONE |
| 4 | `internal/api/client.go` | DONE |
| 5 | `internal/api/models.go` | DONE |
| 6 | `cmd/health.go` | DONE |
| 7 | `cmd/members.go` | DONE |
| 8 | httptest coverage (3 files) | DONE |
| 9 | `CHANGELOG.md` update | DONE |
| 10 | Verify: build + test + lint | DONE |

**Outcome:** Shipped `internal/api` (client.go, models.go, errors.go) as the first HTTP-calling package in the repo, a `--tenant` global flag with flag→env→config resolution, and `seli health` / `seli members list` / `seli members get <id>` wrapping all three operations currently in `api/openapi.json`. Deviated from the template in one place: no `internal/output` package was added (§2.3) since the existing codebase has a single envelope shape with no `--output` flag — the tree's actual state overrode the template's default file layout. `make build`, `make test -race` (72.0% coverage on `internal/api`), and `make lint` (golangci-lint v2, 0 issues) all pass; manually verified the full request path against a local HTTP stub (`seli health --tenant acme` → 200 with the health envelope) and the tenant-required validation path (`seli health` with no tenant → exit 5, `TENANT_REQUIRED`).

---

## 9. Out of scope (deferred)

- `--no-tenant` / global mode — no endpoint in this plan's scope has an optional tenant; adding it now would be an inert flag (§2.5).
- `auth login` / `auth whoami`, `tenants list`, `boards *`, `tasks *`, `products *` — described in `docs/project-context.md §4.1`/`§5.4` but **not present in `api/openapi.json`** at all; not this plan's endpoints. No `docs/api-coverage-gaps.md` entry needed since that file already tracks them.
- Table/YAML output (`--output table|yaml`) — `docs/project-context.md §4.3` describes it, but the current codebase (`cmd/root.go`, `cmd/helpers.go`) has exactly one output shape and explicitly documents that there is no `--output` flag; introducing one is a scaffold-level decision beyond this plan (§2.3).
- `cmd/openapi_path_coverage_test.go` coverage guard — does not exist in the tree yet; per `references/spec-reading.md §7`, skipped since it isn't present to register against.
