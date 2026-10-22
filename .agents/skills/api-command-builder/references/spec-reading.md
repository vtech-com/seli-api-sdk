# Reading `api/openapi.json`

The spec is large. Never read it whole — query it. All recipes below assume you are at
the repo root and `jq` is available.

## 1. Does the spec exist, and what version does it target?

```bash
test -f api/openapi.json && jq -r '.info | "\(.title) v\(.version)"' api/openapi.json
```

If the file is absent: stop and ask (see SKILL.md Phase 1). `make update-spec` is the
documented sync path: it fetches `$SELI_SPEC_URL` (default `$SELI_DEFAULT_API_URL/api/openapi`,
so one of the two must be set; the host is configuration, see `.env.example`) and pretty-prints
it. Never read the base URL from the spec's `servers[0].url`; it has been wrong.

## 2. Find the endpoints that match the requirement

```bash
# every path + method, one per line
jq -r '.paths | to_entries[] | .key as $p | .value | to_entries[]
       | select(.key | IN("get","post","put","patch","delete"))
       | "\(.key|ascii_upcase)\t\($p)\t\(.value.summary // "")"' api/openapi.json

# narrow by keyword (resource name from the requirement)
jq -r '.paths | keys[]' api/openapi.json | grep -i roles
```

Match on the **resource noun** in the requirement, then widen: a "members" ask usually
also means `/api/members/{id}`, and sub-resources like `/api/members/{id}/permissions`.
Every spec path starts with `/api/`; the CLI uses it verbatim.

## 3. Pull one operation's full contract

```bash
jq '.paths["/api/members"].get' api/openapi.json
```

Extract, in this order, and transcribe each into plan §4:

- **Path params** — name, type, format (`uuid` matters: it decides whether the command
  validates the arg before the request).
- **Query params** — name, type, default, enum, min/max. Seli's pagination is `page` +
  `pageSize`, and a list response carries `meta.pagination` `{page, pageSize, total, totalPages}`
  (check the operation's own parameters and response schema; do not assume).
- **Request body** — `requestBody.content["application/json"].schema`; note `required[]`
  exactly, and which fields are conditionally required together.
- **Responses** — the 2xx schema, plus which 4xx codes the endpoint declares.
- **Response headers** — note any header the contract tells callers to use.

## 4. Resolve `$ref`s

Some schemas are refs (`ErrorEnvelope`, `XTenantCode`), but the success bodies in this spec
are mostly **inline** (`jq '.components.schemas | keys'` lists what is shared), so read the
operation's own response schema first and resolve a ref only where one appears:

```bash
jq '.paths["/api/members/{id}"].get.responses["200"].content["application/json"].schema' api/openapi.json

jq -r '.paths["/api/members"].get.responses["200"].content["application/json"].schema
       | .. | .["$ref"]? | select(.)' api/openapi.json   # empty: nothing to resolve

jq '.components.schemas.ErrorEnvelope' api/openapi.json
```

Field-by-field listing with required-ness, which is what plan §4 and the Go struct need:

```bash
jq -r '.paths["/api/members/{id}"].get.responses["200"].content["application/json"].schema.properties.data as $s
       | ($s.required // []) as $req
       | $s.properties | to_entries[]
       | "\(.key)\t\(.value.type // .value["$ref"])\t\(if (.key|IN($req[])) then "required" else "optional" end)"' \
      api/openapi.json
```

Optional request fields become **pointer fields with `,omitempty`** in Go so unset ≠ zero value.
A response field that is `required` but typed `["string","null"]` is a pointer **without**
`omitempty`: the key is always present and the value may be null (see `api.Member`).

## 5. Determine the tenant rule (per endpoint, from the spec)

This is the single most error-prone part. Check, in order:

```bash
# 1. an explicit X-Tenant-Code header param on the operation (usually a $ref to
#    components.parameters.XTenantCode)
jq -r '.paths["/api/members"].get.parameters[]' api/openapi.json
jq '.components.parameters.XTenantCode' api/openapi.json

# 2. a tenant field in the request body (none today; check any POST/PUT/PATCH you wrap)
jq -r '.paths["/api/delivery/{code}/label/100x150"].post.requestBody // "no body"' api/openapi.json
```

Then apply the rules in `docs/project-context.md §5.3` (today every operation in the spec
requires the header, including `GET /api/health`):

| Spec says | CLI behaviour |
|---|---|
| header param, `required: true` | reject **before** the request if no tenant resolved |
| body field `tenant_code` in `required[]` | inject into the body, **not** the header; reject early if unresolved |
| header param, `required: false` | send `X-Tenant-Code` only when resolved; omit entirely otherwise (never empty string) |
| no tenant param | send nothing |

Per-endpoint tenant rules live in `internal/api/` as named constants, not inline in
`cmd/` (project-context §5.3 rule 4).

## 6. Look for spec defects before you plan

The checked-in spec has drifted before. Cross-check and record findings in plan §2.1:

- Method mismatch — spec lists `PUT` only where handlers expose `GET,PATCH,PUT`.
- Missing `required[]` entries the API actually enforces.
- `type: string` where the API returns a number/enum, or a missing `format: uuid`.
- Endpoints present in `docs/project-context.md` but absent from the spec (or vice versa).

A defect that blocks the work becomes an explicit step 1 in §7 ("Fix `api/openapi.json`");
a cosmetic one is still listed in §2.1 with the exact fix. If the endpoint does not exist
server-side at all, it is a §9 out-of-scope item plus a `docs/api-coverage-gaps.md` entry —
**do not** wrap a URL that isn't in the spec.

## 7. Coverage guard

`docs/api-coverage-gaps.md` notes a `cmd/openapi_path_coverage_test.go` guard that flags
spec paths with no CLI command. If it exists in the tree, register new paths there as part
of the implementation; if it doesn't yet, ignore it.
