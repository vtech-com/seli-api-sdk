# Command conventions for the `seli` CLI

Everything here is drawn from the code already in the tree (`cmd/root.go`,
`cmd/config.go`, `cmd/helpers.go`, `internal/config/store.go`) and from
`docs/project-context.md`. Mirror the closest existing command rather than inventing a
shape. If the tree contradicts this file, **the tree wins** — re-read it and update the
plan accordingly.

## 1. Naming and structure

- One file per command group: `cmd/<resource>.go` (e.g. `cmd/products.go`).
- Group noun is plural and matches the API resource: `products`, `tasks`, `members`.
- Subcommands are verbs the user already knows: `list`, `get`, `create`, `update`,
  `delete`. Map spec methods onto them — `GET /x` → `list`, `GET /x/{id}` → `get <id>`,
  `POST /x` → `create`, `PATCH|PUT /x/{id}` → `update <id>`.
- Register with `rootCmd.AddCommand(...)` in the file's own `init()`; `Execute()` runs
  after all `init()`s (see `cmd/root.go`).
- Set `SilenceUsage`/`SilenceErrors` behaviour is already handled on `rootCmd` — don't
  re-declare per command.

## 2. Output — one envelope, always

`cmd/helpers.go` defines the only output shape:

```go
writeEnvelope(data) // → { "data": …, "meta": … } on stdout, JSON, one line
```

There is no `--output` flag and no second shape (see `rootCmd.Long`). List commands put
pagination into `meta` (`page`, `limit`, `total`, `has_more`); single-record commands
leave `meta` empty. If a new output type is needed, extend the envelope's `Meta` — do not
print anything else to stdout. Human-facing notes ("Showing 20 of 312…") go to **stderr**.

## 3. Errors and exit codes

Use the existing helpers; do not `fmt.Errorf` + `os.Exit` at call sites:

```go
failValidation("--tenant is required for %s", cmd.CommandPath()) // exit 5
fail("NOT_FOUND", "task not found", 404)                          // exit 4
```

`ExitCodeFor` currently maps 404→4, 400→5, everything else→1. When wiring real API
errors, extend that table (401→2, 403→3 per project-context) rather than branching in
command code. Error output goes to stderr **and** an error envelope on stdout — that is
`renderCLIError`'s job, already handled by `fail`.

## 4. Tenant resolution

Resolution order (`docs/project-context.md §3.4`): `--tenant` flag → `SELI_TENANT` env →
profile default in `~/.seli/config.json` (`internal/config`). Then apply the per-endpoint
rule you read out of the spec (`references/spec-reading.md §5`):

- Required-by-header endpoints → `failValidation` **before** the HTTP call. Never let the
  server return 400 for a condition the CLI can see.
- `POST /mission/tasks`-style endpoints → tenant goes in the **body** as `tenant_code`;
  the header is ignored by the server.
- Optional endpoints → send the header only when resolved; omit it entirely otherwise.

Encode the rule as a constant in `internal/api/` (e.g. `PCMSRequiresTenant`) so adding an
endpoint doesn't mean editing command logic.

## 5. Flags

- Path params are **positional args** (`seli tasks get <id>`), validated with
  `cobra.ExactArgs(1)` and a UUID check when the spec says `format: uuid`.
- Query params are flags with the spec's defaults: `--page` (1), `--limit` (20, 1–100),
  plus `--all` on list commands to auto-paginate until `has_more == false`.
- Simple request bodies get one flag per field, kebab-cased from the JSON name
  (`display_name` → `--display-name`). Required spec fields → `MarkFlagRequired`.
- Complex or nested bodies get `--from-json <file|->` (stdin when `-`), which takes the
  whole request body. When both are supplied, `--from-json` wins and item flags are
  ignored — say so in the command's help text. This is the `gh`/`stripe`/`kubectl` pattern.
- Never add a flag for a parameter the spec doesn't declare.

## 6. HTTP client

If `internal/api/` does not exist yet, creating it is part of the plan, not a side effect.
It owns: base URL resolution (`--api-url` / `SELI_API_URL` / default), the injected
headers (`Authorization: Bearer`, `X-Tenant-Code`, `User-Agent: seli-api-sdk/<v> (<os>; <arch>)`,
`X-Request-Id`), response decoding, and error mapping. Commands must not build
`http.Request`s themselves.

Preserve response headers the CLI needs (`X-Server-Time` for PCMS delta sync,
`X-Request-Id` for error reporting) on the response struct — a client that returns only
status + body silently kills those features.

## 7. Help text

Every command needs `Short`, and `Long` with USAGE, flags, and what the output contains —
match the density of `cmd/root.go` and `cmd/config.go`. AI agents read `--help`; treat it
as part of the deliverable, not decoration.

## 8. Tests

`internal/api/<resource>_test.go` — table-driven `httptest` tests per HTTP method:
happy path, 4xx mapping, tenant header presence/absence, pagination. Command-level tests
go in `cmd/` when the command has non-trivial flag logic. `make test` runs with `-race`.
See the `golang-pro` skill's `references/testing.md`.

## 9. Verification

```bash
make build && make test && make lint
```

All three must pass before a §8 row is marked DONE. `make lint` is `golangci-lint run ./...`.
