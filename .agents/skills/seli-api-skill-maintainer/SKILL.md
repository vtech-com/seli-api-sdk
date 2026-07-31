---
name: seli-api-skill-maintainer
description: Keep the shipped `skills/seli-api/` skill (SKILL.md, references/, evals/) accurate as the CLI, the OpenAPI spec, and the repo docs change. Use after new commands land, after `make update-spec` or any `api/openapi.json` change, when a plan in docs/plans is marked done, before a release, or whenever the installed CLI's `--help` and the skill disagree — SKILL.md §2 tells operators to "report the mismatch for the skill maintainer", and this is that maintainer. Also use for "sync the seli-api skill", "the skill is out of date", "does the skill still match the CLI".
metadata:
  domain: docs
  scope: maintenance
  maintains: skills/seli-api
  related-skills: api-command-builder
---

# seli-api skill maintainer

`skills/seli-api/` is a **product artifact**: it ships to agents that drive the `seli` binary on
someone else's machine. Those agents cannot check our repo — they trust the skill. So the skill's
job is to be *correct and small*, and this skill's job is to keep it that way as the CLI grows.

**The three rules that govern every edit:**

1. **The skill teaches a discovery workflow; it is not a command inventory.** The installed
   binary's `--help` is authoritative for flag spelling and behaviour. If an edit would duplicate
   help output, it is the wrong edit — delete rather than add.
2. **Every concrete claim must trace to a source in this repo.** Exit codes to
   `cmd/helpers.go:ExitCodeFor`. Tenant requirements to `api/openapi.json`. Field names and
   nullability to the spec schemas. See `references/ground-truth.md`.
3. **Drift is bidirectional.** The skill can be stale (missing a new group), *and* it can be
   over-specified (asserting behaviour the CLI no longer has). Hunt both.

---

## Phase 1 — Establish what changed

Read `skills/seli-api/.sync.json` — the provenance record of the last sync. Then diff reality
against it:

```bash
jq -r '.last_synced_commit' skills/seli-api/.sync.json
git log --oneline <last_synced_commit>..HEAD -- cmd/ internal/api/ api/openapi.json docs/
shasum -a 256 api/openapi.json          # compare with .spec_sha256
```

Also scan for the triggers that don't show up in a path diff:

- `docs/plans/README.md` — any plan newly `done` since the last sync ships commands.
- `CHANGELOG.md` `[Unreleased]` — user-visible behaviour changes.
- `docs/api-coverage-gaps.md` — gaps that closed (the skill may still claim a capability is absent).

If nothing changed, say so and stop. A no-op sync is a valid outcome; do not manufacture edits.

## Phase 2 — Build the ground truth

Never audit the skill against memory or against `docs/project-context.md` (which the repo itself
documents as drift-prone). Build a real inventory first — `references/ground-truth.md` has the
commands: build the binary, walk the help tree, extract the spec paths and schemas, read the exit
code table out of the source.

Produce, as working notes: current command groups and leaf commands with their flags, current spec
paths with tenant requirements, current exit-code mapping.

## Phase 3 — Audit the skill against it

Go file by file. For each concrete claim, find its source or mark it. Categories:

| Finding | Example | Action |
|---|---|---|
| **Stale** | frontmatter lists `health`/`members` but `tasks` shipped | update |
| **Wrong** | claims exit 3 for rate limits; source says 429→7 | fix, and check evals asserting it |
| **Over-specified** | documents a flag's exact values that leaf help already gives | delete, defer to help |
| **Newly needed** | a domain has a non-obvious read model agents get wrong | add a domain reference (last resort — see `references/edit-rules.md`) |
| **Orphaned** | a domain reference for a command that no longer exists | delete, with its evals |

Cover all of: `SKILL.md` frontmatter description, `SKILL.md` body §1–§6, `references/cli-conventions.md`,
every `references/domains/*.md`, and `evals/evals.json`.

The frontmatter `description` deserves its own pass — it is the only thing an agent reads when
deciding whether to load the skill. It names the current command surface explicitly, so **every
new command group is a frontmatter edit**, and it must keep its trigger phrasing (natural-language
intents, "even when the user never names the CLI", cross-language).

## Phase 4 — Apply edits

Follow `references/edit-rules.md` for what belongs in which file, and hold the existing voice:
imperative, second person, no hedging, no marketing. Keep the skill's length roughly constant —
if a section grows, something else should shrink or move to a reference.

When a claim is genuinely uncertain (the API's behaviour isn't pinned by spec or source), say so
in the skill in one clause and tell the operator how to find out (read leaf help, read the error
object) rather than asserting a guess.

## Phase 5 — Update the evals

`evals/evals.json` is the regression suite for the skill's *judgement*, not its prose. Update it
whenever behaviour changed: new exit code, new tenant rule, a new command class that agents will
misuse. Format and rules: `references/edit-rules.md §5`. Validate before finishing:

```bash
jq -e '.evals | length' skills/seli-api/evals/evals.json
jq -e '[.evals[].id] | (length == (unique | length))' skills/seli-api/evals/evals.json  # ids unique
```

## Phase 6 — Record the sync and report

Rewrite `skills/seli-api/.sync.json` with the current commit, timestamp, spec hash, spec paths,
and command inventory. Add a `CHANGELOG.md` `[Unreleased]` entry when the shipped skill changed
in a way consumers would notice.

Report to the user: what changed upstream, what you edited and why, what you deliberately left
alone, and anything you could not verify. Do not commit unless asked.

---

## Boundaries

- **This skill edits `skills/seli-api/` only.** If the audit reveals a *CLI* defect (help text
  wrong, exit code mismapped, a spec path unwrapped), report it — do not fix it here. Wrapping a
  new endpoint is `api-command-builder`'s job.
- **Do not weaken the safety rules** in the shipped skill — no credentials in chat, no raw HTTP
  for routine operations, no destructive recovery invention, no installing software unasked.
  These are not stylistic; they survive every sync.
- **Do not add repo-internal context** to the shipped skill. Consumers have no `docs/plans/`,
  no `cmd/`, no monorepo. A citation like "see `docs/plans/001-…`" is acceptable only as
  provenance inside a domain reference, never as an instruction to go read it.

## References

| File | Load when |
|---|---|
| `references/ground-truth.md` | Building the command/spec/exit-code inventory to audit against |
| `references/edit-rules.md` | Deciding what goes in frontmatter vs SKILL.md vs conventions vs domains vs evals |
