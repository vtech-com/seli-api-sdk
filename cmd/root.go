package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	apiURL  string
	verbose bool
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
  seli [--api-url <url>] [-v] <group> <command> [<args>]

  seli <group> --help          the commands in that group
  seli <group> <cmd> --help    purpose, usage, flags, output`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() {
	// Runs once, after every command file's init() has registered its
	// subcommands (main.go calls Execute() only after package init completes).
	enableUnknownSubcommandErrors(rootCmd)
	if err := rootCmd.Execute(); err != nil {
		wrapped := &CLIError{Code: "VALIDATION_ERROR", Message: err.Error(), HTTPStatus: 400}
		renderCLIError(wrapped)
		os.Exit(ExitCodeFor(wrapped))
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", "", "override the API base URL")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "print the HTTP request and response (key redacted)")

	_ = viper.BindEnv("api_key", "SELI_API_KEY")
	_ = viper.BindEnv("tenant", "SELI_TENANT")
	_ = viper.BindEnv("api_url", "SELI_API_URL")

	_ = viper.BindPFlag("api_url", rootCmd.PersistentFlags().Lookup("api-url"))
}

// initConfig is the cobra.OnInitialize hook. It merges env-var overrides via
// viper; internal/config handles the on-disk profile store separately.
func initConfig() {
	viper.SetEnvPrefix("")
	viper.AutomaticEnv()
}
