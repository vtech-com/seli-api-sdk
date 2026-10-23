# Seli CLI SDK

[![CI](https://github.com/vtech-com/seli-api-sdk/actions/workflows/ci.yml/badge.svg)](https://github.com/vtech-com/seli-api-sdk/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/vtech-com/seli-api-sdk)](https://github.com/vtech-com/seli-api-sdk/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A CLI tool and Go SDK for the Seli platform — built for third-party AI agents, automation scripts, and developers who need a scriptable interface to Seli's Public API.

```bash
seli auth login --key ssk_…          # store your API key (once)
seli config set-default-tenant acme  # name a default tenant (once)
seli health                          # confirm the API accepts both
seli members list
```

## Installation

**Install script (Linux and macOS).** It downloads the release archive for your platform,
verifies it against the release's `checksums.txt`, and installs `seli` into `/usr/local/bin`
(it uses `sudo` only if that directory is not writable):

```bash
curl -fsSL https://raw.githubusercontent.com/vtech-com/seli-api-sdk/main/scripts/install.sh | sh
```

Set `SELI_VERSION` (for example `v0.1.0`) to pin a version and `SELI_BIN_DIR` to install
elsewhere. Re-run the script to upgrade.

**Homebrew (macOS only).** The tap ships a **cask**, and casks are macOS-only — `brew install
--cask` does not work on Linuxbrew. On Linux, use the install script or the `.deb` from the release.

```bash
brew install --cask vtech-com/tap/seli
brew upgrade --cask seli    # later, to upgrade
```

**From source.** Needs Go (see `go.mod` for the version). The binary has to be told where the
API lives, because a bare build has no default host — see [Configuration](#configuration):

```bash
git clone https://github.com/vtech-com/seli-api-sdk && cd seli-api-sdk
SELI_DEFAULT_API_URL=https://<host> make build   # ./dist/seli, with that host baked in
```

`go install github.com/vtech-com/seli-api-sdk@latest` also works, but it produces a binary
named `seli-api-sdk` with **no** default host, so every call fails with `API_URL_MISSING` (exit
5) until you set `SELI_API_URL` (or pass `--api-url`, or run `seli config set api_url
https://<host>`). Release builds bake the host in; a build you make yourself does not unless you
set `SELI_DEFAULT_API_URL` as above.

## Quickstart

```bash
seli auth login --key ssk_…          # 1. store your API key in ~/.seli/config.json (mode 600)
seli config set-default-tenant acme  # 2. name the tenant to act on when --tenant is omitted
seli health                          # 3. confirm the API accepts the key and the tenant
```

`auth login` and `config set-default-tenant` only write the local config file; they do not call
the API, so `seli health` is the first real check. If you cannot or would rather not store the
key (a shared machine, CI), skip step 1 and export `SELI_API_KEY`; export `SELI_TENANT` to skip
step 2. A key passed with `--key` stays in your shell history, which is why the environment
variable is the better route there.

A successful `seli health`:

```json
{
  "data": { "ok": true, "timestamp": "2026-07-31T09:30:00.000Z" },
  "meta": { "tenant": "acme", "tenant_source": "config" }
}
```

Run `seli --help` for the command groups, and `seli <group> <command> --help` for each
command's flags, constraints, and output; the help ships inside the binary and describes that
build exactly.

## Configuration

**API host.** The host is configuration, not part of the CLI, and every path the CLI requests
begins with `/api/` (`/api/health`, `/api/members`). Give the host only — `https://<host>`, no
path. The first of these that is set wins:

1. `--api-url`
2. `SELI_API_URL`
3. `api_url` in the active profile (`seli config set api_url https://<host>`; needs a profile, so run `seli auth login` first)
4. the default baked into the binary at build time (`SELI_DEFAULT_API_URL`, set for release
   builds); `seli version` prints it as `default_api_url` (empty when the build has none)

If none is set the CLI fails locally with exit 5 and `API_URL_MISSING`. A plain `http://` URL
is refused (exit 5, `INSECURE_BASE_URL`) unless the host is `localhost` or `127.0.0.1`.

**API key.** `SELI_API_KEY` if set, otherwise the key stored by `seli auth login`. With neither,
the CLI fails locally with exit 2 and `AUTH_MISSING_HEADER` and sends nothing. `seli auth logout`
removes the stored key (and leaves `SELI_API_KEY` alone).

**Tenant.** Every command that reaches the API today needs a tenant (the spec declares
`X-Tenant-Code` as required). The first of these that is set wins: `--tenant`, `SELI_TENANT`,
the profile's `default_tenant` (`seli config set-default-tenant`). If none is set the command
exits 5 with `TENANT_REQUIRED` before sending anything.

`.env.example` lists every variable with placeholders.

## Output

Every command prints one JSON document on stdout, whether it read a record, read a page of them,
or failed:

```json
{ "data": …, "meta": { … } }
```

`data` is an array for a list and an object for a single record. `meta` carries:

| Key | When | Meaning |
|---|---|---|
| `tenant` | tenant-scoped commands | the tenant code the call resolved to |
| `tenant_source` | tenant-scoped commands | where it came from: `flag`, `env` (`SELI_TENANT`) or `config` (`default_tenant`) |
| anything else | as the API sends it | the API's own `meta`, passed through untouched — `members list` keeps its `pagination` object `{page, pageSize, total, totalPages}` |

Commands that are not tenant-scoped (`version`, `auth`, `config`) print `"meta": {}`; `config set`,
`config set-default-tenant`, and `config unset-default-tenant` print nothing on success.

### Errors

A failure prints `{"error": {…}}` on stdout (so a caller that parses stdout always gets JSON) and
a one-line summary on stderr, and exits non-zero. The `error` object has:

| Field | Meaning |
|---|---|
| `code` | machine-readable code: the API's own, a `HTTP_<status>` fallback, or a CLI code such as `TENANT_REQUIRED`, `AUTH_MISSING_HEADER`, `API_URL_MISSING`, `INSECURE_BASE_URL`, `INVALID_MEMBER_ID`, `VALIDATION_ERROR`, `NETWORK_ERROR` |
| `message` | human-readable explanation |
| `http_status` | HTTP status, when there is one (a CLI-detected error carries the status it maps to, where it has one) |
| `request_id` | the `X-Request-Id` the CLI sent with the request, or the server's if it sends one; absent for errors detected before any request |
| `raw` | the server's response body, verbatim, when the server answered |
| `meaning`, `next`, `capability_note` | present only when the CLI knows more about the code; the catalog is empty today, so expect none |

Branch on the exit code or `error.code`, not on the message text.

If `members list --all` fails after at least one page was fetched, stdout still holds a single
document: the rows fetched so far at `data`, and an `error` key above them, with a non-zero exit.
Test for the `error` key before treating the rows as complete.

### Exit codes

| Exit | Meaning | Source |
|---|---|---|
| 0 | Success | |
| 1 | General error: anything below does not cover, including HTTP 5xx and an unreadable config file | |
| 2 | Auth error | HTTP 401, or no API key configured (`AUTH_MISSING_HEADER`) |
| 3 | Permission error | HTTP 403 |
| 4 | Not found | HTTP 404; also `config` commands when the profile does not exist yet |
| 5 | Validation error | HTTP 400 or 422; also anything the CLI can see is wrong before sending (no tenant, no API URL, `http://` URL, malformed id or key, unknown command) |
| 6 | Network error | DNS, connection refused, timeout (requests give up after 30 seconds), unreadable or malformed response |
| 7 | Rate limit | HTTP 429 |
| 8 | Conflict | HTTP 409 |

## Status

Early, but no longer a scaffold. The API client (`internal/api`) is in place —
base URL resolution, injected auth and tenant headers, response decoding, and
error-to-exit-code mapping — and the first domain commands ship on top of it:

| Command | Endpoint |
|---|---|
| `seli health` | `GET /api/health` |
| `seli members list` | `GET /api/members` |
| `seli members get <id>` | `GET /api/members/{id}` |

Plus `seli auth login` / `seli auth logout`, local `seli config ...`, and `seli version`.
[`api/openapi.json`](api/openapi.json) declares more operations than the CLI wraps (roles,
member permissions, capabilities, delivery); command groups are added as they are planned —
see [`docs/api-coverage-gaps.md`](docs/api-coverage-gaps.md) and [`docs/plans/`](docs/plans/)
for what is missing and what is in flight.

## Design goals (carried over from the scaffold)

- **Built for AI agents** — a single JSON envelope on stdout, always; configurable entirely through `SELI_API_URL`, `SELI_API_KEY` and `SELI_TENANT`, with no config file
- **One output shape** — every command prints `{"data": …, "meta": …}`
- **Deterministic exit codes** — agents branch on exit code, not error text
- **Single binary** — no runtime required
- **Cross-platform** — Linux, macOS, Windows on amd64 and arm64

## Development

```bash
go mod download
make build   # ./dist/seli
make test
make lint
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for full setup and project structure.

### Adding API commands

New command groups are built with the **`api-command-builder`** agent skill
([`.agents/skills/api-command-builder/`](.agents/skills/api-command-builder/)), so that
every command traces back to the spec and lands with a reviewed plan. In Claude Code:

```
/api-command-builder add commands for PCMS product variants
```

The skill:

1. Resolves the requirement against [`api/openapi.json`](api/openapi.json) — the only
   source of truth for paths, params, bodies, and per-endpoint tenant rules. It never
   invents an endpoint; anything the spec doesn't declare goes to
   [`docs/api-coverage-gaps.md`](docs/api-coverage-gaps.md) instead.
2. Writes a numbered implementation plan in [`docs/plans/`](docs/plans/) from
   [`docs/templates/implementation-plan.md`](docs/templates/implementation-plan.md).
3. **Stops for your approval** — no source file is touched before you sign off on the
   endpoints, command surface, and scope.
4. Implements the plan, updating its execution-tracking table as each step lands and
   marking the plan `done` once `make build`, `make test`, and `make lint` all pass.

The conventions it enforces — one JSON envelope, deterministic exit codes, tenant
resolution, `--from-json` for complex bodies — are documented in the skill's
`references/` and apply to hand-written commands too.

### The shipped agent skill

[`skills/seli-api/`](skills/seli-api/) is a product artifact, not repo tooling: it is
the skill an AI agent loads to drive the installed `seli` binary on someone else's
machine. It teaches a discovery workflow (`seli --help` is authoritative for flags),
the CLI's stable contracts, exit-code handling, and tenant resolution — deliberately
without duplicating the command tree, since the binary is self-documenting.

Because those agents can't see this repo, the skill has to stay true to the CLI. Keep
it in sync with the **`seli-api-skill-maintainer`** agent skill
([`.agents/skills/seli-api-skill-maintainer/`](.agents/skills/seli-api-skill-maintainer/)):

```
/seli-api-skill-maintainer sync the skill with the commands that just landed
```

It diffs the repo against the provenance recorded in `skills/seli-api/.sync.json`,
rebuilds the ground truth from a fresh binary, the spec, and `internal/api/errors.go` (the exit codes), then
fixes both directions of drift — stale claims *and* assertions the CLI has outgrown —
including `evals/evals.json`. Run it after new commands land, after `api/openapi.json`
changes, before a release, or whenever `--help` and the skill disagree.

## License

MIT — see [LICENSE](LICENSE).
