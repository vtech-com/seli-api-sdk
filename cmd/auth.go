package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/vtech-com/seli-api-sdk/internal/api"
	"github.com/vtech-com/seli-api-sdk/internal/config"
	"github.com/vtech-com/seli-api-sdk/internal/output"
	"github.com/vtech-com/seli-api-sdk/internal/version"
)

// apiKeyPrefix is what every Seli API key starts with.
const apiKeyPrefix = "ssk_"

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication credentials",
	Long: `The API key this CLI sends with every request.

SELI_API_KEY, if set, overrides the key stored here on every call. Otherwise
the key comes from the active profile in ~/.seli/config.json (file mode 600).
None of these commands take --tenant, and none of them calls the API.

USAGE
  seli auth <command> [<args>]`,
}

var loginKey string

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Save an API key to the active profile",
	Long: `Store an API key in the active profile.

PURPOSE
  Save the key this CLI sends on every request. This only writes to
  ~/.seli/config.json (file mode 600), creating the profile if the file has
  none yet; it makes no API call, so it cannot itself confirm the key is
  accepted. Run health next to confirm that.

USAGE
  seli auth login --key <ssk_...>

FLAGS
  --key <ssk_...>
      API key to store. Required. Must start with ssk_ and contain no
      whitespace; if it does not, login exits 5 without writing anything.
      A key passed as an argument stays in your shell history and is visible
      in the process list while the command runs. On a shared machine or in
      CI, skip login and set SELI_API_KEY instead.

        seli auth login --key ssk_9f2a1c...

OUTPUT
  The key itself is never echoed back, in any form. api_url is the base URL
  the next call would use ("" if none is configured yet):

      {
        "data": { "profile": "default", "api_url": "https://<seli-platform-host>" },
        "meta": {}
      }

  Exit 5 if --key is malformed. Exit 1 if ~/.seli/config.json cannot be read
  or written.`,
	Args: cobra.NoArgs,
	RunE: runLogin,
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove the API key from the active profile",
	Long: `Clear the stored API key from the active profile.

PURPOSE
  Remove the key from ~/.seli/config.json. A profile with no key is not an
  error; logging out twice is harmless. SELI_API_KEY, if set in your
  environment, is not touched and keeps working.

USAGE
  seli auth logout

FLAGS
  This command takes no flags.

OUTPUT
  The profile and its resulting state:

      { "data": { "profile": "default", "status": "logged_out" }, "meta": {} }

  status is "no_credentials" instead, when the profile had no stored key to
  begin with.

  Exit 1 if ~/.seli/config.json cannot be read or written.`,
	Args: cobra.NoArgs,
	RunE: runLogout,
}

func init() {
	loginCmd.Flags().StringVar(&loginKey, "key", "", "API key (must start with ssk_)")
	_ = loginCmd.MarkFlagRequired("key")

	authCmd.AddCommand(loginCmd, logoutCmd)
	rootCmd.AddCommand(authCmd)
}

func runLogin(_ *cobra.Command, _ []string) error {
	key := strings.TrimSpace(loginKey)
	if !strings.HasPrefix(key, apiKeyPrefix) || len(key) == len(apiKeyPrefix) || !api.ValidKeyChars(key) {
		// The message never repeats the value: a mistyped key is still a key.
		fail("INVALID_API_KEY", fmt.Sprintf("invalid API key: it must start with %s, be followed by the secret, and contain no whitespace", apiKeyPrefix), 400)
	}

	cfg, err := config.Load()
	if err != nil {
		return handleErr(&api.APIError{Code: "CONFIG_LOAD_ERROR", Message: err.Error()})
	}

	profileName := config.SetAPIKey(cfg, key)

	if err := config.Save(cfg); err != nil {
		return handleErr(&api.APIError{Code: "CONFIG_SAVE_ERROR", Message: err.Error()})
	}

	// Same precedence as buildClient, without requiring a key to already work.
	baseURL := api.ResolveBaseURL(apiURL, viper.GetString("api_url"), cfg.Profiles[profileName].APIURL, version.DefaultAPIURL)

	return output.Write(os.Stdout, map[string]string{
		"profile": profileName,
		"api_url": baseURL,
	}, output.Meta{})
}

func runLogout(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return handleErr(&api.APIError{Code: "CONFIG_LOAD_ERROR", Message: err.Error()})
	}

	profileName, had := config.ClearAPIKey(cfg)
	if !had {
		return output.Write(os.Stdout, map[string]string{
			"profile": profileName,
			"status":  "no_credentials",
		}, output.Meta{})
	}

	if err := config.Save(cfg); err != nil {
		return handleErr(&api.APIError{Code: "CONFIG_SAVE_ERROR", Message: err.Error()})
	}

	return output.Write(os.Stdout, map[string]string{
		"profile": profileName,
		"status":  "logged_out",
	}, output.Meta{})
}
