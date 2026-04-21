package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the on-disk shape of ~/.config/ologi/config.toml.
type Config struct {
	APIKey     string `toml:"api_key"`
	DeviceID   string `toml:"device_id,omitempty"`
	DeviceName string `toml:"device_name,omitempty"`
	// ServerURL overrides OLOGI_SERVER_URL / the hardcoded default.
	// Used for dev. Omit empty for production writes.
	ServerURL string `toml:"server_url,omitempty"`
}

// Path returns the canonical location: $HOME/.config/ologi/config.toml.
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fall back to $HOME literal if UserHomeDir fails (very rare).
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".config", "ologi", "config.toml")
}

// Load reads the config. Returns os.ErrNotExist (wrapped) if the file
// is missing — callers can check with errors.Is(err, os.ErrNotExist)
// or os.IsNotExist.
func Load() (Config, error) {
	var cfg Config
	data, err := os.ReadFile(Path())
	if err != nil {
		return cfg, err
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// Save writes the config, creating the parent directory if needed,
// at mode 0600. An existing file is overwritten atomically via rename.
func Save(cfg Config) error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "config.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	defer os.Remove(tmp.Name()) // cleanup if rename fails

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}

	if err := toml.NewEncoder(tmp).Encode(cfg); err != nil {
		tmp.Close()
		return fmt.Errorf("encode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// Remove deletes the config file. Returns nil if the file didn't exist.
func Remove() error {
	err := os.Remove(Path())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
