package cmd

import (
	"os"
	"regexp"

	"github.com/spf13/cobra"
	"github.com/vtech-com/seli-api-sdk/internal/api"
	"github.com/vtech-com/seli-api-sdk/internal/output"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const (
	membersMaxPageSize = 100 // the API's maximum pageSize
	membersAllPageSize = 100 // --all fetches pages this big unless --page-size says otherwise
)

var membersCmd = &cobra.Command{
	Use:   "members",
	Short: "Manage tenant members",
	Long: `Read tenant membership records.

USAGE
  seli members <command> [<args>]`,
}

var (
	membersListPage     int
	membersListPageSize int
	membersListStatus   string
	membersListAll      bool
)

var membersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List members",
	Long: `List members for the resolved tenant.

PURPOSE
  Call GET /api/members. Page-based pagination; optional status filter.

USAGE
  seli members list [--tenant <code>] [--page N] [--page-size N] [--status active|inactive] [--all]

FLAGS
  --tenant <code>
      Tenant code to scope the request to. Required — api/openapi.json
      declares X-Tenant-Code as a required header on this endpoint. Exit 5
      if none resolve (--tenant, SELI_TENANT, config default_tenant).

  --page N
      Page number, 1-based. Default 1. Exit 5 if below 1.

  --page-size N
      Items per page, 1-100. Default 20 (100 with --all); values above 100
      are capped before the request is sent. Exit 5 if below 1.

  --status active|inactive
      Filter by membership status. Omitted by default (all statuses).

  --all
      Auto-paginate: repeat the request until page == meta.pagination.totalPages,
      concatenating results. --page is ignored when --all is set. It stops
      with an error after 1000 pages (PAGE_LIMIT_EXCEEDED; narrow the list with
      --status), and fails if a reply carries no meta.pagination, since it
      could not tell where the last page is.

OUTPUT
  The members are at .data. meta names the tenant the call resolved to and
  where that came from (flag, env or config), followed by the API's own
  pagination object, exactly as the API sent it:

      {
        "data": [ <member>, ... ],
        "meta": {
          "tenant": "acme", "tenant_source": "flag",
          "pagination": { "page": 1, "pageSize": 20, "total": 42, "totalPages": 3 }
        }
      }

  With --all, meta.pagination reflects the last page fetched.

  If --all fails after at least one page was fetched, or stops at the page cap,
  stdout still holds one document: the rows fetched so far at .data, and an
  "error" key above them. The exit code is non-zero, so test for the error key
  before trusting the rows as complete.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// A zero or negative value is not "use the default": ListMembers omits it
		// from the request, so --page 0 would silently read page 1. Refuse it
		// while nothing has been sent and no credential is needed to be told.
		if membersListPage < 1 {
			failValidation("--page must be at least 1, got %d", membersListPage)
		}
		if membersListPageSize < 1 {
			failValidation("--page-size must be between 1 and %d, got %d", membersMaxPageSize, membersListPageSize)
		}

		client, cfg, err := buildClient()
		if err != nil {
			return handleErr(err)
		}

		t, source := resolveTenant(cfg)
		if api.MembersRequiresTenant {
			requireTenant(t)
		}

		pageSize := membersListPageSize
		if membersListAll && !cmd.Flags().Changed("page-size") {
			pageSize = membersAllPageSize
		}
		if pageSize > membersMaxPageSize {
			pageSize = membersMaxPageSize
		}
		params := api.ListMembersParams{Page: membersListPage, PageSize: pageSize, Status: membersListStatus}

		if !membersListAll {
			page, err := client.ListMembers(cmd.Context(), t, params)
			if err != nil {
				return handleErr(err)
			}
			return output.Write(os.Stdout, page.Members, listMeta(t, source, page.Meta))
		}

		page, err := client.ListAllMembers(cmd.Context(), t, params)
		if err != nil {
			if page != nil {
				return failWithData(err, page.Members, listMeta(t, source, page.Meta))
			}
			return handleErr(err)
		}
		return output.Write(os.Stdout, page.Members, listMeta(t, source, page.Meta))
	},
}

var membersGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a member by ID",
	Long: `Get a single member by UUID.

PURPOSE
  Call GET /api/members/{id}. Both "no such id" and "id in another tenant"
  return 404 — the API does not leak existence across tenants.

USAGE
  seli members get <id> [--tenant <code>]

FLAGS
  <id>
      Member UUID. Positional, required. Validated as a UUID before the
      request; a malformed value exits 5 without calling the server.

  --tenant <code>
      Tenant code to scope the request to. Required — same rule as
      members list. Exit 5 if none resolve.

OUTPUT
  { "data": <member>, "meta": { "tenant": "acme", "tenant_source": "flag" } }

  Exit 4 if the id doesn't exist or isn't in the resolved tenant.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if !uuidPattern.MatchString(id) {
			fail(api.ErrInvalidMemberID, "member id must be a UUID, got "+id, 400)
		}

		client, cfg, err := buildClient()
		if err != nil {
			return handleErr(err)
		}

		t, source := resolveTenant(cfg)
		if api.MembersRequiresTenant {
			requireTenant(t)
		}

		member, err := client.GetMember(cmd.Context(), t, id)
		if err != nil {
			return handleErr(err)
		}

		return output.Write(os.Stdout, member, itemMeta(t, source))
	},
}

func init() {
	membersListCmd.Flags().IntVar(&membersListPage, "page", 1, "page number (1-based)")
	membersListCmd.Flags().IntVar(&membersListPageSize, "page-size", 20, "items per page (1-100)")
	membersListCmd.Flags().StringVar(&membersListStatus, "status", "", "filter by membership status (active, inactive)")
	membersListCmd.Flags().BoolVar(&membersListAll, "all", false, "auto-paginate until the last page")

	membersCmd.AddCommand(membersListCmd)
	membersCmd.AddCommand(membersGetCmd)
	rootCmd.AddCommand(membersCmd)
}
