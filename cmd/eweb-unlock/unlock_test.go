package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	passwordauth "github.com/littlecxm/ruijie-eweb-tool/pkg/auth"
)

const testPassphrase = "0123456789abcdef0123456789abcdef"

func unlockDevice(t *testing.T, sn, failAt string) (*httptest.Server, *[]string) {
	t.Helper()
	var events []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var stage string
		switch r.URL.Path {
		case "/cgi-bin/luci/":
			stage = "page"
		case "/cgi-bin/luci/api/auth":
			stage = "login"
		case "/cgi-bin/luci/api/common":
			stage = "authority"
		case "/cgi-bin/luci/api/cmd":
			stage = "develop"
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		events = append(events, stage)
		if stage == failAt {
			io.WriteString(w, `{"code":1,"error":"rejected"}`)
			return
		}
		if stage == "authority" || stage == "develop" {
			if r.URL.Query().Get("auth") != "sid +/?" {
				t.Error("missing authenticated session")
			}
		}
		switch stage {
		case "page":
			fmt.Fprintf(w, `GibberishAES.enc(password, "%s")`, testPassphrase)
		case "login":
			var request struct {
				Method string `json:"method"`
				Params struct {
					Username string `json:"username"`
					Pwd      string `json:"pwd"`
				} `json:"params"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			plain, err := passwordauth.DecryptAuthKey(request.Params.Pwd, testPassphrase)
			if err != nil || plain != "web-password" || request.Method != "login" || request.Params.Username != "admin" {
				t.Error("incorrect encrypted admin login")
			}
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"sid": "sid +/?", "token": "private-token", "sn": sn}})
		case "authority":
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"sysinfo": map[string]any{"serial_num": sn}}})
		case "develop":
			io.WriteString(w, `{"code":0,"data":{"result":"success"}}`)
		}
	}))
	t.Cleanup(server.Close)
	return server, &events
}

func TestRunUnlock(t *testing.T) {
	for _, tc := range []struct {
		name, failAt, sn, wantError string
		verifyError                 error
		cancelAfterVerify, saved    bool
		wantEvents                  []string
	}{
		{name: "success", sn: "SN001", wantEvents: []string{"page", "login", "authority", "develop", "ssh"}},
		{name: "existing config", sn: "SN001", saved: true, wantEvents: []string{"page", "login", "authority", "develop", "ssh"}},
		{name: "login failed", sn: "SN001", failAt: "login", wantError: "登录失败", wantEvents: []string{"page", "login"}},
		{name: "authority failed", sn: "SN001", failAt: "authority", wantError: "登录失败", wantEvents: []string{"page", "login", "authority"}},
		{name: "setting failed", sn: "SN001", failAt: "develop", saved: true, wantError: "开启开发者模式失败", wantEvents: []string{"page", "login", "authority", "develop"}},
		{name: "invalid SN", sn: "12345678901234", wantError: "计算 SSH 密码失败", wantEvents: []string{"page", "login", "authority", "develop"}},
		{name: "SSH failed", sn: "SN001", verifyError: errors.New("authentication rejected"), wantError: "SSH 验证失败", wantEvents: []string{"page", "login", "authority", "develop", "ssh"}},
		{name: "canceled before output", sn: "SN001", cancelAfterVerify: true, wantError: "context canceled", wantEvents: []string{"page", "login", "authority", "develop", "ssh"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			const savedConfig = "invalid TOML: must not be read or changed"
			if tc.saved {
				if err := os.WriteFile("config.toml", []byte(savedConfig), 0600); err != nil {
					t.Fatal(err)
				}
			}
			server, events := unlockDevice(t, tc.sn, tc.failAt)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var out bytes.Buffer
			sshPassword, _ := passwordauth.SshPasswdGenerate(tc.sn)
			verify := func(_ context.Context, address, password string) error {
				*events = append(*events, "ssh")
				if address != "127.0.0.1:54133" || password != sshPassword {
					t.Errorf("incorrect SSH target or password: %s", address)
				}
				if strings.Contains(out.String(), sshPassword) {
					t.Error("password displayed before authentication")
				}
				if tc.cancelAfterVerify {
					cancel()
				}
				return tc.verifyError
			}
			result, err := runUnlock(ctx, server.URL, "web-password", false, verify, func(message string) {
				fmt.Fprintln(&out, message)
			})
			if result != nil {
				out.WriteString(ansi.Strip(renderUnlockResult(result)))
			}
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v; want %s", err, tc.wantError)
				}
				if strings.Contains(out.String(), "解锁成功") || strings.Contains(out.String(), "SSH 密码:") || (sshPassword != "" && strings.Contains(out.String(), sshPassword)) {
					t.Fatalf("password or success shown on failure: %s", &out)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				for _, want := range []string{"解锁成功", "主机: 127.0.0.1", "用户名: root", "端口: 54133", "SSH 密码: " + sshPassword, "ssh -oHostKeyAlgorithms=+ssh-rsa -p 54133 'root@127.0.0.1'"} {
					if !strings.Contains(out.String(), want) {
						t.Errorf("output is missing %q", want)
					}
				}
			}
			if !reflect.DeepEqual(*events, tc.wantEvents) {
				t.Errorf("events = %v; want %v", *events, tc.wantEvents)
			}
			for _, secret := range []string{"web-password", "sid +/?", "private-token"} {
				if strings.Contains(out.String(), secret) {
					t.Error("output exposed a Web credential")
				}
			}
			files, err := os.ReadDir(".")
			if err != nil {
				t.Fatal(err)
			}
			if tc.saved {
				data, err := os.ReadFile("config.toml")
				if err != nil || string(data) != savedConfig || len(files) != 1 {
					t.Fatal("changed existing configuration or wrote another file")
				}
			} else if len(files) != 0 {
				t.Fatal("unexpected files written during unlock")
			}
		})
	}
}

func TestRunUnlockTLSAndCancellation(t *testing.T) {
	device, events := unlockDevice(t, "SN001", "")
	server := httptest.NewTLSServer(device.Config.Handler)
	defer server.Close()
	verify := func(context.Context, string, string) error { return nil }
	if _, err := runUnlock(context.Background(), server.URL, "web-password", false, verify, nil); err != nil {
		t.Fatalf("default must support self-signed HTTPS: %v", err)
	}
	*events = nil
	if _, err := runUnlock(context.Background(), server.URL, "web-password", true, verify, nil); err == nil || len(*events) != 0 {
		t.Fatalf("TLS verification did not reject self-signed device: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runUnlock(ctx, device.URL, "web-password", false, verify, nil); !errors.Is(err, context.Canceled) || len(*events) != 0 {
		t.Fatalf("canceled workflow made a request: %v", err)
	}
	if _, err := runUnlock(context.Background(), "https://device/path", "web-password", false, verify, nil); err == nil {
		t.Fatal("accepted invalid endpoint")
	}
}

func TestRunHelpAndNonTerminal(t *testing.T) {
	in, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	for _, args := range [][]string{{"--help"}, {"-h"}} {
		var out bytes.Buffer
		if err := run(context.Background(), args, in, &out); err != nil || !strings.Contains(out.String(), "verify-tls") {
			t.Fatalf("help = %q, %v", &out, err)
		}
	}
	for _, args := range [][]string{nil, {"extra"}, {"--unknown"}} {
		if err := run(context.Background(), args, in, io.Discard); err == nil {
			t.Fatalf("accepted non-terminal or invalid arguments: %v", args)
		}
	}
}
