# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Must-read documents

Before doing anything non-trivial in this repo, read these in order. Do not restate or summarize them here — read the actual files.

1. **`docs/project-context.md`** — the product spec (goals, architecture, UX contract, security model, roadmap). Where a later plan or the OpenAPI file contradicts it, the plan or spec is newer and wins; fix the stale prose.
2. **`api/openapi.json`** — source of truth for every endpoint, schema, parameter, and tenant requirement. When the spec narrative and the OpenAPI file disagree, OpenAPI wins. Refresh it with `make update-spec`, never by hand.
3. **The latest `docs/plans/NNN-*.md`** — the current milestone plan. Plans are numbered (`001-*`, `002-*`, …) and live in `docs/plans/`; `docs/plans/README.md` indexes them. Always look for the highest-numbered plan to know what is actively being built.
4. **`README.md`** — user-facing install, quickstart, commands, exit codes, and AI-agent integration examples.
5. **`CONTRIBUTING.md`** — branch/PR workflow, coding conventions, commit format, release process.
6. **`CODE_OF_CONDUCT.md`** — Contributor Covenant; applies to all interactions.
7. **`SECURITY.md`** — vulnerability disclosure; do not open public issues for security bugs.
8. **`CHANGELOG.md`** — Keep a Changelog format; update `[Unreleased]` for non-trivial changes.

If you find yourself guessing at a rule (tenant headers, exit codes, output shape, command layout), the answer is in one of the files above — go read it instead of inferring.

## Repository state

The repo is a Go CLI (`seli`) for the Seli Public API. **Do not trust any hardcoded version number in prose — the current version is the latest tag (`git tag --sort=-creatordate | head -1`) and the top dated entry in `CHANGELOG.md`.** Treat the highest-numbered plan under `docs/plans/` as the current scope of work; `docs/api-coverage-gaps.md` lists what the API offers that the CLI does not yet wrap.

## API host is configuration, never code

The Seli API host will change, so it is **never written into source, tests, or docs**. It comes from `SELI_API_URL` (or `--api-url` / the profile's `api_url`) at runtime and from `SELI_DEFAULT_API_URL` at build/release time (baked in through ldflags; the release workflow reads it from a repo variable). Use `<host>` as the placeholder in prose and `httptest` URLs in tests. **Never take a default from the spec's `servers[0].url`** — it can point at a host that does not resolve, and `make update-spec` must not reintroduce it. If no host is configured, the CLI fails locally with a clear error; it does not guess.

## Agent ergonomics — output design rule

The primary consumer of this CLI is an AI agent that reads **stdout** and routinely forgets its instruction skill. Settled principle: **truth must be salient on stdout** — totals, the resolved tenant, deleted state, partial/incomplete results, and missing-ID reconciliation are printed where the agent actually looks (on stdout, in the JSON document itself — salient `data`/`meta`/`error` fields), never only on stderr, in an exit code, or in documentation. When changing any command's output, ask: *if an agent read only stdout, could it reach a confidently wrong conclusion?* If yes, fix the output, not the manual.

## Adding commands

New commands go through the **`api-command-builder`** skill (`.agents/skills/api-command-builder/`): it drives the work from `api/openapi.json`, writes a numbered plan in `docs/plans/`, stops for approval, then implements. Do not hand-wire an endpoint outside that flow.

## Bundled skill stays in sync

`skills/seli-api/` is the agent-facing operating manual for this CLI and is distributed straight from this repo (`npx skills add vtech-com/seli-api-sdk --skill seli-api`). **Any PR that changes CLI behavior (flags, output, exit semantics) must update the skill in the same PR** — use the **`seli-api-skill-maintainer`** skill (`.agents/skills/seli-api-skill-maintainer/`) to do it. A skill that describes the binary wrongly is itself a bug, and no CI check catches the drift.

## Branches and releases

Work branches (`feat/`, `fix/`, `docs/`, `chore/`) come off `develop` and PR into `develop`. `main` only receives `release/vX.Y.Z` PRs; merging one is the release (`release-on-merge.yml` tags it). Nobody pushes a tag by hand. See `CONTRIBUTING.md` for the full flow.

## Language

All artifacts written to disk in this repo are in **English** — code, comments, commits, docs, plans, CHANGELOG entries. Chat replies may mirror the user's language; files on disk may not.

## Where to put new logic

`cmd/` contains CLI glue only (flag parsing, arg validation, dispatching to `internal/`). All real logic — HTTP, tenant resolution, error mapping, output formatting, config I/O — lives under `internal/`. If you're about to write business logic inside a `cmd/*.go` file, stop and move it to `internal/`. See `CONTRIBUTING.md` for the full structure rule.

## Spawning agents / teammates

When spawning agents (via the `Agent` tool, `TeamCreate`, or any subagent), **default the model to Sonnet** (`model: "sonnet"` parameter) unless the user explicitly asks for a different model (Opus, Haiku, etc.) or the task obviously demands it (e.g. heavy reasoning that justifies Opus). Sonnet is the team default for this project — cheaper and fast enough for the Go work, doc edits, infra files, and review passes we run here. Pass `model: "sonnet"` on every `Agent` spawn.
