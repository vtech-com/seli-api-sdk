# Seli CLI SDK

[![CI](https://github.com/vtech-com/seli-api-sdk/actions/workflows/ci.yml/badge.svg)](https://github.com/vtech-com/seli-api-sdk/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/vtech-com/seli-api-sdk)](https://github.com/vtech-com/seli-api-sdk/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A CLI tool and Go SDK for the Seli platform — built for third-party AI agents, automation scripts, and developers who need a scriptable interface to Seli's Public API.

```bash
seli version
seli config set-default-tenant acme
seli health --tenant acme
seli members list --tenant acme
```

## Status

Early, but no longer a scaffold. The API client (`internal/api`) is in place —
base URL resolution, injected auth and tenant headers, response decoding, and
error-to-exit-code mapping — and the first domain commands ship on top of it:

| Command | Endpoint |
|---|---|
| `seli health` | `GET /api/v1/health` |
| `seli members list` | `GET /api/v1/members` |
| `seli members get <id>` | `GET /api/v1/members/{id}` |

Plus local `seli config ...` and `seli version`. Every path in
[`api/openapi.json`](api/openapi.json) is currently wrapped; the Public API is
growing, and command groups are added as endpoints land — see
[`docs/api-coverage-gaps.md`](docs/api-coverage-gaps.md) for what the API itself
does not expose yet, and [`docs/plans/`](docs/plans/) for work in flight.

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

### Adding API commands

New command groups are built with the **`api-command-builder`** agent skill
([`.agents/skills/api-command-builder/`](.agents/skills/api-command-builder/)), so that
every command traces back to the spec and lands with a reviewed plan. In Claude Code:

```
/api-command-builder add commands for PCMS product variants
```

The skill:

1. Resolves the requirement against [`api/openapi.json`](api/openapi.json) — the only
   source of truth for paths, params, bodies, and per-endpoint tenant rules. It never
   invents an endpoint; anything the spec doesn't declare goes to
   [`docs/api-coverage-gaps.md`](docs/api-coverage-gaps.md) instead.
2. Writes a numbered implementation plan in [`docs/plans/`](docs/plans/) from
   [`docs/templates/implementation-plan.md`](docs/templates/implementation-plan.md).
3. **Stops for your approval** — no source file is touched before you sign off on the
   endpoints, command surface, and scope.
4. Implements the plan, updating its execution-tracking table as each step lands and
   marking the plan `done` once `make build`, `make test`, and `make lint` all pass.

The conventions it enforces — one JSON envelope, deterministic exit codes, tenant
resolution, `--from-json` for complex bodies — are documented in the skill's
`references/` and apply to hand-written commands too.

### The shipped agent skill

[`skills/seli-api/`](skills/seli-api/) is a product artifact, not repo tooling: it is
the skill an AI agent loads to drive the installed `seli` binary on someone else's
machine. It teaches a discovery workflow (`seli --help` is authoritative for flags),
the CLI's stable contracts, exit-code handling, and tenant resolution — deliberately
without duplicating the command tree, since the binary is self-documenting.

Because those agents can't see this repo, the skill has to stay true to the CLI. Keep
it in sync with the **`seli-api-skill-maintainer`** agent skill
([`.agents/skills/seli-api-skill-maintainer/`](.agents/skills/seli-api-skill-maintainer/)):

```
/seli-api-skill-maintainer sync the skill with the commands that just landed
```

It diffs the repo against the provenance recorded in `skills/seli-api/.sync.json`,
rebuilds the ground truth from a fresh binary, the spec, and `cmd/helpers.go`, then
fixes both directions of drift — stale claims *and* assertions the CLI has outgrown —
including `evals/evals.json`. Run it after new commands land, after `api/openapi.json`
changes, before a release, or whenever `--help` and the skill disagree.

## License

MIT — see [LICENSE](LICENSE).
