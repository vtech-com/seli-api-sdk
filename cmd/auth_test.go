package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------------
// auth login / logout
// -----------------------------------------------------------------------------

func TestAuthLoginLogoutRoundTrip(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, healthOK)
	home := t.TempDir()
	cfgPath := filepath.Join(home, ".seli", "config.json")
	env := map[string]string{"SELI_API_URL": api.url()}

	// No key yet: the CLI says so locally.
	if res := cliRun(t, home, env, "health", "--tenant", "acme"); res.exit != 2 {
		t.Fatalf("before login: exit = %d, want 2", res.exit)
	}

	// Login on a machine with no config creates the profile.
	res := cliRun(t, home, env, "auth", "login", "--key", testAPIKey)
	if res.exit != 0 {
		t.Fatalf("login: exit = %d\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
	}
	if strings.Contains(res.stdout+res.stderr, testAPIKey) || strings.Contains(res.stdout+res.stderr, "SECRET") {
		t.Errorf("login echoed the key:\nstdout: %s\nstderr: %s", res.stdout, res.stderr)
	}
	data := obj(t, res.doc(t), "data")
	if data["profile"] != "default" || data["api_url"] != api.url() {
		t.Errorf("login data = %v", data)
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(cfgPath); info.Mode().Perm() != 0o600 {
			t.Errorf("config file mode = %04o, want 0600", info.Mode().Perm())
		}
	}

	// The stored key is what the next call sends — with no SELI_API_KEY in play.
	if res := cliRun(t, home, env, "health", "--tenant", "acme"); res.exit != 0 {
		t.Fatalf("after login: exit = %d\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
	}
	reqs := api.requests()
	if len(reqs) != 1 || reqs[0].header.Get("Authorization") != "Bearer "+testAPIKey {
		t.Fatalf("requests = %+v: the stored key was not sent", reqs)
	}

	// SELI_API_KEY overrides the stored key.
	withEnvKey := map[string]string{"SELI_API_URL": api.url(), "SELI_API_KEY": "ssk_from_env_key"}
	if res := cliRun(t, home, withEnvKey, "health", "--tenant", "acme"); res.exit != 0 {
		t.Fatalf("env key: exit = %d", res.exit)
	}
	if got := api.requests()[1].header.Get("Authorization"); got != "Bearer ssk_from_env_key" {
		t.Errorf("Authorization = %q, want the env key to win", got)
	}

	// Logout clears it; the profile and the rest of it stay.
	res = cliRun(t, home, env, "auth", "logout")
	if res.exit != 0 {
		t.Fatalf("logout: exit = %d: %s", res.exit, res.stderr)
	}
	if data := obj(t, res.doc(t), "data"); data["status"] != "logged_out" || data["profile"] != "default" {
		t.Errorf("logout data = %v", data)
	}
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(b), "SECRET") {
		t.Errorf("the key is still in the config file after logout:\n%s", b)
	}

	// Back to "no key", and no request is spent learning it.
	before := len(api.requests())
	if res := cliRun(t, home, env, "health", "--tenant", "acme"); res.exit != 2 {
		t.Errorf("after logout: exit = %d, want 2", res.exit)
	}
	if n := len(api.requests()); n != before {
		t.Errorf("a request was sent after logout (%d -> %d)", before, n)
	}

	// Logging out twice is harmless.
	res = cliRun(t, home, env, "auth", "logout")
	if res.exit != 0 {
		t.Fatalf("second logout: exit = %d", res.exit)
	}
	if data := obj(t, res.doc(t), "data"); data["status"] != "no_credentials" {
		t.Errorf("second logout data = %v", data)
	}
}

func TestAuthLoginKeepsTheRestOfTheProfile(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	cfgPath := writeConfigFile(t, home, `{"version":1,"active_profile":"default","profiles":{"default":{"api_key":"ssk_old","api_url":"https://kept.example.test","default_tenant":"acme"}}}`)

	res := cliRun(t, home, nil, "auth", "login", "--key", testAPIKey)

	if res.exit != 0 {
		t.Fatalf("exit = %d: %s", res.exit, res.stderr)
	}
	if data := obj(t, res.doc(t), "data"); data["api_url"] != "https://kept.example.test" {
		t.Errorf("login data = %v, want the profile's api_url", data)
	}
	b, _ := os.ReadFile(cfgPath)
	for _, want := range []string{testAPIKey, "https://kept.example.test", `"default_tenant": "acme"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("config lost %q:\n%s", want, b)
		}
	}
	if strings.Contains(string(b), "ssk_old") {
		t.Errorf("the old key is still stored:\n%s", b)
	}
}

func TestAuthLoginRejectsAMalformedKeyWithoutWritingAnything(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"csk_wrongprefix", "plainstring", "ssk_", "ssk_has space", ""} {
		t.Run(key, func(t *testing.T) {
			home := t.TempDir()

			res := cliRun(t, home, nil, "auth", "login", "--key", key)

			if res.exit != 5 {
				t.Errorf("exit = %d, want 5\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
			}
			if code := obj(t, res.doc(t), "error")["code"]; code != "INVALID_API_KEY" {
				t.Errorf("error.code = %v", code)
			}
			if len(key) > len("ssk_") && strings.Contains(res.stdout+res.stderr, key) {
				t.Errorf("the rejection repeated the key back:\n%s%s", res.stdout, res.stderr)
			}
			if _, err := os.Stat(filepath.Join(home, ".seli", "config.json")); err == nil {
				t.Error("a rejected login still wrote a config file")
			}
		})
	}
}

func TestAuthLoginRequiresKey(t *testing.T) {
	t.Parallel()
	res := cliRun(t, "", nil, "auth", "login")
	if res.exit != 5 {
		t.Errorf("exit = %d, want 5 for a missing required flag", res.exit)
	}
}

func TestAuthHasNoWhoami(t *testing.T) {
	t.Parallel()
	res := cliRun(t, "", nil, "auth", "whoami")
	if res.exit == 0 {
		t.Error("`auth whoami` exited 0, but the API has no /me endpoint to back it")
	}
}
