package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/vtech-com/seli-api-sdk/internal/api"
)

var (
	apiURL  string
	verbose bool
	tenant  string
)

var rootCmd = &cobra.Command{
	Use:   "seli",
	Short: "Seli CLI — interact with the Seli Public API",
	Long: `seli is the command-line interface to the Seli Public API.

It stores your API key, attaches the tenant header, maps API errors to stable
exit codes, and prints JSON. Configuration lives in ~/.seli/config.json.

Every command emits the same envelope on stdout — { "data": …, "meta": … } —
whether it read one record, read a page of them, or wrote one. There is no
output flag, and no second shape to branch on.

USAGE
  seli [--api-url <url>] [--tenant <code>] [-v] <group> <command> [<args>]

  seli <group> --help          the commands in that group
  seli <group> <cmd> --help    purpose, usage, flags, output

GETTING STARTED
  Every command that reaches the API needs a key — which is all of them but
  version, config, and auth login/logout. Store one once, name a default
  tenant, then confirm the API accepts both:

      $ seli auth login --key ssk_...
      $ seli config set-default-tenant <code>
      $ seli health

  Until a key is configured the CLI does not contact the server at all: it
  fails locally with exit 2 and AUTH_MISSING_HEADER, so nothing you read here
  is an answer from the API. SELI_API_KEY, if set, overrides the stored key
  and needs no login; SELI_TENANT does the same for the tenant.

  The API host is configuration, not part of the CLI. It is the first of
  --api-url, SELI_API_URL, the profile's api_url, and the default baked into
  your build of the binary (seli version prints it). If none is set the CLI
  fails locally with exit 5 and API_URL_MISSING.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() {
	// Runs once, after every command file's init() has registered its
	// subcommands (main.go calls Execute() only after package init completes).
	enableUnknownSubcommandErrors(rootCmd)
	if err := rootCmd.Execute(); err != nil {
		// Wrap cobra arg-validation errors so they render through the output
		// formatter and map to exit code 5.
		wrapped := &api.APIError{Code: api.ErrValidation, Message: err.Error(), HTTPStatus: 400}
		renderCLIError(wrapped)
		os.Exit(api.ExitCodeFor(wrapped))
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", "", "override the API base URL (host only, https)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "trace the HTTP request and response on stderr (key redacted)")
	rootCmd.PersistentFlags().StringVar(&tenant, "tenant", "", "tenant code to scope the request to")

	_ = viper.BindEnv("api_key", "SELI_API_KEY")
	_ = viper.BindEnv("tenant", "SELI_TENANT")
	_ = viper.BindEnv("api_url", "SELI_API_URL")

	_ = viper.BindPFlag("api_url", rootCmd.PersistentFlags().Lookup("api-url"))
	_ = viper.BindPFlag("tenant", rootCmd.PersistentFlags().Lookup("tenant"))
}

// initConfig is the cobra.OnInitialize hook. It merges env-var overrides via
// viper; internal/config handles the on-disk profile store separately.
func initConfig() {
	viper.SetEnvPrefix("")
	viper.AutomaticEnv()
}
