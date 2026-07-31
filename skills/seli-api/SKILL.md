---
name: seli-api
description: >
  Drive the Seli platform through the installed `seli` CLI — the entry point for any
  Seli work an agent does for a tenant. Today the CLI exposes a connectivity/auth check
  and tenant-membership reads (`seli health`, `seli members list`, `seli members get`),
  plus local config and tenant management (`seli config ...`, `--tenant`) — use this
  whenever a request maps to those, even when the user never names the CLI or Seli
  itself, and in whatever language they ask: "is Seli reachable", "who's on this
  tenant", "look up this member by email", "set the default tenant". A request about any
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
3. Prefer the installation path that fits the detected machine. For example, when Homebrew is
   present, suggest `brew install --cask vtech-com/tap/seli`; the guide also covers GitHub
   Releases and Go-based installation with their platform-specific caveats.
4. Do not install software or change the machine merely because the binary is missing. Wait
   for the user's authorization, then verify the result with `seli version`.

After the version check succeeds, run the read-only authentication and connectivity probe:

```bash
seli health --tenant <code>
```

`X-Tenant-Code` is a required header on this endpoint — resolve a tenant (`--tenant`,
`SELI_TENANT`, or `config.default_tenant`) before calling it; a missing tenant fails locally
before any request is sent.

Interpret it by exit code and its stdout `error` object:

- Exit `0`: the API is reachable and the configured key was accepted. Continue.
- Exit `2`: the key is missing, malformed, or rejected. Tell the user to create or retrieve
  their key while signed in at <https://platform.seli.app>, and ask them to set
  `SELI_API_KEY` or store it via the supported config flow. Then retry `seli health`.
- Exit `5`: a validation error — most often no tenant resolved. Read the error message before
  assuming it's an auth problem.
- Exit `6`: connectivity failed. Diagnose DNS, network access, or the configured API URL; do not
  mislabel it as an authentication problem or ask the user to log in again.
- Exit `7`: rate limited. Back off before retrying.
- Any other non-zero exit: read the error object before recommending a recovery step.

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
Tenant isolation is enforced server-side at both the service layer and the database (Postgres
RLS), not merely by the CLI passing `X-Tenant-Code`; a resolved tenant is a routing choice for
*which* tenant's data you see, not a client-side access-control mechanism you are responsible
for policing yourself.

Within a resolved tenant, `owner` vs. `member` (`members.md`'s `level` field) is a **structural**
distinction, not a capability grant — an owner has full rights by default, a member is
deny-by-default unless a separately tenant-composed capability grants access. Do not infer what
a `member`-level user can or cannot do from `level` alone; let the API's own 403 respond to that.

After a call:

1. Parse the single JSON document on stdout.
2. Check for an `error` key before trusting `data`; partial results can contain both.
3. On writes, verify `meta.tenant` and the returned object rather than assuming the intended
   tenant or mutation was used.
4. Report the result without exposing credentials, raw auth headers, or unrelated tenant data.

## 4. Handle missing commands and failures

If an expected command is absent, re-check the root and relevant group help. A name you guessed
is not evidence that the capability is unavailable. The operation may live under a neighboring
group, or the installed CLI may be older than the current API.

If a command fails:

1. Read the stdout `error` object, especially `code` and `message`.
2. Read the leaf command help.
3. Use `--verbose` only when the redacted HTTP trace is actually needed.
4. Retry only when the error class supports it, such as a transient network or rate-limit
   response. Do not retry validation, auth, permission, conflict, or ambiguous write outcomes
   blindly.

A failed write does not prove the API lacks the capability. Do not invent destructive recovery
steps such as recreating resources, changing identifiers, or bypassing tenant handling.

## 5. Use the Public API specification sparingly

Public specification: <https://platform.seli.app/api/openapi>

This surface is a deliberately versioned, stable integration boundary — implemented as
`apps/platform`'s `/api/v1/*` Route Handlers in the Seli monorepo, a category Seli reserves
specifically for "versioned public/integration APIs" alongside OAuth callbacks and webhook
ingress, not for the app's own internal UI data-fetching. Treat a documented endpoint as a
durable contract, not an incidental implementation detail that might disappear.

For ordinary user operations, stop at CLI help and execute through `seli`. Open the spec only
when at least one of these is true:

- the user explicitly asks about the Public API contract or is building an integration;
- you are developing or reviewing this SDK;
- live help confirms a CLI gap and you need to distinguish "not exposed by this CLI" from
  "not present in the Public API";
- a version mismatch or inaccurate help/spec claim must be diagnosed.

When inside the SDK repository, prefer its versioned `api/openapi.json` for the release being
worked on; use the public URL to check the currently published contract. The spec defines API
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
