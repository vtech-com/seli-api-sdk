# API coverage gaps

Living backlog of Seli Public API (`/api/v1`) endpoints/actions the CLI cannot
yet wrap because the **server-side endpoint does not exist**, plus any endpoints
that exist but the CLI hasn't surfaced. Add to the SDK as endpoints land.

**Last assessed:** 2026-06-04, against the `seli` monorepo **`develop`** branch.

## How this was determined

The authoritative source is the actual route handlers, not a spec file:
`apps/platform/src/app/api/v1/**/route.ts` (which `export async function GET/POST/PATCH/PUT/DELETE`).

> ⚠️ **Do not trust the checked-in `apps/platform/src/lib/api/openapi.json` for gap analysis.** On 2026-06-04 it was stale: it listed `/pcms/{brands,categories,product-types,units}/{id}` as `PUT`-only, but the real handlers expose `GET,PATCH,PUT` (and the SDK already wraps get/update/replace from the **prod** openapi). The SDK syncs from prod (`make update-spec` ← `https://platform.seli.app/api/openapi`), which is accurate; the checked-in file is hand-maintained and drifts. **Recommendation to backend: regenerate `openapi.json` from the routes.**

## SDK vs. existing API: essentially complete

The CLI wraps every endpoint that currently exists, with one exception:

| Endpoint (exists on develop) | SDK command | Status |
|---|---|---|
| `GET /health` | — | ✅ **being added** (`seli health`) |
| `GET /pcms/products/{id}` | `seli products get <id>` | ✅ **landed on develop / pre-staged in SDK** — UUID-only; reconciles on `make update-spec` after prod deploy |
| `PATCH /mission/tasks/{id}` | `seli tasks update <id>` | ✅ **landed on develop / pre-staged in SDK** — UUID-only; reconciles on `make update-spec` after prod deploy |
| `GET /members/{id}` | `seli members get <id>` | ✅ **landed on develop / pre-staged in SDK** — UUID-only; reconciles on `make update-spec` after prod deploy |
| `GET /pcms/variants/{id}` | `seli variants get <id>` | ✅ **landed on develop / pre-staged in SDK** — UUID-only; reconciles on `make update-spec` after prod deploy |

Everything else under `/members`, `/mission/*`, `/pcms/*`, `/tenants` is wrapped.

## API-level gaps — endpoints that do NOT exist yet

"Needed" depends on the Tấm skill scope (defined elsewhere). The clearly-justified
ones for a work-management agent are task update/delete and product get.

| Priority | Missing action | Notes / status |
|---|---|---|
| High | `DELETE /mission/tasks/{id}` (task delete/cancel) | No way to delete/cancel a task via API. |
| Medium | `POST/PATCH/DELETE /mission/boards` (board create/update/delete) | Boards are **read-only** via the public API. No programmatic board creation. |
| Medium | Board lists management (the `lists`/columns inside a board) | `tasks create` accepts `board_list_id` but there's no endpoint to list/create board lists via the public API. |
| Medium | Members: invite, role change, remove | `list` and `get` (UUID) are exposed. Member management (invite/RBAC/positions) exists in product docs but not in public `/api/v1`. |
| Low (likely intentional) | `DELETE` on brands/categories/product-types/units/products | No resource exposes delete. Probably deliberate for reference data. |

## Addressing resources by human key (code), not UUID — API requirement

**Decision (2026-06-04):** users and Tấm hand off / reference work by a **human key**
(e.g. task `TASK-123`, `tenant_code` `acme`), not by UUID. Single-resource operations
(`get`, and future `update`/`delete`) should be addressable by that key.

**Chosen strategy: API-side exact lookup (Strategy A).** The SDK will NOT do client-side
"resolve key → UUID via search". Instead the API must expose exact lookup by the human
key, and the SDK wraps it. Rationale: search-resolve is fuzzy (ilike), can match 0/many,
and isn't uniform (boards/variants have no `q`). Until the endpoint exists, the only
interim option is `tasks list -q "TASK-123"` (fuzzy, returns a list) — not shipped as a
`get` behaviour.

**Current state:** every single-item `GET` is **UUID-only**; no resource supports exact
lookup by its human key. This blocks the whole pattern.

**Canonical human key per resource** (unique keys only — these are the ones in scope):

| Resource | Human key | Unique | API lookup needed |
|---|---|---|---|
| tenant | `tenant_code` | ✅ | already the handle (`--tenant`) — done |
| **task** | `code` (TASK-123) | ✅ per tenant | **priority** — exact lookup by code (backend already has `findOneTaskByCode`). Apply to get + future update/delete. |
| member | `email` | ✅ | exact lookup by email (members has no get-by-id at all yet) |
| variant | `barcode` / `sku` | ✅ | exact lookup (today only `barcode_prefix` search, no exact get) |
| product | `slug` / `sku` | slug ✅ | fold into the `GET /pcms/products/{id}` work being added — also accept slug/sku |

**Deferred (not in scope now):** ref-data (brand / category / product-type / unit) and
**board** only have `name`, which is **not guaranteed unique** — clean code-lookup there
needs the API to add a `code`/`slug` field first. Shelved until there's a concrete need.

**SDK follow-up (per resource, once the API endpoint exists):** `seli <resource> get <key>`
accepts the human key (and still UUID); same for update/delete. No SDK work until then.

**Note:** The four `get`-by-id commands added in this pre-stage (`products get`, `tasks update`,
`members get`, `variants get`) are **UUID-only** — they do not accept human keys. This section
remains pending until the API exposes exact lookup by code/key.

## When an endpoint lands

1. Backend implements the route handler in the monorepo + the endpoint reaches **prod**.
2. In this repo: `make update-spec` (pulls prod openapi).
3. The `cmd/openapi_path_coverage_test.go` guard will flag the new path → add the CLI command (mirror the closest existing command) and register it in the coverage guards.
