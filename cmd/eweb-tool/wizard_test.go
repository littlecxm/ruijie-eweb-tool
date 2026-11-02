package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	passwordauth "github.com/littlecxm/ruijie-eweb-tool/pkg/auth"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/client"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/config"
	moduleauth "github.com/littlecxm/ruijie-eweb-tool/pkg/eweb-module/auth"
	"github.com/pelletier/go-toml/v2"
)

type fakePrompt struct {
	selections      []int
	endpoint        string
	password        string
	passwordCalls   int
	endpointDefault string
}

func (p *fakePrompt) Select(_ string, options []string) (string, error) {
	if len(p.selections) == 0 {
		return "", errors.New("unexpected selection prompt")
	}
	n := p.selections[0]
	p.selections = p.selections[1:]
	if n < 0 || n >= len(options) {
		return "", fmt.Errorf("invalid selection index %d", n)
	}
	return options[n], nil
}

func (p *fakePrompt) Input(_ string, value string) (string, error) {
	p.endpointDefault = value
	if p.endpoint == "" {
		return value, nil
	}
	return p.endpoint, nil
}

func (p *fakePrompt) Password(_ string) (string, error) {
	p.passwordCalls++
	return p.password, nil
}

func loginResult(sn string) *moduleauth.LoginResult {
	return &moduleauth.LoginResult{
		Session:   moduleauth.Session{SN: sn, SID: "private-session", Token: "private-token"},
		Authority: moduleauth.Authority{Sysinfo: moduleauth.SystemInfo{SerialNum: sn, ModelFullName: "test model"}},
	}
}

func TestNewDeviceDefaultAndSavedLogin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := config.New()
	prompt := &fakePrompt{selections: []int{0}, password: "secret-password"}
	var output bytes.Buffer
	login := func(_ context.Context, c *client.Client, password string) (*moduleauth.LoginResult, error) {
		if c.Endpoint() != "https://192.168.110.1" || password != "secret-password" {
			t.Fatal("wrong default endpoint or password")
		}
		return loginResult("SN001"), nil
	}
	if err := runWizard(context.Background(), cfg, path, false, prompt, login, &output); err != nil {
		t.Fatal(err)
	}
	if prompt.endpointDefault != defaultEndpoint || prompt.passwordCalls != 1 {
		t.Fatal("incorrect new device prompts")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secret-password", "private-session", "private-token"} {
		if bytes.Contains(data, []byte(secret)) || strings.Contains(output.String(), secret) {
			t.Fatalf("persisted or printed secret field %s", secret)
		}
	}
	var stored struct {
		Device map[string]map[string]any `toml:"device"`
	}
	if err := toml.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	fields := stored.Device["SN001"]
	if len(fields) != 2 || fields["endpoint"] == nil || fields["password"] == nil {
		t.Fatal("device config must contain only endpoint and encrypted password")
	}
	loaded, err := config.LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := passwordauth.DecryptAuthKey(loaded.Device["SN001"].Password, passwordauth.StoragePassphrase)
	if err != nil || plain != "secret-password" {
		t.Fatal("password was not saved as compatible ciphertext")
	}
	savedPrompt := &fakePrompt{selections: []int{0, 0}}
	if err := runWizard(context.Background(), loaded, path, false, savedPrompt, login, &output); err != nil {
		t.Fatal(err)
	}
	if savedPrompt.passwordCalls != 0 {
		t.Fatal("saved password requested again")
	}
}

func TestLegacyAndExplicitPasswordUpdates(t *testing.T) {
	for _, mode := range []int{0, 1} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			cfg := config.New()
			cfg.App.Debug = true
			cfg.Device["SN001"] = config.DeviceConfig{Endpoint: "https://device", Password: "old-plaintext"}
			cfg.Device["SN002"] = config.DeviceConfig{Endpoint: "https://other-device", Password: "other-ciphertext"}
			path := filepath.Join(t.TempDir(), "config.toml")
			prompt := &fakePrompt{selections: []int{0, mode}, password: "new-password"}
			login := func(_ context.Context, _ *client.Client, password string) (*moduleauth.LoginResult, error) {
				if password != "new-password" {
					t.Fatal("legacy password was reused")
				}
				return loginResult("SN001"), nil
			}
			var output bytes.Buffer
			if err := runWizard(context.Background(), cfg, path, false, prompt, login, &output); err != nil {
				t.Fatal(err)
			}
			if prompt.passwordCalls != 1 || !cfg.App.Debug || cfg.Device["SN002"].Password != "other-ciphertext" {
				t.Fatal("update lost config or did not prompt")
			}
			if strings.Contains(output.String(), "old-plaintext") || strings.Contains(output.String(), "new-password") {
				t.Fatal("printed a password")
			}
			if plain, err := passwordauth.DecryptAuthKey(cfg.Device["SN001"].Password, passwordauth.StoragePassphrase); err != nil || plain != "new-password" {
				t.Fatal("password update failed")
			}
		})
	}
}

func TestFailedLoginDoesNotWrite(t *testing.T) {
	cfg := config.New()
	path := filepath.Join(t.TempDir(), "config.toml")
	prompt := &fakePrompt{selections: []int{0}, password: "admin", endpoint: "https://192.168.110.1"}
	login := func(_ context.Context, c *client.Client, _ string) (*moduleauth.LoginResult, error) {
		if c.Endpoint() != "https://192.168.110.1" {
			t.Fatal("custom endpoint ignored")
		}
		return nil, errors.New("login rejected")
	}
	if err := runWizard(context.Background(), cfg, path, false, prompt, login, &bytes.Buffer{}); err == nil {
		t.Fatal("ignored login failure")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) || len(cfg.Device) != 0 {
		t.Fatal("failed login saved config")
	}
}

func TestIdentityAndSaveFailuresPreserveConfiguration(t *testing.T) {
	for _, scenario := range []string{"different device", "missing serial", "inconsistent session", "write failure"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := config.New()
			cfg.Device["SN001"] = config.DeviceConfig{Endpoint: "https://old-device", Password: "unchanged"}
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := cfg.SaveToFile(path); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(path)
			result := loginResult("SN001")
			writePath := path
			switch scenario {
			case "different device":
				result = loginResult("SN002")
			case "missing serial":
				result.Authority.Sysinfo.SerialNum = ""
			case "inconsistent session":
				result.Session.SN = "OTHER"
			case "write failure":
				writePath = filepath.Dir(path)
			}
			if err := saveLogin(cfg, writePath, "SN001", "https://new-device", "new-password", result); err == nil {
				t.Fatal("expected save failure")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) || cfg.Device["SN001"].Password != "unchanged" || len(cfg.Device) != 1 {
				t.Fatal("failure changed saved or in-memory configuration")
			}
		})
	}
}

func TestExitDoesNotLoginOrCreateConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	prompt := &fakePrompt{selections: []int{1}}
	login := func(context.Context, *client.Client, string) (*moduleauth.LoginResult, error) {
		t.Fatal("exit logged in")
		return nil, nil
	}
	if err := runWizard(context.Background(), config.New(), path, false, prompt, login, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("exit created config")
	}
}
