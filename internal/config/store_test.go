package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// withTempConfig overrides ConfigPath to use a temp file for the duration of the test.
func withTempConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	orig := ConfigPath
	ConfigPath = func() (string, error) { return path, nil }
	t.Cleanup(func() { ConfigPath = orig })
	return path
}

func TestLoadMissingFile(t *testing.T) {
	withTempConfig(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load on missing file returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load returned nil Config")
		return
	}
	if cfg.Version != 0 {
		t.Errorf("expected version 0 for empty config, got %d", cfg.Version)
	}
	if cfg.Profiles != nil {
		t.Errorf("expected nil Profiles for empty config, got %v", cfg.Profiles)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := withTempConfig(t)

	cfg := &Config{
		Version: schemaVersion,
		Profiles: map[string]Profile{
			"default": {
				APIKey:        "sk_test123",
				APIURL:        "https://api.seli.app",
				DefaultTenant: "acme",
				KnownTenants:  []string{"acme", "globex"},
			},
		},
		ActiveProfile: "default",
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file %s still exists after Save", path+".tmp")
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Version != cfg.Version {
		t.Errorf("Version: got %d, want %d", loaded.Version, cfg.Version)
	}
	if loaded.ActiveProfile != cfg.ActiveProfile {
		t.Errorf("ActiveProfile: got %q, want %q", loaded.ActiveProfile, cfg.ActiveProfile)
	}
	p := loaded.Profiles["default"]
	if p.APIKey != "sk_test123" {
		t.Errorf("APIKey: got %q, want %q", p.APIKey, "sk_test123")
	}
	if p.DefaultTenant != "acme" {
		t.Errorf("DefaultTenant: got %q, want %q", p.DefaultTenant, "acme")
	}
	if len(p.KnownTenants) != 2 {
		t.Errorf("KnownTenants length: got %d, want 2", len(p.KnownTenants))
	}
}

func TestSaveChmodVerification(t *testing.T) {
	path := withTempConfig(t)

	cfg := &Config{
		Version:       schemaVersion,
		Profiles:      map[string]Profile{"default": {APIKey: "sk_x"}},
		ActiveProfile: "default",
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat after Save: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 && runtime.GOOS != "windows" {
		t.Errorf("file permissions: got %04o, want 0600", got)
	}
}

func TestAtomicWriteTmpAbsent(t *testing.T) {
	path := withTempConfig(t)

	cfg := &Config{
		Version:       schemaVersion,
		Profiles:      map[string]Profile{"default": {APIKey: "sk_y"}},
		ActiveProfile: "default",
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("temp file %s should not exist after successful Save", tmpPath)
	}
}

func TestSetProfileSuccess(t *testing.T) {
	withTempConfig(t)

	cfg := &Config{
		Version: schemaVersion,
		Profiles: map[string]Profile{
			"default": {APIKey: "sk_a"},
			"staging": {APIKey: "sk_b"},
		},
		ActiveProfile: "default",
	}

	if err := SetProfile(cfg, "staging"); err != nil {
		t.Fatalf("SetProfile: %v", err)
	}
	if cfg.ActiveProfile != "staging" {
		t.Errorf("ActiveProfile: got %q, want %q", cfg.ActiveProfile, "staging")
	}
}

func TestSetProfileNotFound(t *testing.T) {
	withTempConfig(t)

	cfg := &Config{
		Version:       schemaVersion,
		Profiles:      map[string]Profile{"default": {APIKey: "sk_a"}},
		ActiveProfile: "default",
	}

	if err := SetProfile(cfg, "nonexistent"); err == nil {
		t.Error("expected error for nonexistent profile, got nil")
	}
}

func TestActiveProfileDefault(t *testing.T) {
	cfg := &Config{
		Version: schemaVersion,
		Profiles: map[string]Profile{
			"default": {APIKey: "sk_default"},
		},
		ActiveProfile: "",
	}

	p, err := ActiveProfile(cfg)
	if err != nil {
		t.Fatalf("ActiveProfile: %v", err)
	}
	if p.APIKey != "sk_default" {
		t.Errorf("APIKey: got %q, want %q", p.APIKey, "sk_default")
	}
}

func TestActiveProfileNotFound(t *testing.T) {
	cfg := &Config{
		Version:       schemaVersion,
		Profiles:      map[string]Profile{"default": {APIKey: "sk_a"}},
		ActiveProfile: "missing",
	}

	if _, err := ActiveProfile(cfg); err == nil {
		t.Error("expected error for missing active profile, got nil")
	}
}

func TestMergeKnownTenantsDedupeSort(t *testing.T) {
	cfg := &Config{
		Version: schemaVersion,
		Profiles: map[string]Profile{
			"default": {
				APIKey:       "sk_a",
				KnownTenants: []string{"globex", "acme"},
			},
		},
		ActiveProfile: "default",
	}

	if err := MergeKnownTenants(cfg, []string{"initech", "acme", "zebra"}); err != nil {
		t.Fatalf("MergeKnownTenants: %v", err)
	}

	p := cfg.Profiles["default"]
	want := []string{"acme", "globex", "initech", "zebra"}
	if len(p.KnownTenants) != len(want) {
		t.Fatalf("KnownTenants length: got %d, want %d: %v", len(p.KnownTenants), len(want), p.KnownTenants)
	}
	for i, v := range want {
		if p.KnownTenants[i] != v {
			t.Errorf("KnownTenants[%d]: got %q, want %q", i, p.KnownTenants[i], v)
		}
	}
}

func TestMergeKnownTenantsEmpty(t *testing.T) {
	cfg := &Config{
		Version: schemaVersion,
		Profiles: map[string]Profile{
			"default": {APIKey: "sk_a", KnownTenants: []string{"acme"}},
		},
		ActiveProfile: "default",
	}

	if err := MergeKnownTenants(cfg, []string{}); err != nil {
		t.Fatalf("MergeKnownTenants: %v", err)
	}

	p := cfg.Profiles["default"]
	if len(p.KnownTenants) != 1 || p.KnownTenants[0] != "acme" {
		t.Errorf("unexpected KnownTenants: %v", p.KnownTenants)
	}
}

func TestMergeKnownTenantsProfileNotFound(t *testing.T) {
	cfg := &Config{
		Version:       schemaVersion,
		Profiles:      map[string]Profile{"default": {APIKey: "sk_a"}},
		ActiveProfile: "ghost",
	}

	if err := MergeKnownTenants(cfg, []string{"acme"}); err == nil {
		t.Error("expected error for missing profile, got nil")
	}
}

func TestSaveCreatesDirIfMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "config.json")
	orig := ConfigPath
	ConfigPath = func() (string, error) { return path, nil }
	defer func() { ConfigPath = orig }()

	cfg := &Config{
		Version:       schemaVersion,
		Profiles:      map[string]Profile{"default": {APIKey: "sk_a"}},
		ActiveProfile: "default",
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save with missing dir: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}
