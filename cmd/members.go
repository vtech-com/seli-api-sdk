package cmd

import (
	"regexp"

	"github.com/spf13/cobra"
	"github.com/vtech-com/seli-api-sdk/internal/api"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

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
  Call GET /members. Page-based pagination; optional status filter.

USAGE
  seli members list [--tenant <code>] [--page N] [--page-size N] [--status active|inactive] [--all]

FLAGS
  --tenant <code>
      Tenant code to scope the request to. Required — api/openapi.json
      declares X-Tenant-Code as a required header on this endpoint. Exit 5
      if none resolve (--tenant, SELI_TENANT, config default_tenant).

  --page N
      Page number, 1-based. Default 1.

  --page-size N
      Items per page, 1-100. Default 20; values above 100 are capped before
      the request is sent.

  --status active|inactive
      Filter by membership status. Omitted by default (all statuses).

  --all
      Auto-paginate: repeat the request until page == meta.pagination.totalPages,
      concatenating results. --page is ignored when --all is set.

OUTPUT
  { "data": [ <member>, ... ], "meta": { "pagination": { "page", "pageSize", "total", "totalPages" } } }

  With --all, meta reflects the last page fetched.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		t := resolveTenant()
		if api.MembersRequiresTenant && t == "" {
			fail(api.ErrTenantRequired, "no tenant resolved: pass --tenant, set SELI_TENANT, or run `seli config set-default-tenant`", 400)
		}

		pageSize := membersListPageSize
		if pageSize > 100 {
			pageSize = 100
		}

		client := api.NewClient()

		if !membersListAll {
			members, meta, err := client.ListMembers(cmd.Context(), t, api.ListMembersParams{
				Page:     membersListPage,
				PageSize: pageSize,
				Status:   membersListStatus,
			})
			if err != nil {
				failAPIError(err)
			}
			return writeEnvelopeWithMeta(members, meta)
		}

		var all []api.Member
		var lastMeta *api.MembersMeta
		page := 1
		for {
			members, meta, err := client.ListMembers(cmd.Context(), t, api.ListMembersParams{
				Page:     page,
				PageSize: pageSize,
				Status:   membersListStatus,
			})
			if err != nil {
				failAPIError(err)
			}
			all = append(all, members...)
			lastMeta = meta
			if page >= meta.Pagination.TotalPages {
				break
			}
			page++
		}
		return writeEnvelopeWithMeta(all, lastMeta)
	},
}

var membersGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a member by ID",
	Long: `Get a single member by UUID.

PURPOSE
  Call GET /members/{id}. Both "no such id" and "id in another tenant"
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
  { "data": <member>, "meta": {} }

  Exit 4 if the id doesn't exist or isn't in the resolved tenant.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if !uuidPattern.MatchString(id) {
			fail(api.ErrInvalidMemberID, "member id must be a UUID, got "+id, 400)
		}

		t := resolveTenant()
		if api.MembersRequiresTenant && t == "" {
			fail(api.ErrTenantRequired, "no tenant resolved: pass --tenant, set SELI_TENANT, or run `seli config set-default-tenant`", 400)
		}

		client := api.NewClient()
		member, err := client.GetMember(cmd.Context(), t, id)
		if err != nil {
			failAPIError(err)
		}

		return writeEnvelope(member)
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
