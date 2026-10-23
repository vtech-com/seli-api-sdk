# Seli CLI SDK

[![CI](https://github.com/vtech-com/seli-api-sdk/actions/workflows/ci.yml/badge.svg)](https://github.com/vtech-com/seli-api-sdk/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/vtech-com/seli-api-sdk)](https://github.com/vtech-com/seli-api-sdk/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A CLI tool and Go SDK for the Seli platform — built for third-party AI agents, automation scripts,
and developers who need a scriptable interface to Seli's Public API.

```bash
# Store your API key and name a default tenant (once)
seli auth login --key ssk_…
seli config set-default-tenant acme

# Confirm the API accepts both
seli health

# List the tenant's members, and pipe them to jq
seli members list | jq '.data[]'
```

> **Every command that reaches the API needs a tenant.** The spec declares `X-Tenant-Code` as a
> required header on every endpoint the CLI wraps today — `health` included. Name one with
> `--tenant`, `SELI_TENANT`, or `seli config set-default-tenant`; with none, the command exits `5`
> (`TENANT_REQUIRED`) before sending anything. See [Configuration](#configuration).

## Why Seli CLI?

Seli exposes a Public API, but integrating it directly means writing your own HTTP client, auth and
tenant headers, pagination, and error handling. This SDK removes that friction:

- **Zero-config for AI agents** — configurable entirely through `SELI_API_URL`, `SELI_API_KEY` and
  `SELI_TENANT`, with no config file
- **One output shape** — every command prints one JSON document on stdout: `{"data": …, "meta": …}`
  on success, `{"error": …}` on failure
- **Deterministic exit codes** — agents branch on exit code, not error message text
- **Single binary** — no Node.js, Python, or any runtime required
- **Cross-platform** — Linux, macOS, Windows on amd64 and arm64

## Installation

Release builds have the Seli API host baked in. A binary you build yourself does not — see
[Build from source](#build-from-source) and [Configuration](#configuration).

### Linux / macOS (install script) — recommended for servers and VPS hosts

```bash
curl -fsSL https://raw.githubusercontent.com/vtech-com/seli-api-sdk/main/scripts/install.sh | sh
```

The script detects your OS and architecture, downloads the matching release archive, **verifies it
against the release's `checksums.txt`**, and installs `seli` into `/usr/local/bin` (it uses `sudo`
only if that directory is not writable). To upgrade, run the same command again.

If piping a script into a shell is against your policy, download it, read it, then run it:

```bash
curl -fsSL -O https://raw.githubusercontent.com/vtech-com/seli-api-sdk/main/scripts/install.sh
less install.sh
sh install.sh
```

Two environment variables control it:

| Variable | Purpose |
|----------|---------|
| `SELI_VERSION` | Install a specific version (e.g. `v0.1.1`) instead of the latest release |
| `SELI_BIN_DIR` | Install somewhere other than `/usr/local/bin` (useful to avoid `sudo`) |

```bash
# Pin a version, install without sudo
SELI_VERSION=v0.1.1 SELI_BIN_DIR="$HOME/.local/bin" sh install.sh
```

### Debian / Ubuntu (`.deb`)

Every release ships a `.deb` alongside the archives, so the system package manager tracks the
binary (clean uninstall, `dpkg -l seli`):

```bash
VERSION=0.1.1   # without the leading "v"
curl -fsSLO "https://github.com/vtech-com/seli-api-sdk/releases/download/v${VERSION}/seli_${VERSION}_linux_amd64.deb"
sudo apt install "./seli_${VERSION}_linux_amd64.deb"
```

There is no APT repository, so `apt upgrade` will not pull new versions on its own; install the
newer `.deb` the same way. To remove it: `sudo apt remove seli`.

### Homebrew (macOS only)

The tap ships a **cask**, and casks are macOS-only — `brew install --cask` does not work on
Linuxbrew. On Linux, use the install script or the `.deb` above.

```bash
brew install --cask vtech-com/tap/seli
```

To update an existing installation:

```bash
brew update
brew upgrade --cask seli
seli version
```

### GitHub Releases

Download the pre-built archive for your platform from
[Releases](https://github.com/vtech-com/seli-api-sdk/releases), extract it, and place `seli` in your
`PATH`. Binaries are provided for:

- Linux: `amd64`, `arm64`
- macOS (darwin): `amd64`, `arm64`
- Windows: `amd64`, `arm64` (`.zip`)

### Build from source

A bare build has no default host, so bake one in with `SELI_DEFAULT_API_URL`:

```bash
git clone https://github.com/vtech-com/seli-api-sdk.git
cd seli-api-sdk
SELI_DEFAULT_API_URL=https://<host> make build
# binary at ./dist/seli
```

Requires Go (see `go.mod` for the version).

### Go install

```bash
go install github.com/vtech-com/seli-api-sdk@latest
```

Go names the installed binary after the module path, so it lands as `seli-api-sdk`, not `seli`.
Rename it:

```bash
mv "$(go env GOPATH)/bin/seli-api-sdk" "$(go env GOPATH)/bin/seli"
```

A `go install` binary has **no** default host: every call fails with `API_URL_MISSING` (exit `5`)
until you set `SELI_API_URL`, pass `--api-url`, or run `seli config set api_url https://<host>`.

## Quick Start

```bash
# 1. Store your API key in ~/.seli/config.json (mode 600)
seli auth login --key ssk_…

# 2. Name the tenant to act on when --tenant is omitted
seli config set-default-tenant acme

# 3. Preflight: confirm the API accepts the key and the tenant
seli health

# 4. List members, or fetch one by UUID
seli members list --status active
seli members get <member-uuid>

# 5. Pipe to jq for agent or script processing
seli members list --all | jq '.data | length'
```

> **`seli health` is the first real check.** `auth login` and `config set-default-tenant` only
> write the local config file; they do not call the API. Exit `0` from `health` means the API is
> reachable *and* accepts both the key and the tenant.

A successful `seli health`:

```json
{
  "data": { "ok": true, "timestamp": "2026-07-31T09:30:00.000Z" },
  "meta": { "tenant": "acme", "tenant_source": "config" }
}
```

If you cannot or would rather not store the key (a shared machine, CI), skip step 1 and export
`SELI_API_KEY`; export `SELI_TENANT` to skip step 2. A key passed with `--key` stays in your shell
history, which is why the environment variable is the better route there — see
[Headless hosts](#headless-hosts-vps-ci-agent-runners).

## Commands

```
seli auth login --key <ssk_…>       Save an API key to the active profile (no API call)
seli auth logout                    Remove the stored key (leaves SELI_API_KEY alone)

seli config get <key>               Read api_url, default_profile or default_tenant
seli config set <key> <value>       Set api_url or default_profile
seli config set-default-tenant <code>   Set the default tenant for the active profile
seli config unset-default-tenant        Clear the default tenant

seli health                         Preflight: GET /api/health — proves key + tenant resolve end-to-end

seli members list                   GET /api/members (--page, --page-size 1-100, --status active|inactive,
                                    --all to auto-paginate)
seli members get <id>               GET /api/members/{id} (UUID; malformed ids exit 5 before any request)

seli version                        Print version, commit, build date and the baked-in default_api_url
```

Run `seli <group> <command> --help` for the complete, authoritative flag list from your binary —
the help ships inside it and describes that build exactly. Facts that hold for every command are
stated once, as help topics:

```bash
seli help tenancy       # how --tenant resolves, and who requires it
seli help output        # the envelope, and which stream carries what
seli help exit-codes    # what each exit code and error code means
```

**Global flags** available on every command:

| Flag | Description |
|------|-------------|
| `--api-url <url>` | Override the API base URL (host only; `https`, or `http` on `localhost`) |
| `--tenant <code>` | Tenant code to scope the request to |
| `-v, --verbose` | Trace the HTTP request and response on stderr (key redacted) |

[`api/openapi.json`](api/openapi.json) declares more operations than the CLI wraps today (roles,
member permissions, capabilities, delivery). Command groups are added as they are planned — see
[`docs/api-coverage-gaps.md`](docs/api-coverage-gaps.md) for what is missing and
[`docs/plans/`](docs/plans/) for what is in flight.

## Configuration

Credentials and settings are stored in `~/.seli/config.json` (permissions `600`):

```json
{
  "version": 1,
  "profiles": {
    "default": {
      "api_key": "ssk_…",
      "api_url": "https://<host>",
      "default_tenant": "acme"
    }
  },
  "active_profile": "default"
}
```

There is exactly one active profile; the CLI takes no `--profile` flag. `seli auth login` writes the
key into the active profile, creating `default` when the file has none. Selecting another profile is
a write to the file — `seli config set default_profile <name>` — and exits `5` if `<name>` is not
already in it.

### API host

The host is configuration, not part of the CLI, and every path the CLI requests begins with
`/api/` (`/api/health`, `/api/members`). Give the host only — `https://<host>`, no path. The first
of these that is set wins:

1. `--api-url`
2. `SELI_API_URL`
3. `api_url` in the active profile (`seli config set api_url https://<host>`; needs a profile, so
   run `seli auth login` first)
4. the default baked into the binary at build time (`SELI_DEFAULT_API_URL`, set for release
   builds); `seli version` prints it as `default_api_url` (empty when the build has none)

If none is set, the CLI fails locally with exit `5` and `API_URL_MISSING`. A plain `http://` URL is
refused (exit `5`, `INSECURE_BASE_URL`) unless the host is `localhost` or `127.0.0.1`.

### API key

`SELI_API_KEY` if set, otherwise the key stored by `seli auth login`. With neither, the CLI fails
locally with exit `2` and `AUTH_MISSING_HEADER` and sends nothing.

### Tenant

The first of these that is set wins: `--tenant`, `SELI_TENANT`, the profile's `default_tenant`
(`seli config set-default-tenant`). If none is set, a tenant-scoped command exits `5` with
`TENANT_REQUIRED` before sending anything. `meta.tenant` and `meta.tenant_source` on stdout name
the tenant the call resolved to and where it came from.

### Configuration precedence

```
CLI flag (--api-url, --tenant)
  > Environment variable (SELI_API_URL, SELI_API_KEY, SELI_TENANT)
    > Config file (~/.seli/config.json)
      > Build-time default (API host only)
```

[`.env.example`](.env.example) lists every variable with placeholders.

### Headless hosts (VPS, CI, agent runners)

On a machine with no human at the keyboard, **do not use `seli auth login`** — it exists to write
`~/.seli/config.json`. Provision the environment instead; the variables take precedence over the
config file, so no login step is needed:

```bash
export SELI_API_KEY=ssk_…
export SELI_TENANT=acme
seli health   # exit 0 = API reachable, key and tenant accepted
```

Source the key from whatever your host already uses for secrets (a systemd unit's
`EnvironmentFile=`, the CI secret store, your process manager's env block) rather than committing
it to a shell profile.

To point at a different environment (staging, local dev) without touching the config file:

```bash
SELI_API_URL=http://localhost:3999 SELI_API_KEY=ssk_dev_… SELI_TENANT=acme seli members list
```

## Output

There is no output flag and no output modes. Every command prints exactly one JSON document on
stdout, whether it read a record, read a page of them, or failed:

```json
{ "data": …, "meta": { … } }
```

```bash
seli members list | jq '.data[] | {id, status}'
seli members list --all | jq '.data | length'
seli members get <member-uuid> | jq '.data'
```

Redirecting (`>`) or piping (`|`) is always safe — stdout is JSON and nothing else is ever written
to it.

### JSON contract

| Command type | Shape | Notes |
|---|---|---|
| `members list` | `{"data": [...], "meta": {"tenant", "tenant_source", "pagination": {...}}}` | `data` is always an array. `pagination` is the API's own object, `{page, pageSize, total, totalPages}`, passed through untouched. |
| Single-record commands (`members get`, `health`) | `{"data": {...}, "meta": {"tenant", "tenant_source"}}` | Every field the API sends, under `data`. |
| Local commands (`version`, `auth`, `config get`) | `{"data": {...}, "meta": {}}` | Not tenant-scoped. `config set`, `config set-default-tenant` and `config unset-default-tenant` print nothing on success. |

| `meta` key | When | Meaning |
|---|---|---|
| `tenant` | tenant-scoped commands | the tenant code the call resolved to |
| `tenant_source` | tenant-scoped commands | where it came from: `flag`, `env` (`SELI_TENANT`) or `config` (`default_tenant`) |
| anything else | as the API sends it | the API's own `meta`, passed through untouched |

`meta.pagination.total` is the count across every page — count it, never `data[]`. With `--all`,
`meta.pagination` reflects the last page fetched.

### Errors

A failure prints `{"error": {…}}` on stdout (so a caller that parses stdout always gets JSON), a
one-line summary on stderr, and exits non-zero. The `error` object has:

| Field | Meaning |
|---|---|
| `code` | machine-readable code: the API's own, a `HTTP_<status>` fallback, or a CLI code such as `TENANT_REQUIRED`, `AUTH_MISSING_HEADER`, `API_URL_MISSING`, `INSECURE_BASE_URL`, `INVALID_MEMBER_ID`, `VALIDATION_ERROR`, `NETWORK_ERROR` |
| `message` | human-readable explanation |
| `http_status` | HTTP status, when there is one (a CLI-detected error carries the status it maps to, where it has one) |
| `request_id` | the `X-Request-Id` the CLI sent with the request, or the server's if it sends one; absent for errors detected before any request |
| `raw` | the server's response body, verbatim, when the server answered |
| `meaning`, `next`, `capability_note` | present only when the CLI knows more about the code; the catalog is empty today, so expect none |

> **Partial results carry an `error` key.** If `members list --all` fails after at least one page
> was fetched, or stops at the 1000-page cap (`PAGE_LIMIT_EXCEEDED`), stdout still holds a single
> document: the rows fetched so far at `data`, and an `error` key above them, with a non-zero exit.
> Test for the `error` key before treating the rows as complete.

## Exit Codes

AI agents should branch on exit code or `error.code`, **not** on error message text.

| Code | Meaning | Source |
|------|---------|--------|
| `0` | Success | |
| `1` | General error | anything below does not cover, including HTTP 5xx and an unreadable config file |
| `2` | Auth error | HTTP 401, or no API key configured (`AUTH_MISSING_HEADER`) |
| `3` | Permission error | HTTP 403 |
| `4` | Not found | HTTP 404; also `config` commands when the profile does not exist yet |
| `5` | Validation error | HTTP 400 or 422; also anything the CLI can see is wrong before sending (no tenant, no API URL, `http://` URL, malformed id or key, unknown command) |
| `6` | Network error — agent should retry | DNS, connection refused, timeout (requests give up after 30 seconds), unreadable or malformed response |
| `7` | Rate limit — agent should back off | HTTP 429 |
| `8` | Conflict | HTTP 409 |

## AI Agent Integration

### Shell / jq

```bash
# Count active members of a tenant
seli members list --tenant acme --status active --all | jq '.meta.pagination.total'
```

### Python

```python
import subprocess, json

result = subprocess.run(
    ["seli", "members", "list", "--tenant", "acme", "--all"],
    capture_output=True, text=True,
)
doc = json.loads(result.stdout)  # stdout is JSON on success and on failure
if result.returncode == 0:
    members = doc["data"]
elif result.returncode == 6:
    # Network error — retry
    ...
elif result.returncode == 7:
    # Rate limit — back off before retrying
    ...
else:
    print(doc["error"]["code"], doc["error"]["message"])
```

### n8n

Use the **Execute Command** node:

```
seli members list --tenant acme
```

### Environment variable injection (CI / secrets manager)

```bash
SELI_API_KEY=${{ secrets.SELI_API_KEY }} \
SELI_API_URL=${{ vars.SELI_API_URL }} \
SELI_TENANT=acme \
seli members list
```

### Bundled agent skill

This repo ships an agent **skill** under [`skills/seli-api/`](skills/seli-api/) — the operating
manual an AI agent loads to drive the installed `seli` binary on someone else's machine. It
teaches a discovery workflow (`seli --help` is authoritative for flags), the CLI's stable
contracts, exit-code handling, and tenant resolution — deliberately without duplicating the
command tree, since the binary is self-documenting and the CLI can grow without making a static
catalogue stale.

**Install it** with the [`skills`](https://github.com/vercel-labs/skills) CLI, which pulls the skill
straight from this repo into your agent's skills directory (Claude Code, Cursor, Codex, OpenCode,
and more):

```bash
npx skills add vtech-com/seli-api-sdk --skill seli-api
```

Add `-g` to install at user level (e.g. `~/.claude/skills/`) instead of per-project, `-a <agent>` to
target a specific agent, or `--list` to see the available skills without installing.

## API Reference

OpenAPI spec: [`api/openapi.json`](api/openapi.json) — the source of truth for every endpoint,
schema, parameter, and tenant requirement. Refresh it with `make update-spec`, never by hand.

View it interactively:

- Paste into [editor.swagger.io](https://editor.swagger.io)
- Or use the VS Code **Swagger Viewer** extension

## Compatibility

Don't rely on a hardcoded version number in this file to know what's current — check the
[Release badge](#seli-cli-sdk) at the top of this README or the
[Releases page](https://github.com/vtech-com/seli-api-sdk/releases) for the latest tag, and
[`CHANGELOG.md`](CHANGELOG.md) for what shipped in it.

## Development

```bash
go mod download
make build   # ./dist/seli
make test
make lint
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for full setup, project structure, and the branch and
release flow.

### Adding API commands

New command groups are built with the **`api-command-builder`** agent skill
([`.agents/skills/api-command-builder/`](.agents/skills/api-command-builder/)), so that every
command traces back to the spec and lands with a reviewed plan. In Claude Code:

```
/api-command-builder add commands for PCMS product variants
```

The skill:

1. Resolves the requirement against [`api/openapi.json`](api/openapi.json). It never invents an
   endpoint; anything the spec doesn't declare goes to
   [`docs/api-coverage-gaps.md`](docs/api-coverage-gaps.md) instead.
2. Writes a numbered implementation plan in [`docs/plans/`](docs/plans/) from
   [`docs/templates/implementation-plan.md`](docs/templates/implementation-plan.md).
3. **Stops for your approval** — no source file is touched before you sign off on the endpoints,
   command surface, and scope.
4. Implements the plan, updating its execution-tracking table as each step lands and marking the
   plan `done` once `make build`, `make test`, and `make lint` all pass.

The conventions it enforces — one JSON envelope, deterministic exit codes, tenant resolution,
`--from-json` for complex bodies — are documented in the skill's `references/` and apply to
hand-written commands too.

### Keeping the agent skill in sync

Agents that load [`skills/seli-api/`](skills/seli-api/) can't see this repo, so the skill has to
stay true to the CLI: any change to flags, output, or exit semantics updates it in the same PR. Use
the **`seli-api-skill-maintainer`** agent skill
([`.agents/skills/seli-api-skill-maintainer/`](.agents/skills/seli-api-skill-maintainer/)):

```
/seli-api-skill-maintainer sync the skill with the commands that just landed
```

It diffs the repo against the provenance recorded in `skills/seli-api/.sync.json`, rebuilds the
ground truth from a fresh binary, the spec, and `internal/api/errors.go` (the exit codes), then
fixes both directions of drift — stale claims *and* assertions the CLI has outgrown — including
`evals/evals.json`. Run it after new commands land, after `api/openapi.json` changes, before a
release, or whenever `--help` and the skill disagree.

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines and
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for the standards that apply to all interactions.

## Security

To report a security vulnerability, use GitHub's
[private vulnerability reporting](https://github.com/vtech-com/seli-api-sdk/security/advisories/new).
Do **not** open a public GitHub issue. See [SECURITY.md](SECURITY.md) for the full policy.

## License

[MIT](LICENSE)
