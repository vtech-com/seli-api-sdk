# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Fixed

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
- The client refuses to read a response body over 16 MiB, and `members get`
  escapes the id in the request path.

### Added

- `seli auth login --key <ssk_…>` stores the API key in the active profile,
  creating the profile if the config has none, and `seli auth logout` removes it.
  The key is never echoed.
- `members list --all` prints the rows fetched so far under an `error` key when
  a page after the first fails, in one JSON document, with a non-zero exit.
- A `GETTING STARTED` section in `seli --help`.
- `make update-spec` fetches the OpenAPI document from `SELI_SPEC_URL` (default
  `$SELI_DEFAULT_API_URL/api/openapi`) and pretty-prints it.
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
