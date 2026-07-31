# Building the ground truth

Every claim in `skills/seli-api/` must trace to one of these sources. Build the inventory before
reading the skill — auditing prose against memory is how drift survives a sync.

## Source of authority, per claim type

| Claim in the skill | Authoritative source | Not authoritative |
|---|---|---|
| A command or flag exists, its spelling | the built binary's `--help` | `docs/project-context.md §5.4` |
| Exit code meanings | `cmd/helpers.go` → `ExitCodeFor` | the skill's own prose |
| Which endpoints require a tenant | `api/openapi.json` + the early-reject in `cmd/<group>.go` | `docs/project-context.md §5.3` (documented as drifted) |
| Field names, types, nullability, enums | `api/openapi.json` `components.schemas` | sample responses |
| Output envelope shape | `cmd/helpers.go` (`envelope`, `writeEnvelopeWithMeta`) | — |
| Pagination keys (`meta.pagination.total`, …) | the code that builds `meta` + spec response schema | — |
| An endpoint exists but the CLI doesn't wrap it | `api/openapi.json` vs. the help tree | — |
| An endpoint doesn't exist server-side | `docs/api-coverage-gaps.md` | absence from the CLI |

`docs/project-context.md` is a design document, not a contract. It has been wrong about tenant
requirements before (see `docs/plans/001-health-members-commands.md §2.1`). Use it for intent,
never for facts.

## 1. Command surface

```bash
make build                       # ./dist/seli — always audit against a fresh build
./dist/seli version
./dist/seli --help
```

Then walk one level at a time, only as deep as the groups that exist:

```bash
for g in $(./dist/seli --help | awk '/^  [a-z]/ {print $1}'); do
  echo "=== $g ==="; ./dist/seli "$g" --help
done
```

Record: group names, leaf command names, each leaf's flags and required args. Cross-check against
the source of registration — `grep -n "AddCommand" cmd/*.go` — to catch a command that exists but
is hidden or misregistered.

## 2. Spec surface

```bash
jq -r '.paths | to_entries[] | .key as $p | .value | to_entries[]
       | select(.key | IN("get","post","put","patch","delete"))
       | "\(.key|ascii_upcase) \($p)"' api/openapi.json

shasum -a 256 api/openapi.json    # goes into .sync.json
```

Diff spec paths against the help tree. Every path with no command is either a known gap (belongs
in `docs/api-coverage-gaps.md`) or a signal that `api-command-builder` has work to do — report it,
don't paper over it in the skill.

For a schema the skill describes (currently only members), re-derive fields and nullability:

```bash
jq -r '.components.schemas.Member as $s | ($s.required // []) as $req
       | $s.properties | to_entries[]
       | "\(.key)\t\(.value.type)\tnullable=\(.value.nullable // false)\trequired=\((.key|IN($req[])))"' \
      api/openapi.json
```

## 3. Tenant rules

Two things must agree, and the skill states both:

```bash
# what the spec declares
jq -r '.paths[]? | to_entries[]? | select(.value.parameters?)
       | .value.parameters[] | select(.in=="header" and .name=="X-Tenant-Code")
       | "required=\(.required)"' api/openapi.json

# where the CLI rejects locally, before the request
grep -rn "RequiresTenant\|resolveTenant\|failValidation" cmd/ internal/api/
```

A spec-optional endpoint that the CLI still rejects locally is a *CLI* behaviour the skill must
describe as such. Never describe the server's rule when the operator will actually hit the client's.

## 4. Exit codes

```bash
sed -n '/func ExitCodeFor/,/^}/p' cmd/helpers.go
```

Transcribe the mapping exactly. Today: 500+→6 (transport), 401→2, 403→3, 404→4, 400→5, 429→7,
everything else→1. Any exit code the skill teaches must appear in that function, and any code the
function maps that an operator would plausibly hit should be taught.

## 5. Behaviour changes that leave no trace in `cmd/`

- `CHANGELOG.md` `[Unreleased]` — the intended user-visible story.
- `docs/plans/*.md` §8 outcome notes — what actually shipped vs. what the plan said, including
  `NOTE:` lines for anything deviated or blocked. These often explain *why* a rule exists.
- `internal/api/errors.go` — named error codes the server returns; the skill teaches reading
  `error.code`, so a new class of code may deserve a mention.

## 6. Sanity check before you edit

If the inventory and the skill agree everywhere, stop. Re-read `.sync.json`, confirm the diff was
genuinely empty, and report a no-op. Editing prose that is already correct costs review time and
risks introducing the drift you were sent to remove.
