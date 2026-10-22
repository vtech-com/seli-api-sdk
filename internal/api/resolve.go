package api

import "strings"

// Where a resolved tenant came from. They are what `meta.tenant_source` prints.
const (
	TenantFromFlag   = "flag"   // --tenant
	TenantFromEnv    = "env"    // SELI_TENANT
	TenantFromConfig = "config" // the active profile's default_tenant
)

// ResolveBaseURL implements the base URL precedence rule:
//
//	--api-url  >  $SELI_API_URL  >  profile api_url  >  build-time default
//
// The build-time default (internal/version.DefaultAPIURL) is a last resort and
// is empty unless the binary was built with SELI_DEFAULT_API_URL. Returns ""
// when every layer is empty; NewClient turns that into API_URL_MISSING.
func ResolveBaseURL(flag, env, profile, buildDefault string) string {
	for _, v := range []string{flag, env, profile, buildDefault} {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// ResolveTenant implements the tenant precedence rule:
//
//	--tenant  >  $SELI_TENANT  >  config default_tenant
//
// It returns the tenant code and the layer it came from, so a silently
// defaulted tenant is visible rather than assumed. Both are "" when no layer
// names one. env may already include the flag (viper merges them): the flag is
// checked first either way.
func ResolveTenant(flag, env, defaultTenant string) (code, source string) {
	if v := strings.TrimSpace(flag); v != "" {
		return v, TenantFromFlag
	}
	if v := strings.TrimSpace(env); v != "" {
		return v, TenantFromEnv
	}
	if v := strings.TrimSpace(defaultTenant); v != "" {
		return v, TenantFromConfig
	}
	return "", ""
}
