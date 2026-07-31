# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added

- `internal/api` HTTP client: base URL resolution (`--api-url` /
  `SELI_API_URL` / profile `api_url` / spec default), `Authorization`,
  `X-Tenant-Code`, `User-Agent`, and `X-Request-Id` header injection, and
  `ErrorEnvelope` decoding.
- `--tenant` global flag (`--tenant` / `SELI_TENANT` / `config.default_tenant`
  resolution order).
- `seli health` — `GET /api/v1/health`.
- `seli members list` / `seli members get <id>` — `GET /api/v1/members` and
  `GET /api/v1/members/{id}`, with `--page`, `--page-size`, `--status`, and
  `--all` auto-pagination on `list`.
- Initial project scaffold: cobra/viper CLI skeleton, config profile store,
  build tooling (Makefile, golangci-lint, goreleaser), CI/CodeQL workflows,
  and install script — cloned from the shared project structure, without any
  Seli-specific API client or domain commands yet.
