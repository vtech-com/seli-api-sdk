package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

const (
	schemaVersion = 1
	configDir     = ".seli"
	configFile    = "config.json"
	configPerm    = 0o600
)

// Profile holds credentials and preferences for one named profile.
type Profile struct {
	APIKey        string   `json:"api_key"`
	APIURL        string   `json:"api_url"`
	DefaultTenant string   `json:"default_tenant,omitempty"`
	KnownTenants  []string `json:"known_tenants,omitempty"`
}

// Config is the top-level structure stored in ~/.seli/config.json.
type Config struct {
	Version       int                `json:"version"`
	Profiles      map[string]Profile `json:"profiles"`
	ActiveProfile string             `json:"active_profile"`
}

// pathProvider allows tests to override the config path without global state.
type pathProvider func() (string, error)

var defaultPathProvider pathProvider = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, configDir, configFile), nil
}

// ConfigPath returns the path to the config file. Exposed for tests.
var ConfigPath = defaultPathProvider

// Load reads the config file. If the file does not exist, Load returns a
// zero Config (version=0, nil Profiles) and no error.
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// Save writes cfg to the config file atomically and enforces 0600 permissions.
// It creates the parent directory if needed.
func Save(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	// MkdirAll leaves a directory that already exists as it found it, and the
	// mode it applies to a new one is subject to the umask. A ~/.seli made earlier
	// with 0755 would stay listable, and if group-writable its config.json
	// replaceable by someone else: tighten it every time.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // G302 expects 0600, but a directory needs its execute bit to be entered; 0700 is owner-only

			return fmt.Errorf("secure config dir: %w", err)
		}
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// A temp file in the same directory, so the rename below is atomic, under a
	// name nobody can predict: a fixed name next to the config can be pre-planted
	// as a symlink, and writing the config — API key included — through it would
	// hand the secret to whatever the link points at. CreateTemp opens it
	// O_EXCL, so it also never reuses or follows an existing file, and creates it
	// 0600.
	f, err := os.CreateTemp(dir, configFile+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("sync temp config: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close temp config: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename config: %w", err)
	}

	if err := os.Chmod(path, configPerm); err != nil {
		return fmt.Errorf("chmod config: %w", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat config: %w", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != configPerm {
		return fmt.Errorf("config file permissions are %04o, expected %04o", info.Mode().Perm(), configPerm)
	}

	return nil
}

// ActiveProfile returns the profile currently marked active.
// Falls back to "default" if active_profile is empty.
func ActiveProfile(cfg *Config) (*Profile, error) {
	name := cfg.ActiveProfile
	if name == "" {
		name = "default"
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", name)
	}
	return &p, nil
}

// ActiveProfileName returns the name of the active profile, "default" when
// active_profile is unset. It does not check that the profile exists.
func ActiveProfileName(cfg *Config) string {
	if cfg.ActiveProfile == "" {
		return "default"
	}
	return cfg.ActiveProfile
}

// SetAPIKey stores key in the active profile, creating the profile — and
// marking it active — when the config has none yet. It returns the profile's
// name. Nothing is written to disk; the caller saves.
func SetAPIKey(cfg *Config, key string) string {
	name := ActiveProfileName(cfg)
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]Profile)
	}
	p := cfg.Profiles[name]
	p.APIKey = key
	cfg.Profiles[name] = p

	if cfg.ActiveProfile == "" {
		cfg.ActiveProfile = name
	}
	cfg.Version = schemaVersion
	return name
}

// ClearAPIKey removes the API key from the active profile and returns the
// profile's name, and whether there was a key to remove. A profile that does
// not exist, or has no key, is not an error. Nothing is written to disk; the
// caller saves.
func ClearAPIKey(cfg *Config) (name string, had bool) {
	name = ActiveProfileName(cfg)
	p, ok := cfg.Profiles[name]
	if !ok || p.APIKey == "" {
		return name, false
	}
	p.APIKey = ""
	cfg.Profiles[name] = p
	return name, true
}

// SetProfile sets the active profile to name. Returns an error if the profile
// does not exist.
func SetProfile(cfg *Config, name string) error {
	if _, ok := cfg.Profiles[name]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	cfg.ActiveProfile = name
	return nil
}

// MergeKnownTenants adds tenants from codes into the KnownTenants list of the
// active profile, deduplicates, and sorts the result.
func MergeKnownTenants(cfg *Config, codes []string) error {
	name := cfg.ActiveProfile
	if name == "" {
		name = "default"
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return fmt.Errorf("profile %q not found", name)
	}

	seen := make(map[string]struct{}, len(p.KnownTenants)+len(codes))
	for _, t := range p.KnownTenants {
		seen[t] = struct{}{}
	}
	for _, t := range codes {
		seen[t] = struct{}{}
	}

	merged := make([]string, 0, len(seen))
	for t := range seen {
		merged = append(merged, t)
	}
	sort.Strings(merged)

	p.KnownTenants = merged
	cfg.Profiles[name] = p
	return nil
}
