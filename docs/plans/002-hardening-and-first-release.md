---
name: 002-hardening-and-first-release
status: not_started
goal: Make `seli` reachable, safe, and installable — fix the transport defects, port Capigo's proven infrastructure, add spec-drift guards, and cut v0.1.0.
description: This builds on plan 001 (health + members, the full current spec surface). It adds no new endpoints; it fixes what keeps the CLI from being used today (unreachable base URL, no timeout, dead --verbose, no release) and back-ports the client/output/guard infrastructure from capigo-api-sdk v0.31.0 so command groups added later land on a hardened base.
---

# Seli CLI SDK — Implementation Plan 002 (Hardening & First Release)

> **Milestone:** 002
> **Mapping:** `project-context.md §3–§5` (transport, auth, output contract, distribution)
> **Base:** `50a901a` on `main` — plan 001 done; build + tests pass; no tag, no release
> **Reference implementation:** `capigo-api-sdk` @ `8057818` (v0.31.0) — same scaffold lineage, ~30 releases ahead
> **Execution status:** see §8 (Execution tracking) at the bottom of this file.

---

## 1. Context

Plan 001 wrapped every operation in `api/openapi.json` (`GET /health`, `GET /members`,
`GET /members/{id}`). The CLI builds and its unit tests pass, but nobody can use it yet:

- **No server answers at the configured address.** Checked 2026-10-22:
  `api.seli.vn` (spec `servers[0].url`, the hardcoded default) and `platform.seli.app`
  (`project-context.md §5.1`) do not resolve in DNS. `seli.app` resolves but is an
  unrelated site ("Insider Trading & Congress Stock Tracker") that answers HTML to every
  path, including `/api/v1/health` and `/api/openapi`.
- **No release exists.** The repo is public, `release.yml` and `.goreleaser.yml` exist,
  but no tag has been pushed, so `scripts/install.sh` has nothing to download.
- **Transport defects** (§2.2) that are harmless only because no request has reached a server.

`capigo-api-sdk` started from the same scaffold and has since solved the same problems in
production. This plan ports the generic parts, not its domain commands.

---

## 2. Counter-review findings (pre-implementation audit)

### 2.1 Blocking decisions (need an owner answer before Phase 1 ships)

| # | Question | Why it blocks |
|---|----------|---------------|
| B1 | ~~What is the real Seli Public API base URL?~~ **Answered 2026-10-22: `https://platform.selipos.tech`.** Answers JSON (`401 E0003_UNAUTHORIZED` without a key). | — |
| B1a | ~~Path prefix `/api` or `/api/v1`?~~ **Settled 2026-10-22 with a real key (tenant `vtech-uat`): `/api`.** `GET /api/health` → 200 `{"data":{"ok":true,…}}`; `GET /api/v1/health` → 404 (HTML). The current client (base `…/api/v1`) therefore 404s on every call — Phase 1 must switch to base = host, paths = spec paths verbatim. | — |
| B5 | **Backend: `GET /api/members` and `GET /api/roles` answer 500 `E0000_UNKNOWN`** for a valid key + tenant (with and without `page`/`pageSize`/`status`), 2026-10-22 ~08:25 UTC, client request ids `ba15b8f6-de14-4085-97e9-95af36912a7b`, `91b01eb7-024a-4928-bac4-14ea0bbfb0d1`. `/api/health` and `/api/capabilities` → 200; wrong tenant → 403 `E0004_FORBIDDEN`. | ~~Blocking~~ **Resolved 2026-10-22 08:54 UTC**: both answer 200; `meta.pagination {page,pageSize,total,totalPages}` and member fields match `internal/api/models.go` exactly. |
| B1b | The live spec's `servers[0].url` is `https://api.seli.app`, which does not resolve. Backend should change it to `https://platform.selipos.tech`; until then the CLI default must not be read from `servers[0]`. | `update-spec` would otherwise reintroduce a dead default. |
| B1c | **Decided 2026-10-22: the host is configuration, never code.** The domain will change, so no source file, test, or doc hardcodes `platform.selipos.tech`. See §6 "Base URL from env". | — |
| B2 | ~~Where is the OpenAPI document served?~~ **Answered: `https://platform.selipos.tech/api/openapi`** (public, no auth, `info.version: 2026-10-21`). | — |
| B3 | How does a user obtain an API key? Prefix is `ssk_` (from the live spec's `bearerAuth.bearerFormat`). | `auth login` and the README quickstart need it; `verify-api` needs a test key. |
| B4 | Which domain/email does Seli actually own? | `SECURITY.md` (`security@seli.app`), `.goreleaser.yml` (`homepage`, `maintainer`), README badges reference `seli.app`, which belongs to an unrelated product. Shipping these sends security reports to a stranger. |

### 2.1a The committed spec is three versions behind

Live spec (`/api/openapi`, 2026-10-21) declares 10 operations; `api/openapi.json` has 3.
New on the server, unwrapped by the CLI:

| Operation | Summary |
|---|---|
| `GET /api/capabilities` | List declared capabilities |
| `GET /api/members/{id}/permissions` | Get a member's permissions |
| `GET /api/roles`, `GET /api/roles/{id}` | List / get roles |
| `GET /api/delivery/{code}` | Get waybill by code |
| `POST /api/delivery/{code}/label/100x150` | Issue a 100 × 150 mm shipping label link |
| `POST /api/delivery/{code}/slip/a4` | Issue an A4 delivery slip link |

Phase 3 refreshes the spec, so the path-coverage guard will list all seven as unwrapped
on purpose; wrapping them is plan 003 (via `api-command-builder`), not this plan.

### 2.2 Transport defects in the current client

| # | Defect | Fix (Capigo reference) |
|---|--------|-----|
| 1 | `--verbose` is registered (`cmd/root.go:52`) but nothing reads it — help promises a request/response trace that never prints. | `Client.EnableVerbose(io.Writer)`, Authorization always redacted (`internal/api/client.go` in Capigo). |
| 2 | `http.DefaultClient` has no timeout — a hung server hangs the CLI forever. | `&http.Client{Timeout: 30 * time.Second}`. |
| 3 | Missing API key still sends `Authorization: Bearer ` and spends a round trip. | Diagnose locally: exit 2, `AUTH_MISSING_HEADER`, message naming `SELI_API_KEY` and `seli auth login`. |
| 4 | Plain `http://` base URLs are accepted, so the key travels in clear text. | Refuse non-HTTPS unless host is `localhost`/`127.0.0.1`. |
| 5 | `ExitCodeFor` uses a type assertion, so a wrapped error falls to exit 1; 409 and 422 are unmapped. | `errors.As`; add 409→8, 422→5, `context.DeadlineExceeded`/`net.Error`→6. |
| 6 | `Response.RequestID` is the client-generated id; the server's `X-Request-Id` and the error envelope's `request_id` are dropped. | Prefer the server's id; surface it in the error envelope. NOTE: prod does not echo `X-Request-Id` and its error body has no `request_id` (checked 2026-10-22), so the client-generated id is the only correlation handle — print it in every error envelope. |
| 7 | Go 1.26.5 in `go.mod` and `release.yml`. | Bump to 1.26.10 (Capigo `6668836`, GO-2026-6809). |

### 2.3 Structural gaps vs. Capigo

- **Two error types** (`api.Error`, `cmd.CLIError`) and a `Get`-only client. Every write
  command added later would need a second code path. Capigo uses one `*api.APIError` and a
  single `send()` funnel behind `Do`/`DoWithHeaders`.
- **Thin error envelope** — only `code` and `message`. Capigo's `internal/output` adds
  `request_id`, `http_status`, `raw` (verbatim server body), and catalog-driven `next` /
  `capability_note`, so an agent can self-diagnose without `--verbose`.
- **No partial-result contract** — `members list --all` that fails on page N discards
  pages 1..N-1. Capigo's `output.WritePartial` prints the rows with an `error` key above them.
- **Tenant is invisible in output** — the caller cannot see which tenant a call resolved to
  or from where. Capigo puts `tenant` and `tenant_source` (`flag|env|config`) in `meta`.
- **No `auth` group** — key is set via `config set api_key`. Capigo ships `auth login/logout`.
- **No tests in `cmd/`** and no spec-drift guards. Capigo's three OpenAPI coverage tests
  (operations, query params, body fields) fail CI the moment the spec outgrows the CLI.
- **`make update-spec` does not exist**, though README and the bundled skill refer to it.
- **No `CLAUDE.md` / `AGENTS.md`**, so agents working in the repo don't know the
  "skill changes ship in the same PR" and "logic lives in `internal/`" rules.
- **`docs/api-coverage-gaps.md` is a copy of Capigo's** (products, tasks, `/pcms/*`) and
  describes commands this repo never had.

### 2.4 Not ported (deliberately)

Capigo domain groups (products, tasks, boards, WMS, catalog), multipart upload/download,
and `Idempotency-Key` handling — Seli's spec has no such operations. Port them only when a
Seli endpoint needs them.

---

## 3. Deliverable scope

### Files modified
- `internal/api/client.go` — timeout, HTTPS check, verbose trace, `Do`/`DoWithHeaders`/`send`, server request id
- `internal/api/errors.go` — single `APIError` with `RawBody`, `RequestID`, `LocalDiagnosis`; `ExitCodeFor` moves here
- `cmd/helpers.go` — `buildClient()` (key check, URL precedence), `renderCLIError` via `internal/output`; drop `CLIError`
- `cmd/root.go` — getting-started help; wire `--verbose`
- `cmd/health.go`, `cmd/members.go` — use `output.Write` / `WritePartial`, tenant in `meta`
- `internal/version/version.go` — `DefaultAPIURL` var (empty in source, set by ldflags)
- `cmd/version.go` — print the baked-in default API URL
- `go.mod`, `.github/workflows/{ci,release}.yml` — Go 1.26.10; `release.yml` passes `vars.SELI_DEFAULT_API_URL`
- `Makefile` — `SELI_DEFAULT_API_URL` in `LDFLAGS`; `update-spec` (from `SELI_SPEC_URL`), `verify-api`, `skill-package`
- `.goreleaser.yml` — `DefaultAPIURL` ldflag from env; real homepage/maintainer (B4)
- `.env.example` (new), `.gitignore` — `.env`
- `SECURITY.md`, `README.md`, `docs/project-context.md` — real URLs/contacts (B1, B4)
- `docs/api-coverage-gaps.md` — rewrite for Seli
- `skills/seli-api/**` — sync with every behavior change (via `seli-api-skill-maintainer`)
- `CHANGELOG.md`

### Files created
- `internal/output/json.go` (+ test) — envelope, `Meta`, `ErrorDetail`, `WritePartial`
- `internal/api/error_catalog.go` (+ test) — empty-but-wired; entries only from backend error codes, never guessed
- `cmd/meta.go` — `tenantSource`, `itemMeta`, `listMeta`
- `cmd/auth.go` (+ test) — `auth login --key`, `auth logout`
- `cmd/help_topics.go` — `seli help tenancy|output|exit-codes`
- `cmd/openapi_path_coverage_test.go`, `cmd/openapi_coverage_test.go`, `cmd/openapi_body_coverage_test.go`
- `cmd/help_skeleton_test.go`, `cmd/subcommand_unique_test.go`, `cmd/unknown_command_test.go`
- `scripts/verify_api.py` — live spec-vs-server check (read-only)
- `.github/workflows/release-on-merge.yml` — merging `release/vX.Y.Z` into `main` tags and releases
- `CLAUDE.md`, `AGENTS.md`

---

## 4. Release & distribution contract

- **Version:** first tag `v0.1.0`. Semver; pre-1.0 minor bumps may break.
- **Branch model (from Capigo):** work branches (`feat/`, `fix/`, `docs/`, `chore/`) PR into
  `develop`; `main` only receives release PRs. CI runs on PRs into both.
- **Release flow (from Capigo):** branch `release/vX.Y.Z` from `develop` → bump `CHANGELOG.md` `## [X.Y.Z] — date`
  → PR into `main` → merge → `release-on-merge.yml` tags the merge commit → `release.yml`
  runs GoReleaser (verifies a matching CHANGELOG section first). Nobody pushes tags by hand.
- **Artifacts:** `seli_<ver>_<os>_<arch>.tar.gz|zip` for linux/darwin/windows × amd64/arm64,
  `.deb`, `checksums.txt`.
- **Install paths:**
  - `curl -fsSL https://raw.githubusercontent.com/vtech-com/seli-api-sdk/main/scripts/install.sh | sh`
  - `go install github.com/vtech-com/seli-api-sdk@latest`
  - Agent skill: `npx skills add vtech-com/seli-api-sdk --skill seli-api` (zip via `make skill-package` as fallback)
- **"Usable" means** a fresh machine can: install via the script → `seli auth login --key …`
  → `seli config set-default-tenant <code>` → `seli health` exits 0 against prod →
  `seli members list --all` returns the real count, and an agent with the skill installed
  does the same unaided.

---

## 5. New error codes

Local (CLI-detected, server never called):

```go
const (
    ErrAuthMissingHeader = "AUTH_MISSING_HEADER" // no key configured; exit 2
    ErrInsecureBaseURL   = "INSECURE_BASE_URL"   // non-HTTPS, non-loopback --api-url; exit 5
)
```

Server codes go into `error_catalog.go` only when sourced from the Seli backend's error
code list (ask with B3). Never copy Capigo's `E9xxx` codes.

---

## 6. Implementation details

### Phase 0 — Unblock (owner decisions, no code)
Answer B1–B4. Confirm `seli health` with `curl` against the real host using a real key
before any code points at it.

### Base URL from env (decision B1c)

The host appears in exactly two places, both outside source code: a GitHub repo
variable and the developer's shell. Changing domain = change the variable, cut a release.

| Layer | Variable | Read by | Precedence |
|---|---|---|---|
| Runtime | `SELI_API_URL` (already exists) / `--api-url` / profile `api_url` | `buildClient()` | flag > env > profile > build default |
| Build time | `SELI_DEFAULT_API_URL` → `-X …/internal/version.DefaultAPIURL=…` | `Makefile` `LDFLAGS`, `.goreleaser.yml` ldflags | fallback only |
| Release | repo variable `vars.SELI_DEFAULT_API_URL` | `release.yml` passes it to GoReleaser as env | — |
| Spec sync | `SELI_SPEC_URL` (default `$(SELI_DEFAULT_API_URL)/api/openapi`) | `make update-spec` | — |

- `internal/api/client.go` loses its `defaultBaseURL` const; the value comes from
  `internal/version.DefaultAPIURL` (a `var`, empty in source).
- If every layer is empty (e.g. a bare `go build` / `go install` without the env), the CLI
  fails locally — exit 5, code `API_URL_MISSING`, message naming `SELI_API_URL` — and sends
  nothing. Never fall back to the spec's `servers[0].url` (B1b).
- `release.yml` fails fast if `vars.SELI_DEFAULT_API_URL` is unset, so a release can never
  ship without a default.
- Base URL holds the host only (`https://platform.selipos.tech`); the `/api` vs `/api/v1`
  prefix (B1a) lives in code next to the paths, since it changes with the API version, not
  the domain.
- `.env.example` documents `SELI_API_URL`, `SELI_API_KEY`, `SELI_TENANT`,
  `SELI_DEFAULT_API_URL`; `.env` stays gitignored. `seli version` prints the baked-in
  default so a user can see which host a binary targets.
- Tests use `httptest` URLs only; no test names the real host.

### Phase 1 — Hardening PR (small, ship first)
§2.2 items 1–7, base URL from env (above), contact/domain fix. Each item gets an
`httptest` case (timeout, redacted verbose, missing key → exit 2 with no request sent,
`http://` refused, 409→8, wrapped error keeps its code).

### Phase 2 — Client & output refactor
Port `internal/output`, unify on `api.APIError`, `buildClient()`, `meta.go`, `auth` group,
`WritePartial` for `members list --all`. Output shape stays `{data, meta}` — additive only
(`meta.tenant`, `meta.tenant_source`, richer `error`), so it is not a breaking change.

### Phase 3 — Drift guards
`make update-spec` (pretty-printed, stable diffs), three OpenAPI coverage tests, help
skeleton/uniqueness tests, `make verify-api`. CI already runs `make test`, so the guards
gate PRs with no workflow change.

### Phase 4 — Docs & agent surface
`CLAUDE.md`/`AGENTS.md`, rewritten `api-coverage-gaps.md`, `project-context.md` URLs,
help topics, README quickstart rewritten around `auth login`. Run
`seli-api-skill-maintainer` to resync `skills/seli-api/` and its evals.

### Phase 5 — Release v0.1.0
Add `release-on-merge.yml`, CHANGELOG check in `release.yml`. Dry run with
`make release-snapshot`. Cut `release/v0.1.0`, merge, then smoke-test on a clean
macOS and Linux box using only the README (§4 "usable" checklist).

---

## 7. Implementation order

1. Phase 0 — owner answers B1–B4 (blocking)
2. Phase 1 — hardening PR (§2.2 items 1–7, URLs, contacts) + httptest coverage
3. Phase 2 — `internal/output`, unified `APIError`, `buildClient`, `meta`, `auth`, `WritePartial`
4. Phase 3 — `update-spec`, OpenAPI coverage tests, help tests, `verify-api`
5. Phase 4 — `CLAUDE.md`/`AGENTS.md`, docs rewrite, help topics, skill resync
6. Phase 5 — release workflow, snapshot dry run, `v0.1.0`, clean-machine smoke test
7. Verify each phase: `make build`, `make test`, `make lint`; Phase 1+ also `make verify-api` against prod

---

## 8. Execution tracking

| # | Step | Status |
|---|------|--------|
| 1 | Phase 0 — B1 base URL, B2 spec URL, B3 API key, B4 domain/contacts | IN PROGRESS — NOTE: B1/B2/B1a/B1c settled (host in env, prefix `/api`); B5 resolved; open: B1b dead `servers[0]`, B4 domain/contacts |
| 2 | Phase 1 — hardening PR | TODO |
| 3 | Phase 2 — client & output refactor, `auth` group | TODO |
| 4 | Phase 3 — drift guards, `update-spec`, `verify-api` | TODO |
| 5 | Phase 4 — docs, agent files, skill resync | TODO |
| 6 | Phase 5 — release workflow + `v0.1.0` + smoke test | TODO |
| 7 | Verify: build + test + lint + verify-api | TODO |

---

## 9. Out of scope (deferred to 003+)

- New command groups — `roles`, `members permissions`, `capabilities`, `delivery` (§2.1a) go to plan 003 via `api-command-builder`.
- Homebrew tap, Scoop, signed/notarized macOS binaries.
- Write-path infrastructure (`--from-json`, `Idempotency-Key`, uploads) — port from Capigo with the first Seli write endpoint.
- Retry/backoff on 429/5xx.
