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

`--tenant` is a global flag (`--tenant <code>` on `rootCmd`). Resolution order: `--tenant` flag,
then `SELI_TENANT` env, then the active profile's `default_tenant`. On every write, read
`meta.tenant` when the command's help documents it; a successful write can still land in a
different tenant than intended.

When a read spans tenants, do not attribute a returned record to a specific tenant unless the
response actually identifies it. Some endpoints — `health` and `members` today — require a
resolved tenant and reject the command locally (exit 5) if none is resolved; do not assume every
endpoint follows the same rule without checking its leaf help.

## Output contract

Every command writes one JSON document to stdout:

```json
{ "data": [], "meta": {} }
```

- Lists place an array at `.data`; single-resource reads and writes place an object there.
- Pagination and CLI-added context live under `.meta`.
- There is no output-mode flag in current builds; do not add `--output` or `-o` from memory.
- stdout is the machine-readable stream. stderr carries a short error summary and optional
  verbose tracing (`--verbose` / `-v`).

Parse stdout as JSON before acting. Avoid brittle text parsing and never strip an assumed prefix
line.

## Errors and partial results

A failure still writes JSON to stdout:

```json
{ "error": { "code": "...", "message": "..." } }
```

Branch on the process exit code or the `error.code` field, not on changing prose:

| Exit | Meaning |
|---|---|
| 0 | Success |
| 1 | General error (unexpected) |
| 2 | Auth error (401 — invalid/expired/revoked key) |
| 3 | Permission error (403) |
| 4 | Not found (404) |
| 5 | Validation error (400 — including a locally-detected problem like a missing tenant) |
| 6 | Network error (DNS, timeout, connection refused — safe to retry) |
| 7 | Rate limit (429 — back off before retrying) |

Surface `error.message` when escalating an unexplained server error; it is passed through from
the API's own error envelope unchanged.

## Pagination and counting

List calls are paginated unless their leaf help says otherwise.

- Read `meta.pagination.total` for a full count; never count only the visible `.data[]` page.
- Continue while `page < meta.pagination.totalPages` when a complete set is required, or use
  `--all` if the specific command exposes it.
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
