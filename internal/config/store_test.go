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
				APIURL:        "https://api.example.test",
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

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// The temp file used to have a fixed name next to the config. Anyone able to
// plant a symlink there made Save write the config — API key included — through
// it, to a file of their choosing.
func TestSaveNeverWritesThroughAPlantedTempSymlink(t *testing.T) {
	path := withTempConfig(t)
	dir := filepath.Dir(path)

	victim := filepath.Join(t.TempDir(), "victim.txt")
	if err := os.WriteFile(victim, []byte("untouched"), 0o600); err != nil {
		t.Fatalf("write victim: %v", err)
	}
	for _, name := range []string{path + ".tmp", filepath.Join(dir, ".config.json.tmp"), filepath.Join(dir, "config.tmp")} {
		if err := os.Symlink(victim, name); err != nil {
			t.Skipf("cannot create symlinks here: %v", err)
		}
	}

	cfg := &Config{Version: schemaVersion, Profiles: map[string]Profile{"default": {APIKey: "ssk_secret_value"}}, ActiveProfile: "default"}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("read victim: %v", err)
	}
	if string(got) != "untouched" {
		t.Errorf("Save wrote through a symlink at a predictable temp name: victim now holds %q", got)
	}
	loaded, err := Load()
	if err != nil || loaded.Profiles["default"].APIKey != "ssk_secret_value" {
		t.Errorf("the config was not saved: %v / %+v", err, loaded)
	}
}

// A stale file at the old fixed temp name is none of Save's business.
func TestSaveLeavesAStaleLegacyTempFileAlone(t *testing.T) {
	path := withTempConfig(t)
	if err := os.WriteFile(path+".tmp", []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	cfg := &Config{Version: schemaVersion, Profiles: map[string]Profile{"default": {APIKey: "ssk_x"}}, ActiveProfile: "default"}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if b, err := os.ReadFile(path + ".tmp"); err != nil || string(b) != "stale" {
		t.Errorf("the stale file was disturbed: %q / %v", b, err)
	}
}

// Whether Save succeeds or fails, it leaves nothing but the config behind.
func TestSaveLeavesNoTempFileBehind(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		path := withTempConfig(t)
		cfg := &Config{Version: schemaVersion, Profiles: map[string]Profile{"default": {APIKey: "ssk_x"}}, ActiveProfile: "default"}
		for i := 0; i < 3; i++ {
			if err := Save(cfg); err != nil {
				t.Fatalf("Save: %v", err)
			}
		}
		if got := dirEntries(t, filepath.Dir(path)); len(got) != 1 || got[0] != "config.json" {
			t.Errorf("directory holds %v, want only config.json", got)
		}
	})

	t.Run("failure", func(t *testing.T) {
		path := withTempConfig(t)
		// A non-empty directory where the file belongs makes the rename fail.
		if err := os.MkdirAll(filepath.Join(path, "occupied"), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		cfg := &Config{Version: schemaVersion, Profiles: map[string]Profile{"default": {APIKey: "ssk_x"}}, ActiveProfile: "default"}

		if err := Save(cfg); err == nil {
			t.Fatal("Save succeeded over a directory")
		}
		if got := dirEntries(t, filepath.Dir(path)); len(got) != 1 || got[0] != "config.json" {
			t.Errorf("directory holds %v after a failed Save, want only the planted directory", got)
		}
	})
}

// MkdirAll leaves an existing directory as it found it, so a ~/.seli created
// earlier with 0755 stayed listable and, if group-writable, replaceable.
func TestSaveTightensAnExistingConfigDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	dir := filepath.Join(t.TempDir(), ".seli")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(dir, 0o755); err != nil { // beat the umask
		t.Fatalf("chmod: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	orig := ConfigPath
	ConfigPath = func() (string, error) { return path, nil }
	t.Cleanup(func() { ConfigPath = orig })

	cfg := &Config{Version: schemaVersion, Profiles: map[string]Profile{"default": {APIKey: "ssk_x"}}, ActiveProfile: "default"}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("config dir mode = %04o, want 0700", got)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("config file mode = %04o, want 0600", info.Mode().Perm())
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

func TestActiveProfileName(t *testing.T) {
	if got := ActiveProfileName(&Config{}); got != "default" {
		t.Errorf("unset active_profile: got %q, want default", got)
	}
	if got := ActiveProfileName(&Config{ActiveProfile: "staging"}); got != "staging" {
		t.Errorf("got %q, want staging", got)
	}
}

// The first login on a fresh machine has no config at all: it must create the
// profile, make it active, and stamp the schema version.
func TestSetAPIKeyCreatesTheProfile(t *testing.T) {
	cfg := &Config{}

	name := SetAPIKey(cfg, "ssk_abc")

	if name != "default" || cfg.ActiveProfile != "default" {
		t.Errorf("name = %q, active = %q, want default", name, cfg.ActiveProfile)
	}
	if cfg.Version != schemaVersion {
		t.Errorf("Version = %d, want %d", cfg.Version, schemaVersion)
	}
	if got := cfg.Profiles["default"].APIKey; got != "ssk_abc" {
		t.Errorf("APIKey = %q", got)
	}
}

// Logging in again, or into an existing profile, replaces only the key.
func TestSetAPIKeyKeepsTheRestOfTheProfile(t *testing.T) {
	cfg := &Config{
		Version:       schemaVersion,
		ActiveProfile: "staging",
		Profiles: map[string]Profile{
			"staging": {APIKey: "ssk_old", APIURL: "https://staging.example.test", DefaultTenant: "acme", KnownTenants: []string{"acme"}},
			"default": {APIKey: "ssk_other"},
		},
	}

	name := SetAPIKey(cfg, "ssk_new")

	if name != "staging" {
		t.Errorf("name = %q, want staging", name)
	}
	p := cfg.Profiles["staging"]
	if p.APIKey != "ssk_new" || p.APIURL != "https://staging.example.test" || p.DefaultTenant != "acme" || len(p.KnownTenants) != 1 {
		t.Errorf("profile = %+v: only the key should have changed", p)
	}
	if cfg.Profiles["default"].APIKey != "ssk_other" {
		t.Error("another profile's key was touched")
	}
}

func TestClearAPIKey(t *testing.T) {
	cfg := &Config{
		ActiveProfile: "default",
		Profiles:      map[string]Profile{"default": {APIKey: "ssk_abc", DefaultTenant: "acme"}},
	}

	name, had := ClearAPIKey(cfg)
	if name != "default" || !had {
		t.Fatalf("ClearAPIKey = (%q, %v), want (default, true)", name, had)
	}
	if p := cfg.Profiles["default"]; p.APIKey != "" || p.DefaultTenant != "acme" {
		t.Errorf("profile = %+v: only the key should have been cleared", p)
	}

	// Logging out twice is harmless.
	if _, had := ClearAPIKey(cfg); had {
		t.Error("second ClearAPIKey reports a key to remove")
	}
}

func TestClearAPIKeyWithNoProfile(t *testing.T) {
	cfg := &Config{}
	name, had := ClearAPIKey(cfg)
	if name != "default" || had {
		t.Errorf("ClearAPIKey = (%q, %v), want (default, false)", name, had)
	}
	if cfg.Profiles != nil {
		t.Error("ClearAPIKey must not create a profile")
	}
}
