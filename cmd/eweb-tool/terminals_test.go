package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/littlecxm/ruijie-eweb-tool/pkg/client"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/config"
	moduleauth "github.com/littlecxm/ruijie-eweb-tool/pkg/eweb-module/auth"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/eweb-module/devSta"
)

func TestWizardListsAndRefreshesTerminals(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/cgi-bin/luci/api/cmd" || r.URL.Query().Get("auth") != "private-session" {
			t.Error("menu did not use the logged-in session")
		}
		data := `{"code":"0","total":"2","list":[{"name":"手机","mac":"7c:c2:94:d1:84:ad","userIp":"192.168.60.106","connectType":"wireless","sn":"AP001","ssid":"CiGA","band":"2.4G"},{"hostName":"desktop","mac":"00:11:22:33:44:55","userIp":"192.168.60.107","connectType":"wire","port":3}]}`
		if calls == 2 {
			data = `{"total":"0","list":[]}`
		}
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
	}))
	defer server.Close()
	prompt := &fakePrompt{selections: []int{0, 0, 0, 7}, endpoint: server.URL, password: "password"}
	login := func(context.Context, *client.Client, string) (*moduleauth.LoginResult, error) {
		return loginResult("SN001"), nil
	}
	var output bytes.Buffer
	if err := runWizard(context.Background(), config.New(), filepath.Join(t.TempDir(), "config.toml"), false, prompt, login, &output); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(prompt.selections) != 0 {
		t.Fatalf("calls=%d, remaining choices=%v", calls, prompt.selections)
	}
	for _, text := range []string{"在线终端：2 台", "手机", "192.168.60.106", "7c:c2:94:d1:84:ad", "无线", "AP001", "CiGA", "2.4G", "desktop", "有线", "端口", "在线终端：0 台", "暂无在线终端"} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("missing %q in output: %s", text, output.String())
		}
	}
	if strings.Contains(output.String(), "private-session") {
		t.Fatal("printed SID")
	}
}

func TestDeviceMenuRecoversFromQueryFailure(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			io.WriteString(w, `{"code":0,"data":{"code":"1","msg":"collection failed"}}`)
			return
		}
		io.WriteString(w, `{"code":0,"data":{"total":0,"list":[]}}`)
	}))
	defer server.Close()
	c, err := client.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseIdleConnections()
	prompt := &fakePrompt{selections: []int{0, 0, 7}}
	var output bytes.Buffer
	if err := runDeviceMenu(context.Background(), c, "sid", prompt, &output); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !strings.Contains(output.String(), "列出在线终端失败") || !strings.Contains(output.String(), "collection failed") || !strings.Contains(output.String(), "暂无在线终端") {
		t.Fatalf("failed to recover: %s", output.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runDeviceMenu(ctx, c, "sid", &fakePrompt{}, &output); !errors.Is(err, context.Canceled) || calls != 2 {
		t.Fatalf("canceled menu: %v, calls=%d", err, calls)
	}
}

func TestPrintTerminalsFallbacks(t *testing.T) {
	var output bytes.Buffer
	users := &devSta.UserList{Total: 1, List: []devSta.Terminal{{HostName: "my\n desktop", DeviceAliasName: "客厅 AP"}}}
	if err := printTerminals(&output, users); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "my desktop") || !strings.Contains(output.String(), "客厅 AP") || !strings.Contains(output.String(), "-") {
		t.Fatalf("missing fallback values: %s", output.String())
	}
}
