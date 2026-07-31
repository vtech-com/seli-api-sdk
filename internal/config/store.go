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

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, configPerm); err != nil {
		return fmt.Errorf("write temp config: %w", err)
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
