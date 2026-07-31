package cmd

import (
	"github.com/spf13/cobra"
	"github.com/vtech-com/seli-api-sdk/internal/api"
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check API reachability and auth",
	Long: `Auth-path canary — proves the API key and tenant resolve end-to-end.

PURPOSE
  Call GET /health. Not a general uptime check: it only confirms this CLI's
  credentials and tenant are accepted by the server.

USAGE
  seli health [--tenant <code>]

FLAGS
  --tenant <code>
      Tenant code to scope the request to. Required — api/openapi.json
      declares X-Tenant-Code as a required header on this endpoint, despite
      "health check" sounding tenant-agnostic. Resolution order: --tenant,
      then SELI_TENANT, then config default_tenant. Exit 5 if none resolve.

OUTPUT
  { "data": { "ok": true, "timestamp": "2026-07-31T09:30:00.000Z" }, "meta": {} }

  Exit 2 on auth failure, 7 on rate limit, 6 on network failure.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		t := resolveTenant()
		if api.HealthRequiresTenant && t == "" {
			fail(api.ErrTenantRequired, "no tenant resolved: pass --tenant, set SELI_TENANT, or run `seli config set-default-tenant`", 400)
		}

		client := api.NewClient()
		health, err := client.GetHealth(cmd.Context(), t)
		if err != nil {
			failAPIError(err)
		}

		return writeEnvelope(health)
	},
}

func init() {
	rootCmd.AddCommand(healthCmd)
}
