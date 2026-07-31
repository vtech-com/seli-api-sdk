# What goes where in `skills/seli-api/`

The skill is layered so that an agent loads the least text that still makes it correct. Putting
content in the wrong layer is the most common way this skill degrades: everything ends up in
SKILL.md, SKILL.md becomes a command inventory, and the inventory goes stale.

```
skills/seli-api/
├── SKILL.md                        # always loaded — workflow + safety + when to load more
├── references/cli-conventions.md   # loaded for any real call — cross-command contracts
├── references/domains/<x>.md       # loaded only for that domain — non-obvious read models
├── evals/evals.json                # never loaded by the agent — regression suite for judgement
└── .sync.json                      # provenance of the last maintainer run
```

## 1. Frontmatter `description`

The routing surface. An agent sees only this when deciding to load the skill, so it must:

- **Name the current command surface explicitly** — every new command group edits this line.
- Keep the natural-language intents ("who's on this tenant", "look up this member by email") —
  these are what match a user's actual phrasing.
- Keep the three load-anyway clauses: intents that never name the CLI or Seli; requests in any
  language; *other* Seli domains, so the agent discovers a real gap instead of guessing raw HTTP.
- Keep the closing "also the reference for how the CLI itself works" clause — it routes
  troubleshooting here.

Do not let it become a paragraph of prose. It is a trigger list with a scope statement.

## 2. `SKILL.md` body

Belongs here: the preflight sequence, the discovery workflow, plan/execute/verify, failure
handling, when to open the spec, when to load a domain reference, and the safety rules.

Does **not** belong here:

- Any flag, enum value, or response field an agent could read from `--help`.
- A list of commands beyond what the frontmatter needs to route on.
- Repo-internal paths as instructions (consumers have no `cmd/`, no `docs/plans/`).
- Rationale longer than the rule it justifies.

Section numbers are referenced from `cli-conventions.md` ("see `../SKILL.md` section 2") and from
this repo. Renumbering sections means fixing those cross-references — grep before you renumber.

## 3. `references/cli-conventions.md`

Cross-command contracts that stay true as the surface grows: discovery, queries and filters,
tenant resolution, the output envelope, errors and partial results, pagination and counting,
`--from-json`, write safety.

The test for admission: **would this still be true after three new command groups ship?** If the
answer is "only for `members`", it is a domain reference or it is nothing. If the answer is
"read the leaf help", it does not go in at all.

When a convention becomes conditional (a flag some commands expose and others don't), phrase it
as a conditional — the file already does this ("only if the specific command exposes them") —
rather than dropping it or asserting it universally.

## 4. `references/domains/<domain>.md`

**Last resort.** A domain reference is justified only when an agent reading the leaf help would
still get it wrong. `members.md` earns its place because: `id` is a membership UUID and not the
user UUID; `contactEmail` is the only email field; nullable fields are normal rather than broken;
and the tenant rule is stricter than the design doc claims.

Requirements for any new domain file:

- Open with when to load it, and when not to.
- State only the non-obvious: identity semantics, enum sets, nullability that misleads, tenant
  scope that differs from the general rule, relationships between resources.
- Cite provenance for a claim that contradicts another repo document, as `members.md` does.
- Add the load trigger to SKILL.md §6 in the same edit — an unreferenced domain file is dead.

Delete the file, its §6 trigger, and its evals together when the domain's commands go away.

## 5. `evals/evals.json`

Schema, held exactly:

```json
{ "skill_name": "seli-api",
  "evals": [ { "id": 1, "prompt": "...", "expected_output": "...", "files": [], "assertions": ["..."] } ] }
```

- `id` is a stable integer, unique, never renumbered — new evals append.
- `prompt` describes a *situation and constraints*, not the answer, and generally forbids the
  shortcuts an agent would take ("do not call a tenant endpoint", "do not paste an API key").
- `expected_output` is one sentence on the correct behaviour.
- `assertions` are independently checkable statements about the response, including the negative
  ones — what the agent must *not* do is where the regressions actually live.

Update evals when **behaviour** changes, not when prose is rephrased. A changed exit code, a
changed tenant rule, or a new command class that agents will misuse each need an eval. Grep the
suite for any constant you edited (`grep -n "exit 5" skills/seli-api/evals/evals.json`) — an
assertion that still names the old behaviour is a silent failure.

Validate before finishing:

```bash
jq -e '[.evals[].id] | (length == (unique | length))' skills/seli-api/evals/evals.json
```

## 6. `.sync.json`

Provenance, rewritten at the end of every sync:

```json
{
  "last_synced_commit": "<git rev-parse HEAD>",
  "last_synced_at": "<YYYY-MM-DD>",
  "spec_sha256": "<shasum -a 256 api/openapi.json>",
  "spec_paths": ["GET /api/v1/health", "..."],
  "cli_commands": ["health", "members list", "..."],
  "notes": "<anything deliberately left unsynced, and why>"
}
```

`notes` is the handoff to the next run: a claim you could not verify, a spec path intentionally
not surfaced, a domain reference deferred. Do not leave it empty when something was deferred.

## 7. Voice

Imperative, second person, present tense. State the rule, then at most one clause of rationale.
No hedging ("you may want to"), no marketing, no emoji, no exclamation. Prohibitions are explicit
about the failure they prevent — "do not mislabel it as an authentication problem" beats "be
careful with exit codes". Match the surrounding text; a section that reads differently from the
rest of the file signals an unreviewed patch.
