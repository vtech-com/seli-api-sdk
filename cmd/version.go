package cmd

import (
	"github.com/spf13/cobra"
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
  Version, commit and build date are at .data:

      {
        "data": { "version": "0.1.0", "commit": "4a1f9c2",
                  "built": "2026-06-18T10:03:00Z" },
        "meta": {}
      }`,
	RunE: func(cmd *cobra.Command, args []string) error {
		data := map[string]string{
			"version": version.Version,
			"commit":  version.Commit,
			"built":   version.Date,
		}
		return writeEnvelope(data)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
