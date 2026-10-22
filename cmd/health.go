package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/vtech-com/seli-api-sdk/internal/api"
	"github.com/vtech-com/seli-api-sdk/internal/output"
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check API reachability and auth",
	Long: `Auth-path canary — proves the API key and tenant resolve end-to-end.

PURPOSE
  Call GET /api/health. Not a general uptime check: it only confirms this
  CLI's credentials and tenant are accepted by the server.

USAGE
  seli health [--tenant <code>]

FLAGS
  --tenant <code>
      Tenant code to scope the request to. Required — api/openapi.json
      declares X-Tenant-Code as a required header on this endpoint, despite
      "health check" sounding tenant-agnostic. Resolution order: --tenant,
      then SELI_TENANT, then config default_tenant. Exit 5 if none resolve.

OUTPUT
  The server's reply is at .data; meta names the tenant the call resolved to
  and where that came from (flag, env or config):

      {
        "data": { "ok": true, "timestamp": "2026-07-31T09:30:00.000Z" },
        "meta": { "tenant": "acme", "tenant_source": "flag" }
      }

  Exit 2 on auth failure — including no API key configured, which is
  diagnosed locally without contacting the server. Exit 3 if the key cannot
  act for the tenant, 5 if no tenant or no API base URL resolves, 7 on rate
  limit, 6 on network failure.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, cfg, err := buildClient()
		if err != nil {
			return handleErr(err)
		}

		t, source := resolveTenant(cfg)
		if api.HealthRequiresTenant {
			requireTenant(t)
		}

		health, err := client.GetHealth(cmd.Context(), t)
		if err != nil {
			return handleErr(err)
		}

		return output.Write(os.Stdout, health, itemMeta(t, source))
	},
}

func init() {
	rootCmd.AddCommand(healthCmd)
}
