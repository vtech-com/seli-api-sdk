package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/vtech-com/seli-api-sdk/internal/output"
	"github.com/vtech-com/seli-api-sdk/internal/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the seli version",
	Long: `Print the build of this CLI.

PURPOSE
  Report which binary is running. Help text ships inside the binary, so the
  page you are reading describes this build and no other.

USAGE
  seli version

FLAGS
  (none)

OUTPUT
  Version, commit, build date and the default API URL are at .data:

      {
        "data": { "version": "0.1.0", "commit": "4a1f9c2",
                  "built": "2026-06-18T10:03:00Z",
                  "default_api_url": "https://<seli-platform-host>" },
        "meta": {}
      }

  default_api_url is the host this binary was built to fall back to when
  --api-url, SELI_API_URL and the profile's api_url are all unset. It is ""
  for a binary built without one, which then needs the host given at runtime.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		data := map[string]string{
			"version":         version.Version,
			"commit":          version.Commit,
			"built":           version.Date,
			"default_api_url": version.DefaultAPIURL,
		}
		return output.Write(os.Stdout, data, output.Meta{})
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
