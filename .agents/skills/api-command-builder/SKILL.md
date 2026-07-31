---
name: api-command-builder
description: Build one or more `seli` CLI commands from a plain-language requirement, using api/openapi.json as the authoritative contract. Writes a numbered implementation plan in docs/plans (from docs/templates/implementation-plan.md), stops for the user's approval, then implements it and logs progress into the plan until every step is DONE. Use whenever the ask is "add a command for X", "wrap endpoint Y", "surface Z in the CLI", or any change to cmd/*.go driven by the API spec.
metadata:
  domain: cli
  scope: plan-then-implement
  related-skills: golang-pro
---

# API Command Builder

Turns a requirement like *"add commands for warehouse inventory"* into shipped `seli`
subcommands, via a plan the user approves first.

**Hard rules — these are what make this skill different from just writing code:**

1. `api/openapi.json` is the only source of truth for paths, methods, params, bodies,
   responses, and tenant requirements. Never invent an endpoint, a field, or a query
   parameter that is not in the spec (`docs/project-context.md §5.5`).
2. Every run produces a plan file **before** any source file is touched.
3. **Stop after the plan. Ask the user to verify. Do not implement until they approve.**
4. After approval, keep the plan's §8 table current as work lands, and set
   `status: done` in the frontmatter only when every step is DONE or explicitly noted.

---

## Phase 1 — Resolve the requirement against the spec

1. Locate the spec at `api/openapi.json`.
   - If it is missing, **stop** and tell the user: the spec is the contract; offer
     `make update-spec` (pulls prod openapi per `docs/api-coverage-gaps.md`) or ask
     where the spec lives. Do not proceed from memory or from the doc tables —
     `docs/project-context.md §5.4` is illustrative and drifts.
2. Map the requirement to concrete operations. See `references/spec-reading.md` for the
   `jq` recipes — path listing, operation extraction, schema resolution, tenant detection.
3. Decide the command surface: which command groups, which subcommands, which flags.
   Follow `references/command-conventions.md` — it encodes the envelope, exit codes,
   tenant rules, pagination, and file layout this repo already uses.
4. Record what the requirement asks for that the spec **cannot** support. That belongs in
   the plan's §9 (Out of scope) and, if the endpoint simply doesn't exist server-side, in
   `docs/api-coverage-gaps.md`. Never fill the gap by guessing a URL.

If the requirement covers several unrelated resources, prefer **one plan per resource
group** (one plan → one coherent milestone) and say so up front; write the first plan,
get approval, implement, then move to the next. One plan may cover multiple endpoints of
the same resource — that is the normal case.

## Phase 2 — Write the numbered plan

1. Read `docs/templates/implementation-plan.md` fresh each run — it is the template of
   record and may have changed.
2. Pick the next index: `ls docs/plans` → highest `NNN-` prefix + 1, zero-padded to 3.
   Create `docs/plans/` if it does not exist.
3. File name: `docs/plans/NNN-<slug>.md` where `<slug>` is kebab-case of the resource or
   capability (e.g. `004-warehouse-inventory-commands`).
4. Fill **every** section of the template with real content. Guidance and section-by-section
   rules: `references/plan-authoring.md`. Notably:
   - Frontmatter `name:` = the file's slug (without the `NNN-`), `status: not_started`.
   - §4 (API contract) must be transcribed from the spec, not paraphrased from docs —
     exact paths, exact field names, exact enums, exact required/optional.
   - §7 (Implementation order) and §8 (Execution tracking) mirror each other one-for-one,
     all §8 rows start `TODO`.
   - Delete sections that genuinely do not apply rather than leaving `<placeholders>`;
     say in the section heading why (e.g. "§2.1 Spec defects — none found").
5. Append a row to `docs/plans/README.md` (create it with the header if absent):
   `| NNN | [<title>](NNN-<slug>.md) | not_started | <date> |`.

## Phase 3 — Verification gate (mandatory stop)

Present to the user, in the chat, a short summary — not the whole file:

- the plan path,
- the endpoints being wrapped (method + path),
- the resulting command surface (one line per subcommand, with flags),
- anything deferred to §9 and why,
- any spec defect found in §2.1.

Then **ask for approval** with `AskUserQuestion` (approve / revise / cancel), and stop.
If they ask for changes, edit the plan file and re-present. Do not open an editor on
`cmd/`, `internal/`, or `api/` in this phase.

## Phase 4 — Implement, logging as you go

Only after explicit approval:

1. Set frontmatter `status: in_progress` and the README index row to `in_progress`.
2. Work §7 in order. For each step: flip its §8 row to `IN PROGRESS` before starting and
   to `DONE` immediately after it verifiably works. If a step cannot be done as written,
   set `BLOCKED` and add a `NOTE:` line under the table saying why and what you did
   instead — never silently reinterpret a step.
3. Go code follows the `golang-pro` skill and this repo's existing style (`cmd/config.go`
   is the closest reference for a command group; `cmd/helpers.go` for errors/output).
4. Step 10 is always verification: `make build`, `make test`, `make lint`. Report real
   output. A failing suite means the row stays `TODO`/`BLOCKED` — do not mark it DONE.
5. When every row is `DONE`: set frontmatter `status: done`, update the README index row
   to `done`, and add a one-paragraph outcome note at the bottom of §8 (what shipped,
   what deviated). If anything is `BLOCKED`, the status is `blocked`, not `done`.
6. Report to the user: plan path, commands added, test/lint results, anything left open.

## Phase 5 — Follow-through

- New endpoints wrapped → update `CHANGELOG.md` under `[Unreleased]` (it is a plan step).
- Endpoint requested but absent from the spec → add it to `docs/api-coverage-gaps.md`.
- Do not commit or push unless the user asks.

---

## References

| File | Load when |
|---|---|
| `references/spec-reading.md` | Extracting endpoints, params, schemas, tenant rules from `api/openapi.json` |
| `references/command-conventions.md` | Designing the command surface, flags, output, errors, file layout |
| `references/plan-authoring.md` | Filling the template, numbering, §8 tracking, status transitions |
