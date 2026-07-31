# Seli CLI SDK — Project Context

> **Status:** Draft v2.1
> **Owner:** Luu Trong
> **Last updated:** 2026-05-23
> **Target audience:** Seli dev team & open source contributors

---

## 1. Overview & Goals

### 1.1 Background

Seli exposes a stable Public API at `/api/v1/*` with API key authentication (keys start with `sk_`). Today, any third-party that wants to integrate with Seli must:

- Read the OpenAPI spec and understand it manually
- Implement their own HTTP client
- Manage credentials and tenant context themselves
- Handle pagination, error codes, and retry logic from scratch

This creates **significant friction for third-party AI agents** (LangChain, CrewAI, n8n, custom Python agents, etc.) that want to use Seli as a backend tool.

### 1.2 Goals

Build an **open source CLI SDK** modeled after `gcloud` (Google Cloud) or `stripe` CLI (Stripe):

- **Zero-config integration**: AI agents only need `shell exec("seli tasks list")` to get started
- **Centralized credential & tenant management** at `~/.seli/config.json`
- **Standardized output formats** (`json | table | yaml`) for easy AI agent parsing
- **Standardized exit codes** so agents can decide retry vs escalate
- **Single-binary distribution** — no Node.js or Python runtime required
- **Open source (MIT)** to build developer community and brand recognition

### 1.3 Non-goals (Phase 1)

- Does not replace the Web UI (Web UI remains the primary interface for human users)
- Does not expose internal APIs (`/api/internal/*`, `/api/mcp/*`)
- Does not build IDE plugins or GUI apps
- Does not support OAuth flows in the CLI (only accepts `sk_` keys created on the web)

---

## 2. Technology Decisions

### 2.1 Language: **Go**

| Criteria | Go | Python | Decision |
|---|---|---|---|
| Single binary distribution | ✅ Native, ~8MB | ⚠️ PyInstaller ~50MB, fragile | **Go wins** |
| Startup time | ~5ms | ~200ms (import overhead) | Go wins |
| Cross-compile (Linux/macOS/Windows × amd64/arm64) | ✅ Trivial | ❌ Must build on each OS | Go wins |
| Learning curve (low experience on both sides) | Moderate, clear patterns | Lower but painful packaging | Go sufficient |
| AI agent ecosystem | Shell exec (language-agnostic) | Can import as library | Shell exec → not a factor |

**Conclusion:** Go is the best fit because priority #1 is easy distribution. Low Go experience on the team is not a major issue — CLI is a simple use case (HTTP client + JSON + flag parsing).

### 2.2 Stack

```
Go 1.22+
├── github.com/spf13/cobra        # CLI framework (used by kubectl, hugo, gh CLI)
├── github.com/spf13/viper        # Config management
├── github.com/jedib0t/go-pretty  # Table output
└── (std lib)                     # net/http, encoding/json
```

Not using:
- Web framework (not needed)
- ORM (not needed — calls HTTP API)
- Complex logging framework (std `log` + verbose flag is sufficient)

### 2.3 Build & Release

- **GoReleaser** — auto cross-compile + GitHub Release creation
- **GitHub Actions** for CI/CD
- **golangci-lint** for code quality
- **Dependabot** for dependency updates

---

## 3. Architecture

### 3.1 High-level flow

```
┌─────────────────────────────────────────────────────┐
│                Third-party AI Agent                 │
│  (LangChain / CrewAI / n8n / custom Python agent)   │
└────────────────┬────────────────────────────────────┘
                 │ shell exec / subprocess
                 ▼
┌─────────────────────────────────────────────────────┐
│              seli CLI (single binary)             │
│                                                     │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────┐   │
│  │   auth   │  │  config  │  │  command modules │   │
│  │ (sk_ +  │  │ (active  │  │  tasks / members │   │
│  │  profile)│  │  tenant) │  │  boards / wms... │   │
│  └──────────┘  └──────────┘  └──────────────────┘   │
│                      │                              │
│           ~/.seli/config.json                     │
└────────────────┬────────────────────────────────────┘
                 │ HTTPS + Bearer sk_ + X-Tenant-Code
                 ▼
┌─────────────────────────────────────────────────────┐
│         Seli Public API  /api/v1/*                │
└─────────────────────────────────────────────────────┘
```

### 3.2 Project structure

```
seli-api-sdk/                          # Standalone repo on GitHub
├── LICENSE                          # MIT
├── README.md
├── CONTRIBUTING.md
├── CODE_OF_CONDUCT.md               # Contributor Covenant 2.1
├── SECURITY.md                      # security@seli.app
├── CHANGELOG.md                     # Keep a Changelog format
├── go.mod
├── go.sum
├── main.go                          # Entry point
│
├── api/
│   └── openapi.json                 # OpenAPI spec for the Seli Public API
│
├── .github/
│   ├── workflows/
│   │   ├── ci.yml                   # Test + lint on every PR
│   │   ├── release.yml              # GoReleaser on tag v*
│   │   └── codeql.yml               # Security scanning
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug_report.md
│   │   └── feature_request.md
│   ├── PULL_REQUEST_TEMPLATE.md
│   ├── CODEOWNERS
│   └── dependabot.yml
│
├── .goreleaser.yml                  # Multi-platform release config
├── Makefile                         # Common dev commands
│
├── cmd/                             # Cobra commands (one file per command tree)
│   ├── root.go                      # Root command + global flags
│   ├── auth.go                      # seli auth login/logout/whoami
│   ├── config.go                    # seli config use-tenant/set/get
│   ├── tasks.go                     # seli tasks list/get/create
│   ├── members.go                   # seli members list
│   └── boards.go                    # seli boards list
│
├── internal/                        # Private packages
│   ├── api/
│   │   ├── client.go                # HTTP client + auth headers
│   │   ├── errors.go                # API error → exit code mapping
│   │   └── models.go                # Response types (Task, Member...)
│   ├── config/
│   │   └── store.go                 # Read/write ~/.seli/config.json
│   └── output/
│       └── formatter.go             # table | json | yaml renderer
│
├── pkg/                             # Public packages (for those who want to import as lib)
│   └── (empty in Phase 1)
│
├── docs/                            # Additional documentation
│   ├── commands/                    # Auto-generated command docs
│   └── examples/                    # Use case examples
│
└── examples/                        # Sample scripts for users
    ├── bash/
    ├── python-wrapper/
    └── n8n-integration/
```

### 3.3 Config file schema

```json
// ~/.seli/config.json
{
  "version": 1,
  "profiles": {
    "default": {
      "api_key": "sk_abc123...",
      "api_url": "https://platform.seli.app",
      "default_tenant": "acme",
      "known_tenants": ["acme", "globex", "initech"]
    }
  },
  "active_profile": "default"
}
```

Additional profiles can be added (e.g. for different API keys or environments) by reusing the same shape.

**Notes on tenant fields:**

- `default_tenant` is **optional**. If set, it's used when no `--tenant` flag is passed. It is a **soft default**, not a hard lock — every command can override it per call.
- `known_tenants` is a local cache populated from `GET /api/v1/tenants` (refreshed on `seli tenants list` or on auth). Used for shell completion and validation. Not authoritative.
- A user typically belongs to multiple tenants and may switch between them often (e.g. a product manager working across several tenants in one session), so the CLI must not bind to a single "active tenant".

**File permissions:** Must `chmod 600` on create/update (contains secrets).

### 3.4 Configuration precedence

Following `gcloud` / `aws-cli` conventions:

```
CLI flag (--api-key, --tenant, --no-tenant)
  > Environment variable (SELI_API_KEY, SELI_TENANT, SELI_PROFILE)
    > Config file (~/.seli/config.json)
```

AI agents can inject credentials via env vars instead of writing a file:

```bash
SELI_API_KEY=sk_... SELI_TENANT=acme seli tasks list
```

**Tenant resolution rule (specific):**

```
--tenant <code>       → use that tenant
  else --no-tenant    → force global mode (no X-Tenant-Code)
    else $SELI_TENANT  → use that tenant
      else config.default_tenant  → use that tenant
        else                       → global mode (no X-Tenant-Code)
```

**Default behavior when nothing is configured: global mode** (no tenant header sent). The API returns data across all accessible tenants. This is intentional — it lets new users see something useful immediately without configuration.

---

## 4. UX Design

### 4.1 Command structure

Follows `noun verb` pattern (same as `kubectl`, `gh`, `gcloud`):

```
seli <noun> <verb> [flags]

seli auth login
seli auth logout
seli auth whoami

seli config set <key> <value>
seli config get <key>
seli config set-default-tenant <tenant_code>
seli config unset-default-tenant
seli config list-profiles

seli tenants list                            # List tenants the user can access

seli tasks list [--tenant <code>] [--no-tenant] [--status open]
seli tasks get <task_id> [--tenant <code>]
seli tasks create --title "..." --tenant <code>

seli boards list [--tenant <code>]
seli boards get <id> [--tenant <code>]

seli members list [--tenant <code>]

seli products list --tenant <code>           # Tenant required for /pcms/*
```

**Global flags available on every command:**

- `--tenant <code>` — scope the call to a specific tenant
- `--no-tenant` — force global mode (override `default_tenant` from config)
- `--profile <name>` — use a specific config profile
- `--output table|json|yaml|quiet` — output format
- `--api-url <url>` — override API base URL (for staging/local dev)

### 4.2 Setup flow

```bash
# 1. Install (see Section 8 for details)
brew install --cask vtech-com/tap/seli

# 2. Login (paste key created from Seli web)
seli auth login --key sk_abc123...
# → Saved to ~/.seli/config.json (chmod 600)

# 3. Verify
seli auth whoami
# ✅ Logged in as: alice@example.com
# 🔑 Profile: default

# 4. See which tenants you can access
seli tenants list
# ┌──────────┬──────────────┬─────────┐
# │ Code     │ Name         │ Role    │
# ├──────────┼──────────────┼─────────┤
# │ acme     │ ACME Corp    │ owner   │
# │ globex   │ Globex Corp  │ member  │
# │ initech  │ Initech      │ member  │
# └──────────┴──────────────┴─────────┘

# 5. (Optional) Set a default tenant for convenience
seli config set-default-tenant acme

# 6. Start using
seli tasks list                    # Uses default_tenant = acme
seli tasks list --tenant globex  # Override for one call
seli tasks list --no-tenant        # Global view across all tenants
```

**Key principle:** the user is never forced to pick a single tenant up-front. Tenant scope is decided per command, with a sensible default if one is configured.

### 4.3 Output modes

```bash
# Default: human-readable table
# When scoped to one tenant, the Tenant column is hidden
$ seli tasks list --tenant acme
┌──────────────┬───────────────────┬────────┬────────────┐
│ ID           │ Title             │ Status │ Assignee   │
├──────────────┼───────────────────┼────────┼────────────┤
│ task_abc123  │ Fix login bug     │ To-Do  │ Alice      │
└──────────────┴───────────────────┴────────┴────────────┘

# In global mode, a Tenant column is added so results stay traceable
$ seli tasks list --no-tenant
┌──────────┬──────────────┬───────────────────┬────────┐
│ Tenant   │ ID           │ Title             │ Status │
├──────────┼──────────────┼───────────────────┼────────┤
│ acme     │ task_abc123  │ Fix login bug     │ To-Do  │
│ globex   │ task_xyz789  │ Update menu       │ Done   │
└──────────┴──────────────┴───────────────────┴────────┘

# AI agent uses JSON (Tenant info preserved in payload)
$ seli tasks list --output json
[{"id":"task_abc123","title":"Fix login bug","status":"To-Do",...}]

# Quiet mode — returns ID only (for shell piping)
$ seli tasks create --title "X" --tenant acme --output quiet
task_def456

# Pipe with jq (common AI agent pattern)
$ seli tasks list --no-tenant --output json | jq '.[] | select(.status=="To-Do")'
```

### 4.4 Exit codes — CRITICAL for AI agents

```
0  — Success
1  — General error (unexpected)
2  — Auth error (invalid/expired/revoked key)
3  — Permission error (403 — tenant mismatch, no role)
4  — Not found (404)
5  — Validation error (400)
6  — Network error (DNS, timeout — agent should retry)
7  — Rate limit (429 — agent should backoff)
```

AI agents use exit codes to branch logic — **do not parse error message text** (i18n and format may change).

### 4.5 Error format

```bash
$ seli tasks get bad_id --output json
{
  "error": {
    "code": "NOT_FOUND",
    "message": "Task with id 'bad_id' not found",
    "request_id": "req_xyz789"
  }
}
# Exit code: 4
```

All errors include `code` (machine-readable) and `request_id` (for debugging with Seli support).

---

## 5. Integration with Seli Public API

### 5.1 Base URL

```
Production:  https://platform.seli.app/api/v1
Local dev:   http://localhost:3999/api/v1
```

The CLI uses `https://platform.seli.app/api/v1` by default. Override with `--api-url` or `SELI_API_URL` env var for staging/local development.

### 5.2 Authentication

```http
GET /api/v1/mission/tasks HTTP/1.1
Host: platform.seli.app
Authorization: Bearer sk_abc123...
X-Tenant-Code: acme
User-Agent: seli-api-sdk/1.0.0 (darwin; arm64)
```

**Headers the CLI injects automatically:**
- `Authorization: Bearer <api_key>` (always)
- `X-Tenant-Code: <tenant_code>` (only when a tenant is resolved — see §3.4)
- `User-Agent: seli-api-sdk/<version> (<os>; <arch>)`
- `X-Request-Id: <uuid>` (for debugging)

API keys are issued by Seli through the Seli platform UI. The CLI never creates or rotates keys — it only consumes them.

### 5.3 Tenant handling per endpoint

The Public API supports both global and scoped operations. Each endpoint declares its own tenant requirement — the SDK must encode this per command. **The table below covers the current initial scope; new endpoints follow the same pattern (consult `api/openapi.json` for the authoritative requirement).**

| Endpoint | Tenant location | Required? | Notes |
|---|---|---|---|
| `GET /members` | `X-Tenant-Code` header | Optional | Omit → members from all accessible tenants |
| `GET /mission/boards` | `X-Tenant-Code` header | Optional | Omit → boards from all accessible tenants |
| `GET /mission/boards/{id}` | `X-Tenant-Code` header | Optional | |
| `GET /mission/tasks` | `X-Tenant-Code` header | Optional | Omit → tasks from all accessible tenants |
| `GET /mission/tasks/{id}` | `X-Tenant-Code` header | Optional | |
| `POST /mission/tasks` | **`tenant_code` body field** | **Required** | Header is ignored; body field is mandatory |
| `GET /pcms/products` | `X-Tenant-Code` header | **Required** | Returns 400 if omitted |
| `GET /tenants` | n/a | n/a | Always scoped to the authenticated user |

**Key implementation rules for the CLI:**

1. For `POST /mission/tasks`, inject the tenant code as the `tenant_code` **body field**, not the header. CLI must reject the command if no tenant is resolved (via flag, env, or default).
2. For `/pcms/*` endpoints, CLI must reject the command early if no tenant is resolved — don't let the server return 400.
3. For all other endpoints, send `X-Tenant-Code` only when a tenant is resolved; otherwise omit the header entirely (do not send empty string).
4. Per-endpoint tenant rules should live in `internal/api/` so that adding new endpoints doesn't require touching command code.

### 5.4 Initial endpoint coverage

The SDK starts by wrapping the endpoints below. **This list is not the full Seli API surface** — it's the initial scope for the first releases. The API is growing, and new endpoints are added over time; the SDK expands to cover them in subsequent releases.

| Command | Endpoint | Method |
|---|---|---|
| `auth whoami` | `GET /api/v1/me` | GET |
| `tenants list` | `GET /api/v1/tenants` | GET |
| `members list` | `GET /api/v1/members` | GET |
| `boards list` | `GET /api/v1/mission/boards` | GET |
| `boards get` | `GET /api/v1/mission/boards/{id}` | GET |
| `tasks list` | `GET /api/v1/mission/tasks` | GET |
| `tasks get` | `GET /api/v1/mission/tasks/{id}` | GET |
| `tasks create` | `POST /api/v1/mission/tasks` | POST |
| `products list` | `GET /api/v1/pcms/products` | GET |

The authoritative, always-current list of endpoints, schemas, and query parameters lives in [`api/openapi.json`](./api/openapi.json) — refer to it rather than this table when in doubt.

**Roadmap for additional endpoints:** as the Seli API exposes new resources (e.g. additional PCMS, WMS, or purchasing endpoints), corresponding commands are added to the SDK. Contributors are welcome to propose and implement new command bindings — see [CONTRIBUTING.md](./CONTRIBUTING.md).

### 5.5 Boundary — Do NOT call

The SDK is a client of the Public API at `/api/v1/*` **only**. It must not call any endpoint that is not declared in `api/openapi.json`. If a needed feature is missing, open a GitHub issue rather than reverse-engineering or guessing at unlisted endpoints.

### 5.6 Pagination

The Public API uses page-based pagination with `page` and `limit` query parameters, returning a `meta` block with `page`, `limit`, `total`, `has_more`. The CLI handles three modes:

```bash
# Default: first page only
$ seli tasks list
# → 20 items + footer "Showing 20 of 312. Use --all for full list."

# Fetch all (auto-paginate until has_more = false)
$ seli tasks list --all

# Manual paging
$ seli tasks list --page 2 --limit 50
```

### 5.7 Delta sync (PCMS only)

`GET /pcms/products` supports incremental sync via the `updated_since` query parameter and an `X-Server-Time` response header. AI agents and integration scripts should use this pattern to avoid re-fetching the full catalog on every call:

```bash
# First sync — save X-Server-Time from response header
$ seli products list --tenant acme --all --output json

# Subsequent syncs — use saved timestamp
$ seli products list --tenant acme --updated-since 2026-01-01T00:00:00Z --all
```

Deleted products are returned with `is_deleted: true` so consumers can purge their local copies.

---

## 6. API Documentation Strategy

### 6.1 OpenAPI spec is the source of truth

The repo includes the OpenAPI spec for the Seli Public API at [`api/openapi.json`](./api/openapi.json). This file is the authoritative reference for:

- Available endpoints and HTTP methods
- Request parameters (query, path, header, body)
- Response schemas and error shapes
- Authentication requirements

All SDK code (HTTP client, types, command parameters) should align with this spec. When in doubt about an endpoint's behavior, the spec wins.

### 6.2 Layout in the repo

```
seli-api-sdk/
├── api/
│   └── openapi.json         # OpenAPI spec for the Seli Public API
├── cmd/
├── internal/
└── ...
```

The spec is version-controlled alongside the SDK source. Each SDK release ships with the exact spec it targets, so contributors and users can always tell which API contract a given SDK version supports.

### 6.3 Reading the spec

Suggested in the README:

```markdown
## API Reference

OpenAPI spec: [`api/openapi.json`](./api/openapi.json)

View it interactively:
- Paste into https://editor.swagger.io
- Or use the VS Code "Swagger Viewer" extension
```

### 6.4 Using the spec for code generation

Contributors building additional language bindings or wrappers can use any OpenAPI-compatible code generator against `api/openapi.json`. Common options:

- `openapi-generator` (multi-language)
- `oapi-codegen` (Go)
- `openapi-typescript` (TypeScript)

### 6.5 Keeping the spec in sync

The spec in this repo tracks the Seli Public API as of the most recent SDK release. Updates land via normal PRs whenever new endpoints or schema changes are picked up. Treat `api/openapi.json` like any other source file — review changes in PRs and bump the SDK version when the contract changes.

---

## 7. Repo & Workflow

### 7.1 Repo location

- **Repo:** `https://github.com/vtech-com/seli-api-sdk` (public, standalone)
- **License:** MIT
- **Branching:** GitHub Flow (main + feature branches, no develop branch)

### 7.2 Versioning

**Strict SemVer.** When to bump:

- `v0.x.x` — Pre-1.0; breaking changes allowed in minor (must be clearly noted in CHANGELOG)
- `v1.0.0` — When ≥50 real users + API stable for ≥3 months
- `v1.x.0` — New commands/flags added (backward compatible)
- `v1.x.x` — Bug fixes only
- `vN.0.0` (N≥2) — Breaking change (rare)

**API versioning:** CLI v1.x always targets `/api/v1/*`. When Seli introduces `/api/v2`, release CLI v2 in parallel (maintain CLI v1 for ≥12 months).

### 7.3 Deprecation policy

Before removing any command or flag:

1. Mark as `Deprecated` in help text
2. Print a warning to stderr when the user invokes it
3. Document in CHANGELOG at least **2 minor versions** before removal

```bash
$ seli old-command
⚠️  WARNING: 'old-command' is deprecated, use 'new-command' instead.
   Will be removed in v2.0.0. See CHANGELOG.md for migration notes.
```

### 7.4 Compatibility matrix

Maintain in README:

```markdown
| CLI version | Seli API | Status            | Support until |
|-------------|------------|-------------------|---------------|
| v1.x        | /api/v1    | ✅ Supported      | TBD           |
| v0.x        | /api/v1    | ⚠️ Beta, no SLA  | v1.0 release  |
```

---

## 8. Distribution

### 8.1 Channels — Phase 1

| Channel | Effort | Priority |
|---|---|---|
| **GitHub Releases** (via GoReleaser) | Already needed | P0 |
| **install.sh** (`curl -sSL ... \| sh`) | 1 hour | P0 |
| **Docker image** (`ghcr.io/vtech-com/seli-api-sdk`) | 1 hour | P0 |

### 8.2 Channels — Phase 2

| Channel | Effort | Priority |
|---|---|---|
| **Homebrew tap** (`brew install --cask vtech-com/tap/seli`) | 1 day | P1 |
| **Scoop bucket** (Windows) | 1 day | P1 |
| **APT/RPM repo** (Linux enterprise) | 2-3 days | P2 |

### 8.3 Install script

```bash
# https://seli.app/install.sh
curl -sSL https://seli.app/install.sh | sh

# Or with a specific version
curl -sSL https://seli.app/install.sh | VERSION=v1.2.0 sh
```

The script must:
- Detect OS + arch
- Download the correct binary from GitHub Releases
- Verify checksum (SHA256)
- Install to `/usr/local/bin/seli` (with `sudo` warning if needed)
- Print "✅ Installed seli v1.2.0. Run `seli auth login` to get started."

### 8.4 GoReleaser config

```yaml
# .goreleaser.yml
builds:
  - binary: seli
    main: ./main.go
    env: [CGO_ENABLED=0]
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.Date}}

archives:
  - format: tar.gz
    name_template: >-
      seli_{{ .Os }}_{{ .Arch }}
    format_overrides:
      - goos: windows
        format: zip

checksum:
  name_template: 'checksums.txt'

dockers:
  - image_templates:
      - "ghcr.io/vtech-com/seli-api-sdk:{{ .Tag }}"
      - "ghcr.io/vtech-com/seli-api-sdk:latest"
```

---

## 9. Open Source Governance

### 9.1 Pre-launch checklist

Required before making the repo public:

- [ ] At least 3 commands working end-to-end (`auth login`, `tasks list`, `tasks create`)
- [ ] LICENSE file (MIT)
- [ ] README with install + quickstart that **actually works** (tested on a fresh machine)
- [ ] CI passing on 3 OSes (Linux, macOS, Windows)
- [ ] At least 1 release with a downloadable binary
- [ ] SECURITY.md with a security contact email
- [ ] No secrets in git history:
  ```bash
  git log -S "sk_" --all
  ```
- [ ] Complete `.gitignore` (`*.env`, `.seli/`, `dist/`, `*.local`)
- [ ] Test fixtures contain no production data
- [ ] All code, comments, and docs are public-safe (reviewed by a maintainer)

### 9.2 Governance model

**Phase 1 (0–50 users):** Single maintainer
- 1–3 core maintainers with write access
- All PRs require ≥1 maintainer approval
- External contributors → PR must have a pre-approved issue

**Phase 2 (50–500 users):** Trusted contributors
- Promote 3–5 contributors with a good track record to maintainer
- Set up CODEOWNERS for auto-assigning reviewers

**Phase 3 (500+ users):** Steering committee
- Consider CLA (Contributor License Agreement) if needed
- May set up GitHub Discussions for the community

### 9.3 Issue & PR workflow

1. **Bug report:** User opens issue → maintainer triages within 48h
2. **Feature request:** Issue → discuss → approved label → then PR is welcome
3. **PR requirements:**
   - Must have tests (for new logic)
   - Must pass CI
   - Must have a CHANGELOG entry (for non-trivial changes)
   - Squash merge (1 PR = 1 commit in main)

### 9.4 Communication channels

- **GitHub Issues:** Bugs, feature requests
- **GitHub Discussions:** Q&A, ideas
- **Discord/Slack:** Real-time community (set up when ≥20 active users)
- **Twitter/X:** Release announcements (linking to CHANGELOG)
- **dev.to / blog:** Tutorials, use cases

---

## 10. Security

### 10.1 Threat model

| Threat | Mitigation |
|---|---|
| API key leaked via git history | Pre-commit hook checks for `sk_` pattern; `.gitignore` covers config files |
| API key leaked via process list | Do not accept `--api-key` as a CLI flag; use env var or config file |
| API key leaked via logs | Redact `Authorization` header in verbose mode |
| Config file readable by other users | `chmod 600` enforced on create/update |
| Man-in-the-middle | Force HTTPS; do not allow `http://` URLs |
| Compromised binary distribution | SHA256 checksum in GitHub Releases; binary signing (Phase 2) |
| Supply chain attack (malicious dep) | Dependabot + manual review of new deps; `go.sum` verification |

### 10.2 Security disclosure

`SECURITY.md`:

```markdown
# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability, please email **security@seli.app**.

**Do NOT open a public GitHub issue.**

We will:
- Acknowledge receipt within 48 hours
- Provide a detailed response within 7 days
- Issue a fix and CVE within 30 days (severity-dependent)

## Supported Versions

| Version | Supported |
|---------|-----------|
| 1.x     | ✅        |
| 0.x     | ❌        |
```

### 10.3 Dependencies

- **Whitelist approach**: Every new dependency must be reviewed by a maintainer
- **Auto-update**: Dependabot opens PRs for patch/minor updates
- **Latest Stable Policy**: Always use the latest stable version of Go (both in `go.mod` and CI workflows) and core tooling (GitHub Actions, linters, scanners) to ensure compatibility and avoid CI toolchain mismatches. Dependabot PRs that bump action versions must be merged promptly — stale action versions are the #1 cause of CI breakage on newer GitHub runners.
- **Action version reference** (update when new majors ship):
  | Action | Minimum version | Notes |
  |---|---|---|
  | `actions/checkout` | `@v6` | Node.js 24; `@v4` and below fail on modern runners |
  | `actions/setup-go` | `@v6` | Required for Go ≥1.26 |
  | `golangci/golangci-lint-action` | `@v9` | |
  | `github/codeql-action/*` | `@v4` | |
  | `goreleaser/goreleaser-action` | `@v7` | |
- **`govulncheck`**: Run via `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` directly — do NOT use `golang/govulncheck-action` (its nested checkout breaks on PR events).
- **Major updates**: Manual review + testing

---

## 11. Roadmap

### Phase 1 — MVP (2–3 weeks)

**Goal:** Demoable + ready for internal dogfooding

- [ ] Project scaffolding (`cmd/`, `internal/`, `go.mod`)
- [ ] `auth login` / `auth logout` / `auth whoami`
- [ ] `config set` / `config get` / `config set-default-tenant`
- [ ] `tenants list`
- [ ] `tasks list` / `tasks get` / `tasks create`
- [ ] `boards list`
- [ ] Global flags: `--tenant` / `--no-tenant` / `--profile` / `--output`
- [ ] Output: `--output table|json|quiet` with dynamic Tenant column
- [ ] CI/CD setup (GitHub Actions + GoReleaser)
- [ ] Internal release (repo still private)

### Phase 2 — Public Launch (4–6 weeks after Phase 1)

**Goal:** Public repo + accepting external contributors

- [ ] `members list`
- [ ] `products list` (with `--updated-since` delta sync)
- [ ] `boards get` (with lists)
- [ ] `tasks search` (full-text)
- [ ] `--output yaml`
- [ ] Multi-profile support
- [ ] Auto-pagination (`--all` flag)
- [ ] Install script + Docker image
- [ ] README, CONTRIBUTING, SECURITY, CODE_OF_CONDUCT
- [ ] Compliance check (no internal info leaked)
- [ ] **Public repo launch**

### Phase 3 — AI-native (2–3 months after Phase 2)

**Goal:** Become the standard tool for third-party AI agents

- [ ] **`seli mcp serve`** — expose the CLI as an MCP server so AI agents (Claude, Cursor, Copilot, etc.) can call it directly via the MCP protocol instead of shell exec. This is the **endgame** — any MCP-compatible agent works out of the box with no integration code.
- [ ] Stdin support: `echo '{"title":"..."}' | seli tasks create --from-stdin`
- [ ] Batch operations: `seli tasks create --file tasks.json`
- [ ] Homebrew tap, Scoop bucket
- [ ] Shell completion (bash, zsh, fish)
- [ ] Auto-update mechanism

### Phase 4 — Beyond (future)

- [ ] WMS commands (`seli wms inbound create`, `seli wms outbound list`)
- [ ] Purchasing commands (`seli po list`)
- [ ] Interactive mode (`seli` with no args → TUI)
- [ ] Plugin system for community extensions

---

## 12. Success Metrics

### Phase 1 (Internal)

- 100% of Seli dev team able to install and use
- Zero crashes across 3 OSes (Linux, macOS, Windows)
- Startup time < 50ms

### Phase 2 (Public Launch)

- 100+ GitHub stars in the first month
- ≥10 external contributors with merged PRs
- ≥3 case studies from third-parties (n8n workflow, custom agent, etc.)

### Phase 3 (AI-native)

- ≥50 production deployments (tracked via opt-in telemetry)
- Featured in ≥1 AI agent framework's documentation (LangChain, CrewAI, AutoGPT, etc.)
- Listed on `awesome-mcp-servers` GitHub list

---

## 13. Open Questions

To resolve during early development:

1. **Opt-in telemetry:** Do we want to track anonymized usage stats to know which commands are popular? (gcloud does; kubectl does not)
2. **Update notifications:** Should the SDK check for new versions and print a warning? (gh CLI does this)
3. **Documentation language:** English only, or also Vietnamese?
4. **CLA for contributors:** Required or not? (Stripe and Vercel require one; Supabase does not)

---

## 14. References

### API reference

- [`api/openapi.json`](./api/openapi.json) — OpenAPI spec for the Seli Public API (source of truth for endpoints and schemas)

### External references

- [Cobra docs](https://cobra.dev/) — CLI framework
- [GoReleaser docs](https://goreleaser.com/) — Release automation
- [Stripe CLI](https://github.com/stripe/stripe-cli) — Reference implementation (Go)
- [gh CLI](https://github.com/cli/cli) — Reference implementation (Go)
- [gcloud SDK](https://cloud.google.com/sdk) — UX patterns
- [Keep a Changelog](https://keepachangelog.com/) — CHANGELOG format
- [SemVer](https://semver.org/) — Versioning
- [Contributor Covenant](https://www.contributor-covenant.org/) — Code of Conduct

---

## 15. Appendix: Initial command list

Commands planned for Phase 1 + Phase 2. **This is the initial scope, not the final command set** — more commands will be added as the Seli Public API exposes new endpoints.

```
seli
├── auth
│   ├── login                    [P1]  Login with sk_ API key
│   ├── logout                   [P1]  Remove credential from config
│   └── whoami                   [P1]  Show current user
│
├── config
│   ├── set                      [P1]  Set config value
│   ├── get                      [P1]  Get config value
│   ├── set-default-tenant       [P1]  Set the default tenant code
│   ├── unset-default-tenant     [P1]  Clear the default tenant
│   ├── list-profiles            [P2]  List all profiles
│   └── use-profile              [P2]  Switch active profile
│
├── tenants
│   └── list                     [P1]  List tenants the user can access
│
├── tasks
│   ├── list                     [P1]  List tasks
│   ├── get                      [P1]  Get task by ID
│   ├── create                   [P1]  Create new task
│   ├── update                   [P2]  Update task
│   └── search                   [P2]  Full-text search
│
├── boards
│   ├── list                     [P1]  List boards
│   └── get                      [P2]  Get board detail (with lists)
│
├── members
│   └── list                     [P2]  List tenant members
│
├── products
│   └── list                     [P2]  List products (supports delta sync)
│
├── version                      [P1]  Print version
├── help                         [P1]  Show help
└── completion                   [P2]  Generate shell completion
```

**Global flags on every command:** `--tenant`, `--no-tenant`, `--profile`, `--output`, `--api-url`.

---

**End of document.**

> Questions / feedback: open an issue at `github.com/vtech-com/seli-api-sdk` or contact Luu Trong directly.
