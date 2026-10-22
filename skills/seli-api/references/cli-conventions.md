# Stable `seli` CLI conventions

Read this after `../SKILL.md`. These are cross-command contracts that are useful even as the
command surface expands. For the exact command, flags, accepted values, and response fields,
always read the installed binary's help.

## Command and flag discovery

See `../SKILL.md` section 2 for the three-level discovery workflow (`seli --help`,
`seli <group> --help`, `seli <group> <command> --help`).

Do not translate API query-parameter names directly into CLI flags. The CLI may expose a clearer
name, validate the value, combine several API fields, or intentionally omit a parameter.

## Queries and filters

- Use `--query` or `-q` for text search only when the leaf help exposes it.
- Use typed filter flags such as `--status`, `--priority`, or date bounds exactly as documented
  by that command. Do not invent a generic `--filter` or append raw `?filters[...]` syntax.
- Quote values containing spaces or shell metacharacters. For example, pass `--status "To-Do"`
  and quote human text.
- Preserve the documented casing and format for enums, UUIDs, dates, timestamps, comma-separated
  lists, and literal sentinel values such as `null`.
- Combine filters only when the command help permits their combination. Respect mutually
  exclusive forms shown in `USAGE`.

Search narrows a result; it does not automatically make a page complete. Inspect pagination
metadata before concluding that a record does not exist.

## Tenant resolution

`--tenant` is a global flag. Resolution order: `--tenant` flag, then `SELI_TENANT` env, then the
active profile's `default_tenant` (`seli config set-default-tenant <code>`). Every tenant-scoped
command reports the result in `meta.tenant` and where it came from in `meta.tenant_source`
(`flag`, `env`, or `config`), so a silently defaulted tenant is visible; read both, especially on
a write — a successful write can still land in a different tenant than intended.

When a read spans tenants, do not attribute a returned record to a specific tenant unless the
response actually identifies it. Some endpoints — `health` and `members` today — require a
resolved tenant and reject the command locally (exit 5, `TENANT_REQUIRED`) if none is resolved;
do not assume every endpoint follows the same rule without checking its leaf help.

## Base URL and API key

Both are configuration, resolved before any request, and a missing one fails locally.

- Host (`https://<host>`, no path): `--api-url`, then `SELI_API_URL`, then the profile's
  `api_url`, then the default baked into the build (`seli version` prints it as
  `default_api_url`; empty for a build without one). None set: exit 5, `API_URL_MISSING`. A
  non-`https` URL other than `localhost`/`127.0.0.1`: exit 5, `INSECURE_BASE_URL`.
- Key: `SELI_API_KEY`, which overrides the key stored by `seli auth login --key <ssk_…>`. None:
  exit 2, `AUTH_MISSING_HEADER`, nothing sent. Only the user stores or supplies a key; never
  pass one on their behalf or echo it.

## Output contract

Every command writes one JSON document to stdout:

```json
{ "data": [], "meta": {} }
```

- Lists place an array at `.data`; single-resource reads and writes place an object there.
- `.meta` holds the tenant fields above, plus the API's own `meta` passed through untouched
  (a list's `pagination`). Commands that touch no tenant print `"meta": {}`; some `config`
  commands print nothing on success.
- There is no output-mode flag in current builds; do not add `--output` or `-o` from memory.
- stdout is the machine-readable stream. stderr carries a short error summary and optional
  verbose tracing (`--verbose` / `-v`).

Parse stdout as JSON before acting. Avoid brittle text parsing and never strip an assumed prefix
line.

## Errors and partial results

A failure still writes JSON to stdout, with an `error` key:

```json
{ "error": { "code": "...", "message": "...", "http_status": 404, "request_id": "...", "raw": "..." } }
```

Fields: `code` and `message` always; `http_status` when there is one; `request_id` (the id the CLI
sent, or the server's) when a request left the machine; `raw` (the server's body, verbatim) when
the server answered; `meaning`, `next` and `capability_note` only if present, as a hint. Branch on
the process exit code or `error.code`, not on changing prose:

| Exit | Meaning |
|---|---|
| 0 | Success |
| 1 | General error — anything not below, including an HTTP 5xx and an unreadable config file |
| 2 | Auth error (401, or no key configured: `AUTH_MISSING_HEADER`) |
| 3 | Permission error (403) |
| 4 | Not found (404; for `config`, the profile does not exist yet) |
| 5 | Validation error (400 or 422, or a locally-detected problem: no tenant, no API URL, an `http://` URL, a malformed id or key, an unknown command) |
| 6 | Network error (DNS, timeout after 30 seconds, connection refused, unreadable response — safe to retry) |
| 7 | Rate limit (429 — back off before retrying) |
| 8 | Conflict (409) |

Surface `error.message` and `error.raw` when escalating an unexplained server error; they are the
API's own words.

A list that failed part-way (`--all`) prints one document with both an `error` key and the `data`
fetched so far, and exits non-zero. Treat those rows as a prefix: report the error, say how many
rows you hold, and do not present the set as complete.

## Pagination and counting

List calls are paginated unless their leaf help says otherwise.

- Read `meta.pagination.total` for a full count; never count only the visible `.data[]` page.
- Continue while `page < meta.pagination.totalPages` when a complete set is required, or use
  `--all` if the specific command exposes it (with `--all`, `meta.pagination` describes the last
  page fetched, and the page size defaults to the maximum).
- Use `--page`, `--page-size`, or `--all` only if the specific command exposes them.

Prefer a server-side search or typed filter over fetching an entire large collection. Fall back
to exhaustive paging only when the answer truly requires it.

To answer "how many X are there?", request a 1-row page and read the total instead of fetching
everything:

```bash
seli <group> list --tenant <code> --page-size 1 | jq '.meta.pagination.total'
```

## Structured JSON input

When leaf help exposes `--from-json <path|->`, use it for nested objects, arrays, or payloads
that are clearer and safer as JSON than as many flags.

- `-` means stdin; a path means a file.
- Validate the JSON shape before sending it.
- Include only fields the user intends to change.
- Omitted and explicit `null` fields can have different meanings; read the leaf help.
- Check whether `--from-json` is mutually exclusive with field flags or takes precedence over
  them. This differs between commands.

Do not place credentials in JSON payloads, examples, logs, or shell history.

## Write safety

- Treat an explicit user request containing the intended resource, mutation, and tenant as
  authorization for that scoped write. Ask when any of those are materially ambiguous.
- Search or read the target before an update when identity or current state matters.
- Do not silently change stable identifiers or expand a single requested write into bulk work.
- After success, verify `.data` and `meta.tenant` (when the command surfaces it).
- After an ambiguous network failure, read the target before retrying; the server may have
  committed the first attempt even though the response was lost.
