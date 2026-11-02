package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	App    AppConfig               `toml:"app"`
	Device map[string]DeviceConfig `toml:"device"`
}

type AppConfig struct {
	Debug bool `toml:"debug"`
}

type KeychipConfig struct {
	List []string `toml:"list"`
}

type DeviceConfig struct {
	Endpoint string `toml:"endpoint"`
	Password string `toml:"password"`
}

var cfg = New()

func New() *Config {
	return &Config{Device: make(map[string]DeviceConfig)}
}

func LoadFromFile(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	return Load(b)
}

func Load(b []byte) (*Config, error) {
	next := New()
	if err := toml.Unmarshal(b, next); err != nil {
		// TOML parse errors can include source lines containing passwords.
		return nil, fmt.Errorf("config: invalid TOML: %w", &parseError{cause: err})
	}
	cfg = next
	return next, nil
}

// SaveToFile preserves the existing package-level API.
func SaveToFile(path string) error {
	return cfg.SaveToFile(path)
}

// SaveToFile atomically replaces the configuration with an owner-only file.
func (c *Config) SaveToFile(path string) error {
	b, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return fmt.Errorf("config: create temporary file: %w", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return fmt.Errorf("config: set file permissions: %w", err)
	}
	if _, err := file.Write(b); err != nil {
		return fmt.Errorf("config: write: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("config: sync: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("config: close: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("config: replace %s: %w", path, err)
	}
	return nil
}

func GetConfig() *Config {
	return cfg
}

func GetDebug() bool {
	cfg := GetConfig()
	return cfg.App.Debug
}

// parseError keeps the underlying parser error available to errors.As without
// including potentially sensitive TOML source text in command-line output.
type parseError struct{ cause error }

func (e *parseError) Error() string { return "could not decode configuration" }
func (e *parseError) Unwrap() error { return e.cause }
