# Contributing to Seli CLI SDK

Thank you for your interest in contributing! This guide covers everything you need to get started.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Before You Start](#before-you-start)
- [Development Setup](#development-setup)
- [Project Structure](#project-structure)
- [Making Changes](#making-changes)
- [Commit & PR Guidelines](#commit--pr-guidelines)
- [Testing](#testing)
- [Release Process](#release-process)

---

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you agree to uphold it.

---

## Before You Start

**For bug fixes and small improvements:** feel free to open a PR directly.

**For new features or significant changes:** please open a GitHub Issue first and wait for a maintainer to apply the `approved` label. This avoids wasted effort on PRs that won't be accepted.

---

## Development Setup

### Prerequisites

- Go 1.26.5+
- `golangci-lint` (`brew install golangci-lint` / [install guide](https://golangci-lint.run/usage/install/))
- `make`

### Clone and build

```bash
git clone https://github.com/vtech-com/seli-api-sdk.git
cd seli-api-sdk
go mod download
make build
```

### Common make targets

```bash
make build           # Build binary to ./dist/seli
make test            # Run unit tests
make lint            # Run golangci-lint
make fmt             # Format code (gofmt)
make check           # lint + test (run before pushing)
make clean           # Remove build artifacts
```

---

## Project Structure

```
cmd/              cobra commands — one file per command group
internal/config   on-disk profile store (~/.seli/config.json)
internal/version  build-time version metadata (ldflags-injected)
scripts/          install script
```

`internal/api` (the HTTP client for the Seli Public API) and the domain
command groups it backs are intentionally not part of this scaffold yet —
add them as the Seli API surface is built out.

---

## Making Changes

1. Fork and branch from `main`
2. Keep PRs focused — one concern per PR
3. Add or update tests for any behavior change
4. Run `make check` before opening the PR

## Commit & PR Guidelines

- Use clear, imperative commit messages (`fix: ...`, `feat: ...`, `docs: ...`)
- Fill out the PR template checklist
- Update `CHANGELOG.md` under `[Unreleased]`

## Testing

```bash
make test
```

New commands and internal packages should ship with unit tests.

## Release Process

Tags matching `v*` trigger the release workflow, which runs `goreleaser` to
build and publish binaries for Linux, macOS, and Windows (amd64/arm64).
