# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added

- Homebrew cask: `brew install --cask vtech-com/tap/seli` (macOS), published to
  the same tap as capigo. A release without the `TAP_GITHUB_TOKEN` secret warns
  and skips the cask instead of failing.

### Fixed

- `release.yml` passed `SELI_HOMEPAGE` / `SELI_MAINTAINER` to the wrong step, so
  the package metadata always used the fallback. They now reach GoReleaser.
- `CONTRIBUTING.md`: release PRs are merged with a merge commit, not squashed.

### Fixed

- `SECURITY.md` sent vulnerability reports to an email on a domain that belongs to
  an unrelated site, and said 0.x releases are unsupported. Reports now go
  through GitHub private vulnerability reporting, and the latest release is the
  supported one. Package `homepage` / `maintainer` come from the `SELI_HOMEPAGE`
  / `SELI_MAINTAINER` repository variables instead of being hardcoded.
- **Every API call was broken.** The base URL ended in `/api/v1` and the paths
  were `/health` and `/members`, but the service answers on `/api/health`,
  `/api/members` and `/api/members/{id}` — `/api/v1/*` is a 404. The base URL is
  now the host only and each endpoint carries its spec path verbatim.
  `api/openapi.json` is refreshed from the live spec with `make update-spec`.
- `--verbose` printed nothing. It now traces every request and response to
  stderr, with the `Authorization` header always redacted.
- A hung server hung the CLI forever. Requests now time out after 30 seconds
  (exit 6).
- A missing API key still sent `Authorization: Bearer ` and spent a round trip
  to learn it. It is now diagnosed locally — exit 2, `AUTH_MISSING_HEADER`,
  naming `SELI_API_KEY` and `seli auth login` — and nothing is sent.
- A plain `http://` base URL was accepted, sending the key in clear text. It is
  now refused (exit 5, `INSECURE_BASE_URL`) unless the host is `localhost` or
  `127.0.0.1`.
- A wrapped error fell through to exit 1, and 409 and 422 had no exit code.
  Exit codes are now resolved with `errors.As`: 409 maps to 8, 422 to 5, and a
  timeout or any other transport failure to 6.
- The server's request id was dropped. The service sends none today, so the
  id the client generates for each request (`X-Request-Id`) is now named in
  every error envelope as `request_id`.
- A failure part-way through `members list --all` discarded the pages already
  fetched. They are now printed, with an `error` key above them (see Added).
- `seli config set` help pointed at `auth login` before it existed and showed a
  base URL with a stale path.
- Generic `API_KEY`, `API_URL` and `TENANT` environment variables were honoured,
  and won over the `SELI_*` ones, so a stranger's `API_URL` could receive the
  API key. The CLI now reads only `SELI_API_KEY`, `SELI_API_URL` and
  `SELI_TENANT`.
- A redirect was followed, and Go forwards the `Authorization` header to the same
  host whatever the scheme, so an https origin redirecting to http leaked the key
  in clear text. Redirects are never followed: a 3xx is reported as
  `UNEXPECTED_REDIRECT` (exit 1) naming where it pointed.
- A base URL with a path, query, fragment or credentials produced garbage
  request URLs (`https://h/x?y=1/api/health`). It must be host only, e.g.
  `https://<host>`; anything else is refused locally with exit 5. Messages about
  a bad base URL no longer repeat it, since it can carry a password.
- `members list --page 0`, `--page-size 0` and negative values were silently
  dropped, reading page 1 or the server's default. They now exit 5.
- `members list --all` trusted the server's `totalPages` with no ceiling, and
  read a reply with no `meta.pagination` as "that was the last page". It now
  stops after 1000 pages with `PAGE_LIMIT_EXCEEDED`, and fails when pagination
  is missing; both print the rows fetched so far under an `error` key.
- Saving the config wrote to a fixed `config.json.tmp`, so a symlink planted at
  that name received the config, API key included. It now writes through a
  unique temp file created exclusively, and tightens an existing `~/.seli` to
  mode 0700 on every save.

### Changed

- The API host is configuration, never code. The base URL is the first of
  `--api-url`, `SELI_API_URL`, the profile's `api_url`, and a default baked in at
  build time from `SELI_DEFAULT_API_URL` (`-ldflags -X`, set by the Makefile and
  `.goreleaser.yml`). The hardcoded default is gone; if every layer is empty the
  CLI fails locally with exit 5 and `API_URL_MISSING`. `seli version` prints the
  baked-in default as `default_api_url`.
- The release workflow now takes `vars.SELI_DEFAULT_API_URL` and fails before
  building if it is unset or not `https://`, can be called from another workflow
  with a `tag` input, and refuses a tag that is not `vX.Y.Z`, not on `main`, or
  has no matching `CHANGELOG.md` section.
- One error type, `api.APIError` (code, message, request id, HTTP status, the
  server's raw body), replaces both `api.Error` and `cmd.CLIError`. Failures
  print `{"error": {…}}` on stdout with `request_id`, `http_status` and `raw`
  (the server's body, verbatim) beside `code` and `message`, and a one-line
  summary on stderr.
- Every command prints through `internal/output`: `{"data": …, "meta": …}`,
  indented. The API's own `meta` is passed through untouched — `members list`
  keeps its `pagination` object as the API sends it.
- `meta.tenant` and `meta.tenant_source` (`flag`, `env` or `config`) name the
  tenant every tenant-scoped command resolved to and where that came from.
- `members list --all` asks for pages of 100 unless `--page-size` is given,
  instead of 20: fewer round trips for the same rows.
- Commands are listed in the order they are registered, reads first, not
  alphabetically: `seli members --help` shows `list` before `get`.
- `seli members get --help` now shows a complete member in its OUTPUT section
  (all twelve fields, the seven nullable ones included), and `members list`
  points at it. The pages named none of them before.
- The client refuses to read a response body over 16 MiB, and `members get`
  escapes the id in the request path.
- Docs: the README, `docs/project-context.md`, the shipped `seli-api` agent skill
  (with new evals) and the `api-command-builder` conventions now describe the
  hardened CLI: `/api/...` paths, the host as configuration (`<host>`), the
  `auth login` quickstart, exit codes 0–8, the error envelope, `meta.tenant_source`,
  and the API's own `meta.pagination`.

### Added

- `seli auth login --key <ssk_…>` stores the API key in the active profile,
  creating the profile if the config has none, and `seli auth logout` removes it.
  The key is never echoed.
- `members list --all` prints the rows fetched so far under an `error` key when
  a page after the first fails, in one JSON document, with a non-zero exit.
- A `GETTING STARTED` section in `seli --help`.
- `make update-spec` fetches the OpenAPI document from `SELI_SPEC_URL` (default
  `$SELI_DEFAULT_API_URL/api/openapi`) and pretty-prints it.
- Spec-drift guards that fail `make test` (and so CI) the moment `api/openapi.json`
  outgrows the CLI:
  - every `METHOD /path` in the spec is either wrapped (`implementedOps`) or
    listed as deliberately unwrapped with a reason (`intentionallyUnwrapped`;
    the seven operations plan 003 will wrap today);
  - every query parameter of a wrapped GET has a flag, camelCase and snake_case
    becoming kebab-case (`pageSize` is `--page-size`), with a documented alias
    map for renames;
  - every body field of a wrapped write has a flag or the command registers
    `--from-json`. No wrapped operation has a body yet; the mechanism is proven
    against the spec's own POSTs so the first write command lands under guard.
- Help-page tests: every runnable command carries PURPOSE, USAGE, FLAGS and
  OUTPUT in that order, FLAGS documents exactly the flags the command defines,
  OUTPUT names `.data`, no page advertises an output mode, `list` leads its
  group, no subcommand is registered twice, and an unknown subcommand exits 5
  with a suggestion.
- `seli help tenancy`, `seli help output` and `seli help exit-codes`: the
  cross-cutting contracts (tenant resolution and `meta.tenant_source`, the
  `{error?, data, meta}` envelope and partial results, exit codes 0-8 and the
  CLI-side error codes) stated once. A test fails if a page points at a topic
  that does not exist.
- `make verify-api` (`scripts/verify_api.py`) calls the live API, read-only and
  GET only, with `SELI_API_URL`, `SELI_API_KEY` and `SELI_TENANT`. For every
  GET in the spec that needs no path parameter, and every by-id GET reachable
  from a collection, it reports MISSING, EXTRA or STATUS against the response
  schema, and checks that each wrapped command's help OUTPUT names every field
  the API returned. The key is never printed.
- `.env.example` documenting `SELI_API_URL`, `SELI_API_KEY`, `SELI_TENANT`,
  `SELI_DEFAULT_API_URL` and `SELI_SPEC_URL`, with placeholders only.
- An empty-but-wired error catalog (`internal/api/error_catalog.go`): entries
  are added only from the Seli backend's own error code list.
- Tests for the client (timeout, redacted `--verbose`, `http://` refused,
  request id, exit-code mapping) and, for the first time, end-to-end tests of
  the commands themselves in `cmd/`.
- `internal/api` HTTP client: base URL resolution, `Authorization`,
  `X-Tenant-Code`, `User-Agent`, and `X-Request-Id` header injection, and
  `ErrorEnvelope` decoding.
- `--tenant` global flag (`--tenant` / `SELI_TENANT` / `config.default_tenant`
  resolution order).
- `seli health` — `GET /api/health`.
- `seli members list` / `seli members get <id>` — `GET /api/members` and
  `GET /api/members/{id}`, with `--page`, `--page-size`, `--status`, and
  `--all` auto-pagination on `list`.
- Initial project scaffold: cobra/viper CLI skeleton, config profile store,
  build tooling (Makefile, golangci-lint, goreleaser), CI/CodeQL workflows,
  and install script — cloned from the shared project structure, without any
  Seli-specific API client or domain commands yet.
