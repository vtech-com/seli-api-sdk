# Seli CLI SDK

[![CI](https://github.com/vtech-com/seli-api-sdk/actions/workflows/ci.yml/badge.svg)](https://github.com/vtech-com/seli-api-sdk/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/vtech-com/seli-api-sdk)](https://github.com/vtech-com/seli-api-sdk/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A CLI tool and Go SDK for the Seli platform — built for third-party AI agents, automation scripts, and developers who need a scriptable interface to Seli's Public API.

```bash
seli version
seli config get api_url
```

## Status

This is the scaffolding stage: project structure, build tooling, config
storage, and the cobra/viper command skeleton are in place, mirrored from an
existing sibling SDK. The Seli API client (`internal/api`) and the domain
command groups it backs (whatever Seli's resources turn out to be) are not
implemented yet — that's the next piece of work.

## Design goals (carried over from the scaffold)

- **Zero-config for AI agents** — a single JSON envelope on stdout, always
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

## License

MIT — see [LICENSE](LICENSE).
