# API coverage gaps

Living backlog of what the Seli Public API offers versus what the `seli` CLI wraps,
plus API-level problems to report to the backend. Update it whenever a command lands
or the live spec changes.

**Last assessed:** 2026-10-22
**Source:** the live spec at `<host>/api/openapi` (`info.version: 2026-10-21`, 10 operations).
`<host>` is the configured API host (`SELI_API_URL`); it is deliberately not written down here.

## Wrapped

| Endpoint | CLI command |
|---|---|
| `GET /api/health` | `seli health` |
| `GET /api/members` | `seli members list` |
| `GET /api/members/{id}` | `seli members get` |

## In the API, not yet wrapped (plan 003)

| Endpoint | Summary |
|---|---|
| `GET /api/capabilities` | List declared capabilities |
| `GET /api/members/{id}/permissions` | Get a member's permissions |
| `GET /api/roles` | List roles |
| `GET /api/roles/{id}` | Get a role |
| `GET /api/delivery/{code}` | Get a waybill by code |
| `POST /api/delivery/{code}/label/100x150` | Issue a 100 x 150 mm shipping label link |
| `POST /api/delivery/{code}/slip/a4` | Issue an A4 delivery slip link |

These are added through the `api-command-builder` skill, one numbered plan in `docs/plans/`.

## API-level issues to report to the backend

1. **`servers[0].url` is a dead host.** The live spec's `servers[0].url` points at a host
   that does not resolve in DNS. The CLI never reads its default from the spec; `make update-spec`
   must not turn it into one. Backend should publish the real address there.
2. **No request-id echo.** Responses do not return an `X-Request-Id` header and error bodies
   carry no `request_id`, so a client-generated id cannot be correlated with a server log line
   from the response alone.
3. **The committed spec was stale.** `api/openapi.json` in this repo was three versions behind
   the live spec (3 operations versus 10). Nothing in the API signals a spec change; the CLI
   needs `make update-spec` plus the coverage tests to notice drift.

## How this was determined

The live spec fetched from `<host>/api/openapi` (public, no auth), compared operation by
operation against the commands registered in `cmd/`. Behaviour of the wrapped endpoints and the
header observations in the issues above were confirmed with authenticated `GET` probes; no
write endpoint was called.
