# Authoring the plan in `docs/plans/`

Template of record: `docs/templates/implementation-plan.md`. Re-read it every run.

## Numbering and naming

```bash
mkdir -p docs/plans
ls docs/plans | grep -Eo '^[0-9]{3}' | sort -n | tail -1   # → highest index, or empty
```

Next file is `docs/plans/<NNN>-<slug>.md` with `NNN` = highest + 1, zero-padded to three
digits, starting at `001`. `<slug>` is kebab-case and names the capability, not the ticket:
`003-pcms-variant-commands`, not `003-plan`. Never renumber or overwrite an existing plan —
if a previous plan needs changing after approval, edit that file in place; if the scope is
genuinely new, write a new numbered plan and cross-link them.

## Index file

`docs/plans/README.md` is the index. Create it if missing:

```markdown
# Implementation plans

Numbered plans produced by the `api-command-builder` skill. Each is the source of truth
for its own progress (see §8 in each file).

| # | Plan | Status | Created |
|---|------|--------|---------|
```

Append one row per plan and keep its Status in sync with the file's frontmatter.

## Frontmatter

```yaml
---
name: <slug>            # the file slug without the NNN- prefix
status: not_started     # not_started → in_progress → done | blocked
goal: <one sentence — the concrete outcome: which commands exist that didn't before>
description: <two sentences — what this builds on, what it adds>
---
```

`status` is written three times over a plan's life: `not_started` at creation,
`in_progress` when the user approves and work starts, `done`/`blocked` at the end. It must
always agree with the §8 table and with the README index row.

## Section-by-section

- **§1 Context** — what exists today, why this scope is being done now, and the
  authoritative source with its version: `api/openapi.json` (`.info.version`). Everything
  in §4 must be derivable from what you name here.
- **§2 Counter-review findings** — the pre-implementation audit. §2.1 lists spec defects
  found via `references/spec-reading.md §6` with the exact fix per row; write
  "none found" rather than deleting the heading if the spec is clean. §2.2–§2.5 cover
  client gaps, missing output types, flag design, and tenant enforcement. Any of these
  that don't apply get one line saying so.
- **§3 Deliverable scope** — exact file paths, split into modified vs created. This is the
  contract the user is approving; a file not listed here should not be touched in Phase 4
  without saying why.
- **§4 API contract** — transcribed from the spec, per operation: auth, path/query params
  with types and defaults, request body with required-ness and behavioural rules, response
  shape, response headers. Exact field names. This section is the one that must never be
  paraphrased from `docs/project-context.md`.
- **§5 Error codes** — the named constants to add to `internal/api/errors.go`, with the
  meaning of each. Note whether `ExitCodeFor` needs changing (usually it does not).
- **§6 Implementation details** — the actual Go snippets: client struct changes, request
  models (pointers + `,omitempty` for optional fields), output display types, and the
  command surface as a usage block.
- **§7 Implementation order** — numbered, dependency-ordered, ending with
  `Verify: make build, make test, make lint`. Each item must be one sitting's work.
- **§8 Execution tracking** — mirrors §7 one-for-one, same numbering, same wording. All
  rows start `TODO`.
- **§9 Out of scope** — what the requirement asked for that this plan does not deliver,
  each with a reason (endpoint absent from spec, deferred to a later phase, packaging).

Replace every `<placeholder>` with real content. A plan that still contains angle brackets
is not ready for the verification gate.

## Updating §8 during implementation

- Flip a row to `IN PROGRESS` before starting, `DONE` right after it verifiably works
  (compiles, tests green — not "written").
- `BLOCKED` requires a `NOTE:` line below the table: what blocked it, what you did instead,
  what would unblock it.
- Don't batch updates at the end. The table's value is that it is accurate mid-flight if
  the session is interrupted.
- When every row is `DONE`, add a short outcome paragraph under the table (what shipped,
  what deviated from the plan and why), set frontmatter `status: done`, and update the
  README index row. Any `BLOCKED` row means `status: blocked`, not `done`.
