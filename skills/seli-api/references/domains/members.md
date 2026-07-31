# Member domain relationships

Read this for any member lookup, or when resolving an exact person's identity for some other
write. `seli members list` and `seli members get <id>` are live commands backed by
`GET /members` and `GET /members/{id}`.

## Read model

- A member record represents a **membership**, not a user account: `id` is the membership UUID,
  `userId` is the underlying user UUID. Use `id` when the leaf help asks for a member ID.
- `level` is `owner` or `member`. `status` is `active` or `inactive`. Both are enums — do not
  invent other values.
- `displayName`, `avatarUrl`, `contactEmail`, `contactPhone`, `jobTitle`, `department`, and
  `memberCode` are all nullable. A `null` means the field was never set for that member, not a
  broken record — do not treat it as missing data to report as an error.
- There is no `email` field distinct from `contactEmail`; when a request needs an identity
  resolved by "exact work email", match against `contactEmail`.

## Tenant scope

`X-Tenant-Code` is a **required** header on both `members list` and `members get` — the CLI
rejects the command locally (exit 5) if no tenant resolves. This is stricter than
`docs/project-context.md §5.3`, which documents `GET /members` as tenant-optional; that table has
drifted from `api/openapi.json` (see `docs/plans/001-health-members-commands.md §2.1`). Always
resolve a tenant before a member lookup — do not expect a global, cross-tenant listing.

## Resolving an exact member

1. Prefer `seli members list --tenant <code> --status active` and match on `contactEmail`
   case-insensitively. Accept only a single unambiguous match.
2. If more than one member shares a `contactEmail` (should not happen but do not assume it can't),
   or none match, ask the user rather than guessing which one they mean.
3. `seli members get <id>` only accepts a UUID — both "no such id" and "id belongs to another
   tenant" return the same 404, so a 404 here is not proof the member doesn't exist anywhere, only
   that it isn't visible in the resolved tenant.
4. Never resolve identity from the OS account, git author, API key, or local config — those are
   not Seli identities.

## Pagination

`members list` returns `meta.pagination.{page, pageSize, total, totalPages}` — there is no
`has_more` field (do not look for one, unlike some other list endpoints described elsewhere).
Use `--all` to auto-paginate, or `--page-size 1` plus `meta.pagination.total` to answer a "how
many members" question without fetching every row.

## Out of scope (API does not expose this yet)

Member invite, role change, and remove are **not** in the Public API today
(`docs/api-coverage-gaps.md`) — only `list` and `get` exist. Do not attempt to construct a write
call for member management; tell the user this is currently read-only and point them to the web
UI or the coverage-gaps note if they need it tracked.
