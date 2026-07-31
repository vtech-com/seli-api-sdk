---
name: <milestone>-phase-<n>-<slug>
status: not_started | in_progress | done | blocked
goal: One sentence — the concrete outcome this milestone delivers (endpoints wired, defects fixed, capability surfaced).
description: Two sentences — what this builds on, and what it adds (commands, spec fixes, client changes, output types).
---

# <Project> CLI SDK — Implementation Plan <milestone> (<Short Title>)

> **Milestone:** <milestone>
> **Mapping:** `project-context.md §<n> Phase <n>` (<subcommands covered>)
> **Base:** v<x.y.z> on `<branch>` — all Phase <prev> code is present and passing CI
> **Execution status:** see §8 (Execution tracking) at the bottom of this file.

---

## 1. Context

<What the previous phase shipped. Why this scope was deferred until now — blocking
defects, unmerged backend APIs, missing spec.>

<What changed to unblock it: backend version, branch/PR, spec availability. State the
authoritative source explicitly, since everything in §4 is derived from it.>

---

## 2. Counter-review findings (pre-implementation audit)

The following issues were identified before writing any code. All are addressed in this plan.

### 2.1 <Spec> defects (<N> total — original list said <M>)

| # | Defect | Fix |
|---|--------|-----|
| 1 | **<Short name>**: <what is wrong, with the exact path/field> | <the exact change to make> |
| 2 | **<Short name>**: <…> | <…> |

<Any additional smaller spec gaps that don't warrant a table row — state them and
note whether the API already accepts the field.>

### 2.2 <Client/transport gap>

<What the current code does, what it discards or gets wrong, and which feature that
breaks downstream.>

Fix: <the specific struct/function change.>

### 2.3 Missing output types

`internal/output/types.go` has no `<Resource>` display type. `internal/output/formatter.go`
has no `"<resource>"` renderer entry. Both are required before `cmd/<resource>.go`
can call `output.Render`.

### 2.4 Flag design for write commands

<Which payload fields are too complex for individual flags.> Pattern adopted:

- Simple case: individual flags (`--name`, `--sku`, …)
- Complex case: `--from-json <file>` or `--from-json -` (stdin) accepting the full request body

This mirrors the pattern used by `gh`, `stripe`, and `kubectl` for complex payloads.
<State roughly what share of real use cases each path covers, and any command where
`--from-json` is the *only* viable flag.>

### 2.5 CLI tenant enforcement

<Which subcommands target tenant-scoped paths.> Per §5.3 rule #2, the CLI must reject
early if no tenant is resolved. The constant `api.<Domain>RequiresTenant` already encodes
this; `cmd/<resource>.go` must check it using the same pattern as `<existingCmd>`.

---

## 3. Deliverable scope

### Files modified
- `api/openapi.json` — fix <N> defects as described in §2.1
- `internal/api/client.go` — <change>
- `internal/api/models.go` — add `<Type1>`, `<Type2>`, …
- `internal/api/errors.go` — add <domain> error codes as named constants
- `internal/output/types.go` — add `<Resource>` display type
- `internal/output/formatter.go` — add `"<resource>"` renderer
- `CHANGELOG.md` — add `[Unreleased]` entries

### Files created
- `cmd/<resource>.go` — `<resource> list`, `<resource> create`, …
- `internal/api/<resource>_test.go` — unit tests for <resource> HTTP methods (httptest)

---

## 4. API contract (authoritative — from <source> v<version>)

### Auth
- `Authorization: Bearer {api_key}` — always required
- `X-Tenant-Code: {tenant_code}` — required for all `<path prefix>` endpoints (CLI rejects early if absent)

### GET <path>
Query: `page` (int, default 1), `limit` (1–100, default 20), `<filter>` (<format>)
Response header: `<X-Header>` — <how the caller uses it>
Response 200: `{ data: <Resource>[], meta: { page, limit, total, has_more } }`

### POST <path>
Request body: `<field>` (required, <constraint>), `<field>` (<enum|default>), …
- <Behavioural rule: what happens when only the minimum is supplied>
- <Behavioural rule: which fields are conditionally required together>
Response 201: `{ data: <Resource> }`

### PUT <path>/{id}
Request body: all fields optional, at least 1 required. Fields: `<…>`
Response 200: `{ data: <Resource> }`

### PUT <path>/{id}/<sub>
Request body: array (max <N> items). Each item: `<id field>` (present → UPDATE, absent → CREATE — `<field>` required for CREATE), …
Response 200: `{ data: <Resource> }`

---

## 5. New error codes (to add to `internal/api/errors.go`)

```go
const (
    Err<Name>NotFound     = "<CODE>" // <meaning>
    Err<Name>CreateFailed = "<CODE>" // <meaning>
    Err<Name>Validation1  = "<CODE>" // validation: <field>
    ErrTenantDenied       = "<CODE>" // tenant access denied
    ErrAuthRequired       = "<CODE>" // auth required
)
```

These are informational constants — `ExitCodeFor()` already handles the HTTP status
codes (400→5, 401→2, 403→3, 404→4) correctly. No change to `ExitCodeFor` needed.

---

## 6. Implementation details

### 6.1 `internal/api/client.go` change

```go
type Response struct {
    StatusCode int
    Body       []byte
    RequestID  string
    // <new field> // <source header; behaviour when absent>
}
```

Populate: `<assignment>` <where in the call flow>.

### 6.2 `internal/api/models.go` additions

```go
// <Type> is <what it maps to and for which endpoint>.
type <Type> struct {
    // Required fields: plain types.
    // Optional fields: pointers with `,omitempty` so unset != zero value.
}
```

### 6.3 `internal/output/types.go` addition

```go
// <Resource> is the display model for a <domain> <resource>.
type <Resource> struct {
    ID         string `json:"id"`
    Name       string `json:"name"`
    Status     string `json:"status"`
    TenantCode string `json:"tenant_code,omitempty"`
    // Derived/flattened fields — note where the value comes from.
}
```

### 6.4 `cmd/<resource>.go` command surface

```
<bin> <resource> list     [--tenant <code>] [--page N] [--limit N] [--all]
<bin> <resource> create   --<required> <str> [--<optional> <str>] …
                          [--from-json <file|->]  (when provided, item flags are ignored)
<bin> <resource> update   --id <uuid> [--<field> <str>] …
<bin> <resource> <sub>    --<parent>-id <uuid> --from-json <file|->
```

All commands:
- Reject early if no tenant resolved (`api.<Domain>RequiresTenant`)
- Pass tenant via `<header or body field>` — state which, and why

---

## 7. Implementation order

1. Fix `api/openapi.json` — <N> defects
2. Extend `internal/api/client.go`
3. Extend `internal/api/models.go`
4. Extend `internal/api/errors.go`
5. Extend `internal/output/types.go`
6. Extend `internal/output/formatter.go`
7. Create `cmd/<resource>.go` — all <N> subcommands
8. Write `internal/api/<resource>_test.go` — httptest unit tests
9. Update `CHANGELOG.md`
10. Verify: `make build`, `make test`, `make lint`

---

## 8. Execution tracking

Mirror §7 one-for-one. Update the Status column as work lands — this is the file's
source of truth for progress. Use `TODO` / `IN PROGRESS` / `DONE` / `BLOCKED`, and
add a `NOTE:` where a step could not be completed as written.

| # | Step | Status |
|---|------|--------|
| 1 | Fix `api/openapi.json` (<N> defects) | TODO |
| 2 | `internal/api/client.go` | TODO |
| 3 | `internal/api/models.go` | TODO |
| 4 | `internal/api/errors.go` | TODO |
| 5 | `internal/output/types.go` | TODO |
| 6 | `internal/output/formatter.go` | TODO |
| 7 | `cmd/<resource>.go` | TODO |
| 8 | `internal/api/<resource>_test.go` | TODO |
| 9 | `CHANGELOG.md` update | TODO |
| 10 | Verify: build + test + lint | TODO |

---

## 9. Out of scope (deferred to <next milestone>+)

- `<resource> <subcommand>` (<why — endpoint not in spec, etc.>)
- <Feature listed in the phase but not in this milestone>
- <Packaging/distribution work>
- <Later-phase command groups>
