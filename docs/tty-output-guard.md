# Design note: non-TTY output guard (`-o json` nudge)

Status: **implemented** (see `cmd/output_hint.go`). Follow-up to the v0.14.x self-diagnosing
work and the v0.14.1 skill output-mode edits.

> **Implementation note.** Built per this spec with one refinement: command groups whose
> output is not a JSON-parseable resource (`version`, `config` — they print a scalar/fixed text
> and ignore `-o json`) are **exempt**, so the nudge is never a false positive that would train
> the agent to ignore it. `health` and `auth whoami/login` honour `-o json` and are not exempt.
> The open question below (does openclaw surface command stderr to Tấm?) still governs whether
> this guard is *sufficient* — it was decoupled from the build because a stderr-only advisory is
> harmless in either case, so shipping it does not depend on the answer.

## Problem

The CLI's primary consumer is an AI agent (Tấm) that reads **stdout**. A recurring failure
class: the agent runs a command that produces the default **table** output but redirects or
pipes it into a machine parser —

```
seli products list ... > /tmp/file.json && python3 parse.py   # no -o json
```

The file holds a text table (with a `Server time:` line and a `Tenant: … · Total: N` footer),
not JSON, so `json.load()` fails. In the observed incident the agent then **fabricated** the
failure (claimed a JSON body *with* a `Server time:` prefix — a state that cannot occur) and
proposed a fragile workaround (`json.loads(''.join(lines[1:]))`) that would corrupt real
`-o json` output, where `Server time:` is on stderr and stdout opens with `{`.

This is the one variant in this class the tool currently **cannot** catch: the CLI exits 0 —
it printed a perfectly good table. There is no error, so the v0.14 on-stdout diagnosis block
never fires. The only failure is on the consumer's side, after the process exited.

## Goal

Give the CLI a way to flag the likely mistake *at the moment it happens*, without regressing
the "truth salient on stdout in table mode" doctrine and without breaking legitimate
table-piping.

## Non-goals

- **Do NOT flip the default output mode to JSON.** Table-mode stdout footers (`Total`,
  resolved tenant, `ACTIVE (DELETED)`, `INCOMPLETE`, missing-ids) are a deliberate investment
  so a prose-reading agent reaches the right conclusion (origin: the 43-brands-read-as-20
  incident). A JSON wall buries `total` in `meta` and is *less* salient to a skimming agent.
- **Do NOT change exit codes or stdout data.** The guard is advisory only.

## Proposed behavior

On every command, after flag parsing, emit a one-line advisory to **stderr** when ALL hold:

1. stdout is **not a TTY** (it was redirected or piped), and
2. `--output` / `-o` was **not explicitly set** (cobra `flag.Changed == false`), and
3. the resolved output mode is `table` (the default).

Suggested text:

```
[seli] stdout is not a terminal and --output was not set, so this is table text, not JSON.
If you intend to parse it, re-run with -o json. (set SELI_NO_HINTS=1 to silence)
```

Behavior is otherwise unchanged: same stdout, same exit code. If `--output table` was passed
explicitly, or mode is `json`/`quiet`, no warning — the user/agent made a deliberate choice.

## Why stderr, not stdout

The warning is a statement about the *channel*, not about tenant/data truth, so the
"truth-on-stdout" doctrine does not pull it onto stdout. More importantly, putting it on stdout
would corrupt the very pipe we are protecting and pollute legitimate `table | grep` use. stderr
is the conventional, non-destructive channel for this.

## Honest limitation (must validate before building)

The doctrine's premise is that **Tấm reads stdout and frequently ignores its skill** — and, by
extension, may ignore **stderr** too. If Tấm/openclaw does not surface stderr back into the
agent's context, this nudge is invisible to the agent and buys little. So before implementing:

- **Open question:** does openclaw forward command stderr into Tấm's transcript/context? If
  not, this guard helps humans and CI but not the actual target consumer, and the
  higher-leverage fix stays at the skill/agent layer.
- This guard is explicitly a **secondary** layer. It does not replace the skill rule ("if you
  write `>` or `|`, also write `-o json`"); it backstops it for the case the CLI can see.

A stdout-side variant was considered (so Tấm definitely reads it) and rejected: it would break
legitimate table-piping and pollute redirected files. If stderr proves invisible to Tấm, the
correct next move is agent-layer (skill reinforcement / openclaw surfacing stderr), not moving
the warning onto stdout.

## Implementation sketch

- Detection: `golang.org/x/term` → `term.IsTerminal(int(os.Stdout.Fd()))` (new dependency; or
  a small `isatty` helper). Treat error / non-character-device as "not a TTY".
- Placement: root `PersistentPreRunE`, after the global `--output` flag is parsed, so it
  applies uniformly. Read `rootCmd.PersistentFlags().Lookup("output").Changed`.
- Respect `SELI_NO_HINTS=1` (and skip entirely when `outputMode != "table"`).
- Lint gotcha (CI errcheck): guard `fmt.Fprintln(os.Stderr, …)` returns with `_, _ =`.
- Tests: table-piped-no-flag warns; `-o json` silent; explicit `--output table` silent;
  `SELI_NO_HINTS=1` silent; TTY (no redirect) silent.
- Skill sync (repo rule): when this lands, note in `cli_basics.md` that a stderr hint fires on
  non-TTY table output — so the skill and binary stay in lockstep.

## Decision needed

Build only after the openclaw-stderr open question is answered. If stderr is not surfaced to
Tấm, defer and keep the fix at the skill layer.
