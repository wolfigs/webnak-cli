// Package config keeps the CLI's settings and sign-in on disk.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config is what the CLI remembers between runs.
type Config struct {
	API       string `json:"api,omitempty"`
	Token     string `json:"token,omitempty"`
	Email     string `json:"email,omitempty"`
	Workspace string `json:"workspace,omitempty"`
}

// Dir is where the config lives: ~/.config/webnak on Linux, ~/Library/Application
// Support/webnak on macOS, %AppData%\webnak on Windows. WEBNAK_CONFIG_DIR wins.
func Dir() (string, error) {
	if d := os.Getenv("WEBNAK_CONFIG_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "webnak"), nil
}

func file() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

// Load reads the config; a missing file is an empty config.
func Load() (*Config, error) {
	f, err := file()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(f)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	c := &Config{}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, err
	}
	return c, nil
}

// Save writes the config, readable only by the user: it holds a sign-in token.
func Save(c *Config) error {
	f, err := file()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := f + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f)
}
