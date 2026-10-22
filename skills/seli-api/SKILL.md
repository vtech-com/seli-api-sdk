---
name: seli-api
description: >
  Drive the Seli platform through the installed `seli` CLI — the entry point for any
  Seli work an agent does for a tenant. Today the CLI exposes a connectivity/auth check
  and tenant-membership reads (`seli health`, `seli members list`, `seli members get`),
  plus API key and local config management (`seli auth login`, `seli config ...`,
  `--tenant`) — use this whenever a request maps to those, even when the user never
  names the CLI or Seli itself, and in whatever language they ask: "is Seli reachable",
  "who's on this tenant", "look up this member by email", "set the default tenant",
  "log in to Seli". A request about any
  other Seli domain still belongs here: use this skill to discover the live command
  surface (`seli --help`) and report an accurate gap instead of guessing raw API calls or
  assuming the capability doesn't exist anywhere. Also the reference for how the CLI
  itself works: detecting the install, tenant resolution, exit codes, JSON output, and
  self-diagnosing a misbehaving command.
---

# Seli CLI operator

Use `seli` as the normal interface to Seli. The installed binary is self-documenting and
the Public API will continue to grow, so this skill teaches a discovery workflow rather than
duplicating the command tree or API reference.

## 1. Preflight the environment and authentication

Start each new environment or session by resolving the executable with the shell's native lookup
(`command -v seli` in a POSIX shell or `Get-Command seli` in PowerShell), then run:

```bash
seli version
```

Use the resolved executable consistently for version, help, preflight, and execution; mixing help
from one build with commands sent through another produces false flag and capability errors.

If the shell cannot find `seli`:

1. Inspect the OS, architecture, shell, and available package managers using read-only checks.
2. Tell the user that the Seli CLI is not installed and recommend the official installation
   guide: <https://github.com/vtech-com/seli-api-sdk#installation>.
3. Prefer the installation path that fits the detected machine: the guide covers the install
   script, GitHub Releases, and building from source, with their platform-specific caveats.
4. Do not install software or change the machine merely because the binary is missing. Wait
   for the user's authorization, then verify the result with `seli version`.

After the version check succeeds, run the read-only authentication and connectivity probe:

```bash
seli health --tenant <code>
```

`health` needs three things: an API host, an API key, and a tenant. `X-Tenant-Code` is a required
header on this endpoint, so resolve a tenant (`--tenant`, `SELI_TENANT`, or the stored
`default_tenant`) first. The host is configuration: `seli version` shows the default baked into
this build as `default_api_url` (empty means the host must be given at runtime). Every one of
these is checked locally before a request is sent.

Interpret the result by exit code and the stdout `error` object (`error.code` tells apart causes
that share an exit code):

- Exit `0`: the API is reachable and the configured key was accepted for the tenant. Continue.
- Exit `2`: `AUTH_MISSING_HEADER` means no key is configured and nothing was sent; any other
  `error.code` means the server rejected the key. Tell the user to create or retrieve their key in
  the Seli web app and store it themselves, in their own terminal, with `seli auth login --key
  <key>`, or to export `SELI_API_KEY` (preferred in CI and on shared machines, since a `--key`
  argument stays in shell history). Then retry `seli health`.
- Exit `3`: the key is valid but cannot act for that tenant. Check the tenant code with the user.
- Exit `5`: a local or validation error. `TENANT_REQUIRED`: no tenant resolved; ask the user for
  the tenant code, then pass `--tenant` or run `seli config set-default-tenant <code>`.
  `API_URL_MISSING`: this build has no default host; ask the user for the API host (it is not a
  secret) and set `SELI_API_URL`, pass `--api-url`, or run `seli config set api_url https://<host>`.
  `INSECURE_BASE_URL`: the URL is not `https`; use `https`. Read the message before assuming it is
  an auth problem.
- Exit `6`: connectivity failed, including a request that timed out after 30 seconds. Diagnose
  DNS, network access, or the configured API URL; do not mislabel it as an authentication problem
  or ask the user to log in again.
- Exit `7`: rate limited. Back off before retrying.
- Any other non-zero exit, including exit `1` (which also covers an HTTP 5xx from the server):
  read the error object, including `request_id` and `raw`, before recommending a recovery step.

Authentication secrets belong to the user. Never ask them to paste an API key into chat, inspect
credential files or environment variables to recover it, print it, place it in examples, or expose
it through shell tracing.

## 2. Discover the live command surface

Once `seli version` succeeds, use built-in help progressively:

```bash
seli --help
seli <group> --help
seli <group> <command> --help
```

Read only the group and command relevant to the user's intent. A runnable command page includes
its purpose, usage, flags, constraints, examples, and output. Do not reconstruct a command from
memory, guess a flag, or treat this skill as a command inventory.

The installed help wins for CLI spelling and behavior because it ships with that exact build.
If help and this skill disagree, follow help and report the mismatch for the skill maintainer.

## 3. Plan, execute, and verify

Before a call:

1. Translate the request into one mechanical Seli intent.
2. Discover the group and command from help.
3. Read the leaf command's help before constructing flags or JSON.
4. Resolve the tenant. If a write's tenant is ambiguous, ask before writing. Do not add a
   redundant confirmation when the user already authorized the exact resource, change, and
   tenant.
5. Use the conventions in [`references/cli-conventions.md`](./references/cli-conventions.md).

A user can belong to multiple tenants at once — there is no single "current" tenant to assume.
A resolved tenant is a routing choice for *which* tenant's data you see, not a client-side
access-control mechanism you are responsible for policing yourself. When the tenant came from a
stored default (`meta.tenant_source` is `config`) rather than from the user's request, say which
tenant you used.

Do not infer what a `member`-level user (`members.md`'s `level` field) can or cannot do from
`level` alone; let the API's own 403 respond to that.

After a call:

1. Parse the single JSON document on stdout.
2. Check for an `error` key before trusting `data`; a partial result (`members list --all`
   that failed after the first page) carries both, with a non-zero exit. Rows beside an `error`
   key are a prefix of the answer, never the whole of it.
3. Verify `meta.tenant` (and `meta.tenant_source`: `flag`, `env`, or `config`) rather than
   assuming the intended tenant was used; on writes, verify the returned object too.
4. Report the result without exposing credentials, raw auth headers, or unrelated tenant data.

## 4. Handle missing commands and failures

If an expected command is absent, re-check the root and relevant group help. A name you guessed
is not evidence that the capability is unavailable. The operation may live under a neighboring
group, or the installed CLI may be older than the current API.

If a command fails:

1. Read the stdout `error` object: `code`, `message`, and `http_status`; `raw` is the server's own
   response body, and `request_id` is the handle to quote when escalating.
2. Read the leaf command help.
3. Use `--verbose` only when the redacted HTTP trace is actually needed.
4. Retry only when the error class supports it, such as a transient network or rate-limit
   response. Do not retry validation, auth, permission, conflict, or ambiguous write outcomes
   blindly.

A failed write does not prove the API lacks the capability. Do not invent destructive recovery
steps such as recreating resources, changing identifiers, or bypassing tenant handling.

## 5. Use the Public API specification sparingly

Public specification: `<host>/api/openapi` (public, no key), where `<host>` is the API host from
`seli version` (`default_api_url`), `SELI_API_URL`, or the stored `api_url`. Its paths all begin
with `/api/` — not `/api/v1/`. Treat a documented endpoint as a durable contract, not an
incidental implementation detail that might disappear.

For ordinary user operations, stop at CLI help and execute through `seli`. Open the spec only
when at least one of these is true:

- the user explicitly asks about the Public API contract or is building an integration;
- you are developing or reviewing this SDK;
- live help confirms a CLI gap and you need to distinguish "not exposed by this CLI" from
  "not present in the Public API";
- a version mismatch or inaccurate help/spec claim must be diagnosed.

When inside the SDK repository, prefer its versioned `api/openapi.json` for the release being
worked on; fetch the public URL to check the currently published contract. The spec defines API
paths and schemas, not the installed CLI's flag spelling. Finding an endpoint in the spec does
not authorize bypassing the CLI for a routine tenant operation. Report the gap or recommend an
upgrade. Call raw HTTP only when the user's task is explicitly to build or test a Public API
integration and that work is separately authorized.

## 6. Load special-case guidance only when needed

Most operations need no bundled domain guide: leaf help is sufficient.

For a member lookup, or for resolving an exact person's identity for some other write, load
[`references/domains/members.md`](./references/domains/members.md) after reading leaf help. Do
not load a domain reference for unrelated listing or search work.

## Scope boundary

This skill covers Seli CLI operation and Public API discovery. It does not define an
organization's product-code, barcode, naming, approval, or data-governance policies. Apply those
from the user's own policy or a separate organization-specific skill.
