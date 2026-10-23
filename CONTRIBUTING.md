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

- Go 1.26.10+
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
make update-spec     # Refresh api/openapi.json from $SELI_SPEC_URL (or $SELI_DEFAULT_API_URL/api/openapi)
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

1. Fork and branch from `develop` (`feat/`, `fix/`, `docs/`, `chore/`), and open the PR into `develop`. `main` only receives `release/vX.Y.Z` PRs — see [Release Process](#release-process)
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

Releases are managed by maintainers via [GoReleaser](https://goreleaser.com/) and GitHub Actions.
Merging a release branch into `main` is the release; nobody pushes a tag by hand.

1. Cut `release/vX.Y.Z` from `develop`. The branch name is the version, and the leading `v` is
   required. A PR from any other branch — `develop` itself, `release/0.1.0`, `chore/cut-v0.1.0` —
   merges into `main` without releasing anything, and nothing reports that it did not.
2. On that branch, move the `[Unreleased]` entries to a new `## [X.Y.Z] — YYYY-MM-DD` section in
   `CHANGELOG.md`, commit, and open a PR into `main`.
3. Review the PR and merge it with **Create a merge commit** — not squash. A squashed release
   commit gives `main` a history `develop` does not share, and the automatic back-merge into
   `develop` then conflicts. Every other PR is squash-merged.
   After the merge, `release-on-merge.yml` checks that the branch name and the
   `CHANGELOG.md` section agree and that the version is newer than the latest tag, tags the merge
   commit, publishes through `release.yml`, and merges `main` back into `develop` through a sync PR.
4. If the back-merge hits a conflict, the workflow run goes red and leaves the sync PR open: resolve
   and merge it by hand.

`release.yml` also runs on a hand-pushed `v*` tag as a fallback, but it refuses any tag that does
not point at a commit on `main` or whose `CHANGELOG.md` lacks the matching section.

The API host the released binary defaults to is not in the repository: `release.yml` reads it from
the `SELI_DEFAULT_API_URL` repository variable, and a release fails if the variable is unset.

If you notice the project is missing a tag or a release is broken, open an issue rather than trying to trigger releases yourself.
