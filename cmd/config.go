package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vtech-com/seli-api-sdk/internal/config"
)

// configNotFoundErr exits with code 4 (not found) via the standard error path.
func configNotFoundErr(msg string) {
	fail("NOT_FOUND", msg, 404)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage seli CLI configuration",
	Long: `Local settings for this CLI.

Every command here reads and writes ~/.seli/config.json (file mode 600)
directly; none of them calls the API. There is exactly one active profile;
this CLI takes no --profile flag. Settings apply to that active profile.

USAGE
  seli config <command> [<args>]`,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration key",
	Long: `Set a configuration value.

PURPOSE
  Write one setting into ~/.seli/config.json. default_tenant is not
  settable here — use config set-default-tenant instead.

USAGE
  seli config set <key> <value>

FLAGS
  <key>
      One of api_url, default_profile. Positional, required. An unrecognised
      key — including default_tenant — exits 5 and names the keys that are
      recognised.

  <value>
      The value to store. Positional, required.

        seli config set api_url https://api.seli.app/v1
        seli config set default_profile staging

OUTPUT
  Nothing on success: exit 0 and silence. Confirm with config get <key>.

  Exit 5 for an unrecognised key, or when default_profile names a profile
  that does not exist. Exit 4 when api_url is set before any profile exists —
  run auth login first.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]

		cfg, err := config.Load()
		if err != nil {
			fail("CONFIG_LOAD_ERROR", err.Error(), 0)
		}

		switch key {
		case "api_url":
			profileName := cfg.ActiveProfile
			if profileName == "" {
				profileName = "default"
			}
			p, ok := cfg.Profiles[profileName]
			if !ok {
				configNotFoundErr(fmt.Sprintf("profile %q not found", profileName))
			}
			p.APIURL = value
			cfg.Profiles[profileName] = p

		case "default_profile":
			if err := config.SetProfile(cfg, value); err != nil {
				failValidation("%s", err.Error())
			}

		default:
			failValidation("unknown key %q; supported: api_url, default_profile", key)
		}

		if err := config.Save(cfg); err != nil {
			fail("CONFIG_SAVE_ERROR", err.Error(), 0)
		}
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Long: `Read a configuration value.

PURPOSE
  Print one setting from ~/.seli/config.json. To change it, use config set
  or config set-default-tenant.

USAGE
  seli config get <key>

FLAGS
  <key>
      One of api_url, default_profile, default_tenant. Positional, required.
      An unrecognised key exits 5 and names the keys that are recognised.

        seli config get default_tenant
        seli config get api_url

OUTPUT
  The key and its value are at .data:

      { "data": { "key": "default_tenant", "value": "acme" }, "meta": {} }

  value is "" when the key exists but is unset (e.g. no default_tenant has
  been stored yet) — never null.

  Exit 4 if the active profile itself is not in the config file yet.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		cfg, err := config.Load()
		if err != nil {
			fail("CONFIG_LOAD_ERROR", err.Error(), 0)
		}

		var value string
		switch key {
		case "default_profile":
			value = cfg.ActiveProfile
			if value == "" {
				value = "default"
			}

		case "api_url":
			p, err := config.ActiveProfile(cfg)
			if err != nil {
				configNotFoundErr(err.Error())
			}
			value = p.APIURL

		case "default_tenant":
			p, err := config.ActiveProfile(cfg)
			if err != nil {
				configNotFoundErr(err.Error())
			}
			value = p.DefaultTenant

		default:
			failValidation("unknown key %q; supported: api_url, default_profile, default_tenant", key)
		}

		return writeEnvelope(map[string]string{"key": key, "value": value})
	},
}

var configSetDefaultTenantCmd = &cobra.Command{
	Use:   "set-default-tenant <code>",
	Short: "Set the default tenant for the active profile",
	Long: `Set the tenant used when --tenant is omitted.

PURPOSE
  Store default_tenant as the last fallback in tenant resolution: --tenant,
  then SELI_TENANT, then this value.

USAGE
  seli config set-default-tenant <code>

FLAGS
  <code>
      A tenant code. Positional, required. Stored as given: this command does
      not call the API, so it does not check that the tenant exists.

        seli config set-default-tenant acme

OUTPUT
  Nothing on success: exit 0 and silence. Confirm with config get
  default_tenant.

  Exit 4 if the active profile itself is not in the config file yet.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		code := args[0]

		cfg, err := config.Load()
		if err != nil {
			fail("CONFIG_LOAD_ERROR", err.Error(), 0)
		}

		profileName := cfg.ActiveProfile
		if profileName == "" {
			profileName = "default"
		}
		p, ok := cfg.Profiles[profileName]
		if !ok {
			configNotFoundErr(fmt.Sprintf("profile %q not found", profileName))
		}
		p.DefaultTenant = code
		cfg.Profiles[profileName] = p

		if err := config.Save(cfg); err != nil {
			fail("CONFIG_SAVE_ERROR", err.Error(), 0)
		}
		return nil
	},
}

var configUnsetDefaultTenantCmd = &cobra.Command{
	Use:   "unset-default-tenant",
	Short: "Clear the default tenant for the active profile",
	Long: `Clear the default tenant.

PURPOSE
  Remove default_tenant, so that every command which needs a tenant must be
  given one explicitly via --tenant or SELI_TENANT.

USAGE
  seli config unset-default-tenant

FLAGS
  (none)

OUTPUT
  Nothing on success: exit 0 and silence. Confirm with config get
  default_tenant, which then reports value: "".

  Exit 4 if the active profile itself is not in the config file yet.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			fail("CONFIG_LOAD_ERROR", err.Error(), 0)
		}

		profileName := cfg.ActiveProfile
		if profileName == "" {
			profileName = "default"
		}
		p, ok := cfg.Profiles[profileName]
		if !ok {
			configNotFoundErr(fmt.Sprintf("profile %q not found", profileName))
		}
		p.DefaultTenant = ""
		cfg.Profiles[profileName] = p

		if err := config.Save(cfg); err != nil {
			fail("CONFIG_SAVE_ERROR", err.Error(), 0)
		}
		return nil
	},
}

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configSetDefaultTenantCmd)
	configCmd.AddCommand(configUnsetDefaultTenantCmd)
	rootCmd.AddCommand(configCmd)
}
