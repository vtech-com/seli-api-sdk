package api

import "testing"

func TestResolveBaseURL_Precedence(t *testing.T) {
	tests := []struct {
		name                             string
		flag, env, profile, buildDefault string
		want                             string
	}{
		{"flag beats everything", "https://flag.test", "https://env.test", "https://profile.test", "https://build.test", "https://flag.test"},
		{"env beats profile and build", "", "https://env.test", "https://profile.test", "https://build.test", "https://env.test"},
		{"profile beats build", "", "", "https://profile.test", "https://build.test", "https://profile.test"},
		{"build default is the last resort", "", "", "", "https://build.test", "https://build.test"},
		{"every layer empty", "", "", "", "", ""},
		{"whitespace is empty", "  ", "\t", " \n", "https://build.test", "https://build.test"},
		{"value is trimmed", " https://flag.test \n", "", "", "", "https://flag.test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveBaseURL(tt.flag, tt.env, tt.profile, tt.buildDefault); got != tt.want {
				t.Errorf("ResolveBaseURL = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveTenant_PrecedenceAndSource(t *testing.T) {
	tests := []struct {
		name                 string
		flag, env, def       string
		wantCode, wantSource string
	}{
		{"flag beats env and config", "from-flag", "from-env", "from-config", "from-flag", TenantFromFlag},
		{"env beats config", "", "from-env", "from-config", "from-env", TenantFromEnv},
		{"config is the last resort", "", "", "from-config", "from-config", TenantFromConfig},
		{"nothing resolves", "", "", "", "", ""},
		{"whitespace is empty", " ", "  ", "\t", "", ""},
		{"value is trimmed", " acme ", "", "", "acme", TenantFromFlag},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, source := ResolveTenant(tt.flag, tt.env, tt.def)
			if code != tt.wantCode || source != tt.wantSource {
				t.Errorf("ResolveTenant = (%q, %q), want (%q, %q)", code, source, tt.wantCode, tt.wantSource)
			}
		})
	}
}

func TestTenantSourceNamesAreTheDocumentedOnes(t *testing.T) {
	if TenantFromFlag != "flag" || TenantFromEnv != "env" || TenantFromConfig != "config" {
		t.Errorf("tenant_source values changed: %q %q %q", TenantFromFlag, TenantFromEnv, TenantFromConfig)
	}
}
