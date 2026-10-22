// Package cmd — help_topics.go
//
// Cross-cutting help topics. Each is a cobra.Command with no Run: cobra never
// executes it, it only prints the Long text, and lists it under "Additional
// help topics" in the root help.
//
// These pages exist so that a fact true of many commands is stated in exactly
// one place. Command help pages reference a topic instead of restating it.
// Restating a cross-cutting contract inside every command page is how the
// pages drift apart.
//
// House rules for the text below:
//
//   - Describe what the CLI does. Rules about how a caller should behave
//     ("never call the API directly", "confirm before writing", "stop and ask")
//     belong to the caller's own policy, not to a public SDK's help.
//   - Every literal block declares what it is: `$` prefixes a command you type,
//     an un-prefixed block is what the CLI prints, and a schema says so.
//   - There is one output shape. A sentence that begins "in table mode" or "in
//     json mode" is describing a CLI that does not exist.
//   - The API host is configuration, never text on a page: write
//     <seli-platform-host> where one is needed.
package cmd

import "github.com/spf13/cobra"

var tenancyTopicCmd = &cobra.Command{
	Use:   "tenancy",
	Short: "How --tenant resolves, and which commands require it",
	Long: `How the tenant is chosen for a command, and which commands require one.

RESOLUTION ORDER
  --tenant <code>        the flag, when given
  SELI_TENANT            the environment variable
  default_tenant         the active profile in ~/.seli/config.json

  The first one set wins. --tenant is a global flag: it is accepted before or
  after the command, and is ignored by the commands that take no tenant.

  $ seli config set-default-tenant acme
  $ seli config unset-default-tenant

  A tenant code is stored as given. Nothing checks it against the API until a
  command that requires one is run, and the server answers a code the key
  cannot act for with 403 (exit 3).

WHICH COMMANDS REQUIRE A TENANT
  Always      health, members list, members get
              — the API declares X-Tenant-Code as a required header on every
              operation it has today, health included. With no tenant resolved
              the command exits 5 (TENANT_REQUIRED) without contacting the
              server.
  Never       auth, config, version, help
              — these never call the API.

SEEING WHICH TENANT WAS USED
  Every response from a tenant-scoped command names the tenant it resolved to,
  and where that came from:

      "meta": { "tenant": "acme", "tenant_source": "flag" }
      "meta": { "tenant": "acme", "tenant_source": "env" }
      "meta": { "tenant": "acme", "tenant_source": "config" }

  A tenant picked up from SELI_TENANT or from the profile is easy to forget
  about. Read meta.tenant_source when the answer is for the wrong tenant: a
  default left in the profile is a valid request, and it succeeds.

  $ seli config get default_tenant     # what the profile will fall back to

SEE ALSO
  seli help output       the envelope, and which stream carries what
  seli help exit-codes   what a non-zero exit means`,
}

var outputTopicCmd = &cobra.Command{
	Use:   "output",
	Short: "The JSON envelope, and which stream carries what",
	Long: `The one shape every command emits, and which stream carries what.

THE ENVELOPE
  Every command that succeeds prints exactly this to stdout:

      { "data": …, "meta": { … } }

  data is an array for a list and an object for a single item. The key is
  always there, and always called data, so a caller never has to know which
  kind of command it ran. An empty list is [], never null.

      $ seli members list | jq '.data[].id'    # list   → array
      $ seli members get <id> | jq '.data.id'  # single → object

  There is no output flag. stdout is JSON, always, and nothing else is ever
  written to it. Fields inside data are the API's own, with the API's names and
  casing: the CLI does not rename them.

META
  meta names the tenant the call resolved to, and where that value came from:

      tenant          the tenant code the request was scoped to
      tenant_source   flag, env (SELI_TENANT), or config (default_tenant)

  Both are absent on a command that takes no tenant, which prints "meta": {}.
  How the tenant is chosen: seli help tenancy

  A list also carries the API's own meta, passed through untouched under the
  keys the API names it by:

      pagination.page         current page, 1-based
      pagination.pageSize     items per page
      pagination.total        the full count across all pages
      pagination.totalPages   how many pages there are

  total is the count across every page, not the number of rows in data. Read
  it; do not count data[]. With --all, pagination describes the last page
  fetched.

FAILURE
  A command that fails prints an object with an error key — still JSON, still
  on stdout, so a caller that parses stdout unconditionally reads a diagnosis
  rather than a parse error:

      { "error": { "code": …, "message": …, "request_id": …,
                  "http_status": …, "raw": … } }

  A one-line summary also goes to stderr. request_id is the id the CLI sent as
  X-Request-Id (the server's own, when it sends one); quote it when reporting a
  failed call. A failure the CLI diagnosed itself, without a request, has no
  request_id and no http_status that came from a server.

A PARTIAL RESULT
  A command may fail after fetching real rows: members list --all that aborts
  on page 41 still holds forty pages. Those rows are not discarded. The
  document carries all three keys, error first:

      { "error": { … }, "data": [ … ], "meta": { … } }

  The error key is the test. A document that has one is not a complete answer,
  whatever the rows and the meta beside it may look like — pagination.total
  still reports the full count, which is exactly what success looks like.
  Check for the key, and you never have to reach for the exit code or read
  stderr to know what you are holding.

  stdout carries ONE JSON document, always. Nothing is ever appended to a
  document already printed.

  What the exit code means, and what each field of the error object carries:
  seli help exit-codes

STREAMS
  stdout    one JSON document, and nothing else. It carries data, or an error,
            or — when a command failed after fetching rows — both.
  stderr    the one-line error summary, and --verbose HTTP tracing.

  Everything a caller must act on is on stdout: which tenant a read was
  scoped to, how many records exist, whether the answer is complete. Nothing
  that matters is announced only on a stream the caller may not read, or only
  in an exit code it may not check.

SEE ALSO
  seli help exit-codes   what a non-zero exit means
  seli help tenancy      how --tenant resolves`,
}

var exitCodesTopicCmd = &cobra.Command{
	Use:   "exit-codes",
	Short: "What each exit code means",
	Long: `Every command exits with one of these codes. The codes are stable; the wording
of an error message is not.

  0   Success.
  1   General or unexpected failure — an unreadable or unwritable
      ~/.seli/config.json, a server error (HTTP 5xx), or an error the CLI
      could not classify.
  2   Authentication failed (HTTP 401). The API key is missing, malformed, or
      rejected. With no key configured the CLI does not contact the server at
      all: it exits 2 with AUTH_MISSING_HEADER. A key is stored with:
          $ seli auth login --key ssk_...
      or supplied for one call with SELI_API_KEY.
  3   Permission denied (HTTP 403). The key is valid but not allowed to act
      for that tenant.
  4   Not found (HTTP 404). No such id, or it is outside the resolved tenant:
      the API does not say which. Also a config command that needs a profile
      before one exists.
  5   Validation error (HTTP 400 or 422), or a request the CLI refused to
      send. The CLI-side causes, none of which contact the server:
          TENANT_REQUIRED     no tenant resolved for a command that needs one
          VALIDATION_ERROR    a bad flag, a bad argument, an unknown command
          INVALID_MEMBER_ID   members get was given something that is not a UUID
          API_URL_MISSING     no API host from --api-url, SELI_API_URL, the
                              profile, or the binary's build-time default
          INSECURE_BASE_URL   a plain-HTTP (not HTTPS) host other than localhost
                              or 127.0.0.1 — the key would travel in clear text
  6   Network error. The API could not be reached, the connection failed before
      a response arrived, or the request ran past its 30 second limit
      (NETWORK_ERROR).
  7   Rate limited (HTTP 429).
  8   Conflict (HTTP 409). The server refused the change because of the state
      it found; re-read the resource rather than retrying.

WHEN A COMMAND FAILS
  stdout carries a JSON object with an error key; stderr carries a one-line
  summary. The error object carries, when it has them:

      code             the stable error code: the API's own, or one of the
                       CLI-side codes above
      message          the API's own error message
      request_id       identifies the call; quote it when reporting a failure
      http_status      the response status, when the server responded
      raw              the verbatim server body
      meaning          an added interpretation, when the CLI has one for the code
      next             a concrete step, when the CLI has one for the code
      capability_note  present on a failed write. It says: a failed write does
                       NOT mean this operation is unsupported

  Only code and message are always present. meaning, next and capability_note
  come from the CLI's own catalog of error codes, which carries an entry only
  for a code the API's documented error list backs.

  --verbose prints the full HTTP request and response to stderr, with the
  Authorization header redacted.

SEE ALSO
  seli help output   the envelope, and which stream carries what`,
}

func init() {
	rootCmd.AddCommand(
		tenancyTopicCmd,
		outputTopicCmd,
		exitCodesTopicCmd,
	)
}
