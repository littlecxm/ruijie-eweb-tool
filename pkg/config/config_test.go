package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestLoadResetsState(t *testing.T) {
	first, err := Load([]byte("[app]\ndebug = true\n[device.A]\nendpoint = 'https://device-a'\npassword = 'old'\n"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Load([]byte("[device.B]\nendpoint = 'https://device-b'\npassword = 'new'\n"))
	if err != nil {
		t.Fatal(err)
	}
	if second.App.Debug || len(second.Device) != 1 || second.Device["B"].Password != "new" || GetConfig() != second || GetDebug() {
		t.Fatal("load retained previous state")
	}
	if len(first.Device) != 1 || first.Device["A"].Password != "old" || !first.App.Debug {
		t.Fatal("load changed previously returned configuration")
	}
	if _, err := Load([]byte("[app]\ndebug = [ secret-password")); err == nil || strings.Contains(err.Error(), "secret-password") {
		t.Fatalf("expected safe TOML parse error: %v", err)
	} else {
		var decodeError *toml.DecodeError
		if !errors.As(err, &decodeError) {
			t.Fatalf("lost original parser cause: %v", err)
		}
	}
	if GetConfig() != second {
		t.Fatal("invalid load replaced working config")
	}
}

func TestAtomicSaveAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	c := New()
	c.App.Debug = true
	c.Device["A"] = DeviceConfig{Endpoint: "https://device-a", Password: "encrypted-a"}
	if err := c.SaveToFile(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	c.Device["B"] = DeviceConfig{Endpoint: "https://device-b", Password: "encrypted-b"}
	if err := c.SaveToFile(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("wrong permissions: %v, %v", info, err)
	}
	loaded, err := LoadFromFile(path)
	if err != nil || !loaded.App.Debug || len(loaded.Device) != 2 || loaded.Device["A"] != c.Device["A"] || loaded.Device["B"] != c.Device["B"] {
		t.Fatalf("save lost settings: %v", err)
	}
	if err := SaveToFile(path); err != nil {
		t.Fatal(err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".config-*.toml"))
	if len(leftovers) != 0 {
		t.Fatal("temporary config was not cleaned up")
	}
}

func TestFileErrorsPreserveCauseAndContents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if _, err := LoadFromFile(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lost not-exist cause: %v", err)
	}
	if _, err := LoadFromFile(dir); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatal("directory incorrectly treated as missing config")
	}
	if err := os.WriteFile(path, []byte("original-config"), 0600); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		t.Skip("file permission failure requires an unprivileged user")
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0700)
	if err := New().SaveToFile(path); err == nil || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected permission error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "original-config" {
		t.Fatal("failed save changed original config")
	}
}

func TestRenameFailureCleansTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config.toml")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := New().SaveToFile(target); err == nil {
		t.Fatal("replaced a directory with config")
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".config-*.toml"))
	if len(leftovers) != 0 {
		t.Fatal("failed rename left temporary config")
	}
}
