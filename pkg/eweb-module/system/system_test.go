package system

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/littlecxm/ruijie-eweb-tool/pkg/client"
)

func TestSystemMethodContracts(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name, method, params, data, localMAC string
		invoke                               func(*client.Client) error
	}{
		{"language", "setLang", `{"lang":"zh_cn"}`, `{"result":true}`, "", func(c *client.Client) error { return SetLang(ctx, c, "sid", "zh_cn") }},
		{"version current", "getVersion", `{}`, `{"cur":"v1\n","new":[]}`, "", func(c *client.Client) error { _, e := GetVersion(ctx, c, "sid", ""); return e }},
		{"version self", "getVersion", `{"deviceType":"self"}`, `{"cur":"v1","new":{"version":"v2"}}`, "", func(c *client.Client) error { _, e := GetVersion(ctx, c, "sid", "self"); return e }},
		{"version bottom", "getVersion", `{"deviceType":"bottom"}`, `{"new":"cloud unavailable"}`, "", func(c *client.Client) error { _, e := GetVersion(ctx, c, "sid", "bottom"); return e }},
		{"time", "asyncTime", `null`, `"2026/11/03 12:34:56"`, "", func(c *client.Client) error { _, e := AsyncTime(ctx, c, "sid"); return e }},
		{"shell string param", "getRebootShell", `"user reboot"`, `"; sync; reboot"`, "", func(c *client.Client) error { _, e := GetRebootShell(ctx, c, "sid", "user reboot"); return e }},
		{"factory reset", "restoreConfiguration", `null`, `{"code":0,"shell":"firstboot"}`, "", func(c *client.Client) error { _, e := RestoreConfiguration(ctx, c, "sid"); return e }},
		{"reboot", "reboot", `null`, `{"code":0,"shell":"reboot"}`, "", func(c *client.Client) error { _, e := Reboot(ctx, c, "sid"); return e }},
		{"cleanup absent", "hasKillRemaining", `null`, `null`, "", func(c *client.Client) error {
			v, e := HasKillRemaining(ctx, c, "sid")
			if v {
				t.Error("null must mean absent")
			}
			return e
		}},
		{"cleanup present", "hasKillRemaining", `null`, `true`, "", func(c *client.Client) error {
			v, e := HasKillRemaining(ctx, c, "sid")
			if !v {
				t.Error("true must mean present")
			}
			return e
		}},
		{"cleanup void", "doKillRemaining", `null`, `null`, "", func(c *client.Client) error { return DoKillRemaining(ctx, c, "sid") }},
		{"upgrade flags", "upgradeConfirm", `{"path":"/tmp/firmware.img","check":true,"isPersist":true,"isEstAll":false}`, `{"code":0}`, "", func(c *client.Client) error {
			_, e := UpgradeConfirm(ctx, c, "sid", UpgradeOptions{Path: "/tmp/firmware.img", Check: true, IsPersist: true})
			return e
		}},
		{"upgrade network", "upgradeConfirm", `{"path":"/tmp/fw image.img","check":false,"isPersist":false,"isEstAll":true}`, `{"code":0}`, "", func(c *client.Client) error {
			_, e := UpgradeConfirm(ctx, c, "sid", UpgradeOptions{Path: "/tmp/fw image.img", IsEstAll: true})
			return e
		}},
		{"cancel upgrade", "upgradeCancel", `null`, `{"code":0,"msg":"delete fireware.img ok"}`, "", func(c *client.Client) error { _, e := UpgradeCancel(ctx, c, "sid"); return e }},
		{"session text", "getSessiontime", `null`, `"3600"`, "", func(c *client.Client) error {
			v, e := GetSessionTime(ctx, c, "sid")
			if v != 3600 {
				t.Errorf("seconds=%d", v)
			}
			return e
		}},
		{"session number", "getSessiontime", `null`, `3600`, "", func(c *client.Client) error {
			v, e := GetSessionTime(ctx, c, "sid")
			if v != 3600 {
				t.Errorf("seconds=%d", v)
			}
			return e
		}},
		{"backup", "hasBackupFile", `null`, `{"hasfile":false,"path":"/overlay/backup-config.tar.gz"}`, "", func(c *client.Client) error {
			v, e := HasBackupFile(ctx, c, "sid")
			if e == nil && (v.HasFile || v.Path != "/overlay/backup-config.tar.gz") {
				t.Errorf("backup=%+v", v)
			}
			return e
		}},
		{"restore default", "restoreConfig", `null`, `{"code":0,"addr":"192.168.110.1","_shell":"reboot"}`, "", func(c *client.Client) error { _, e := RestoreConfig(ctx, c, "sid", false); return e }},
		{"restore local MAC", "restoreConfig", `null`, `{"code":0}`, "1", func(c *client.Client) error { _, e := RestoreConfig(ctx, c, "sid", true); return e }},
		{"delete backup", "delRestoreConfig", `null`, `"ok"`, "", func(c *client.Client) error { _, e := DelRestoreConfig(ctx, c, "sid"); return e }},
		{"step complete", "setStep", `{"steped":"1"}`, `{"result":true}`, "", func(c *client.Client) error { return SetStep(ctx, c, "sid", "1") }},
		{"step incomplete", "setStep", `{"steped":"0"}`, `{"result":true}`, "", func(c *client.Client) error { return SetStep(ctx, c, "sid", "0") }},
		{"space", "getOverlayFreeSpace", `null`, `"123456\n"`, "", func(c *client.Client) error {
			v, e := GetOverlayFreeSpace(ctx, c, "sid")
			if v != "123456\n" {
				t.Errorf("raw space=%q", v)
			}
			return e
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/cgi-bin/luci/api/system" || r.URL.Query().Get("auth") != "sid" || r.URL.Query().Get("localMac") != tc.localMAC {
					t.Error("incorrect authenticated system request")
				}
				var req struct {
					Method string          `json:"method"`
					Params json.RawMessage `json:"params"`
				}
				if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
					t.Error(e)
				}
				var got, want any
				json.Unmarshal(req.Params, &got)
				json.Unmarshal([]byte(tc.params), &want)
				if req.Method != tc.method || !reflect.DeepEqual(got, want) {
					t.Errorf("request=%s %s", req.Method, req.Params)
				}
				io.WriteString(w, `{"code":0,"error":null,"data":`+tc.data+`}`)
			}))
			defer server.Close()
			c, e := client.New(server.URL)
			if e != nil {
				t.Fatal(e)
			}
			defer c.CloseIdleConnections()
			if e := tc.invoke(c); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestSystemBusinessErrors(t *testing.T) {
	cases := []struct {
		data   string
		invoke func(*client.Client) error
		code   int
	}{
		{`{"code":1,"msg":"firmware missing"}`, func(c *client.Client) error {
			_, e := UpgradeConfirm(context.Background(), c, "sid", UpgradeOptions{Path: "/tmp/firmware.img", Check: true})
			return e
		}, 1},
		{`{"code":-1,"msg":"params illegal"}`, func(c *client.Client) error { _, e := UpgradeCancel(context.Background(), c, "sid"); return e }, -1},
		{`{"code":1,"msg":"not supports"}`, func(c *client.Client) error { _, e := RestoreConfig(context.Background(), c, "sid", false); return e }, 1},
		{`{}`, func(c *client.Client) error { _, e := Reboot(context.Background(), c, "sid"); return e }, 0},
		{`{"result":false}`, func(c *client.Client) error { return SetLang(context.Background(), c, "sid", "unknown") }, 0},
		{`{}`, func(c *client.Client) error { return SetStep(context.Background(), c, "sid", "1") }, 0},
		{`"true"`, func(c *client.Client) error { _, e := HasKillRemaining(context.Background(), c, "sid"); return e }, 0},
		{`{}`, func(c *client.Client) error { return DoKillRemaining(context.Background(), c, "sid") }, 0},
		{`"-1"`, func(c *client.Client) error { _, e := GetSessionTime(context.Background(), c, "sid"); return e }, 0},
	}
	for _, tc := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{"code":0,"data":`+tc.data+`}`) }))
		c, _ := client.New(server.URL)
		err := tc.invoke(c)
		if err == nil {
			t.Errorf("accepted invalid result %s", tc.data)
		}
		if tc.code != 0 {
			var apiErr *client.APIError
			if !errors.As(err, &apiErr) || apiErr.Code != tc.code {
				t.Errorf("business error=%v", err)
			}
		}
		c.CloseIdleConnections()
		server.Close()
	}
}

func TestSystemValidationAndCancellation(t *testing.T) {
	c, _ := client.New("http://device")
	defer c.CloseIdleConnections()
	ctx := context.Background()
	for _, p := range []string{"", "relative.img", "/tmp/fw;reboot", "/tmp/fw\nimg", "/tmp/$fw"} {
		if _, e := UpgradeConfirm(ctx, c, "sid", UpgradeOptions{Path: p}); e == nil {
			t.Errorf("accepted path %q", p)
		}
	}
	if _, e := GetVersion(ctx, c, "sid", "invalid"); e == nil {
		t.Fatal("accepted invalid version target")
	}
	if e := SetStep(ctx, c, "sid", "true"); e == nil {
		t.Fatal("accepted invalid step")
	}
	if e := SetLang(ctx, c, "sid", " "); e == nil {
		t.Fatal("accepted empty language")
	}
	if _, e := AsyncTime(ctx, c, ""); e == nil {
		t.Fatal("accepted missing SID")
	}
	if _, e := HasKillRemaining(ctx, c, ""); e == nil {
		t.Fatal("accepted missing SID for raw call")
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	_, e := Reboot(ctx, c, "private-session")
	if !errors.Is(e, context.Canceled) || strings.Contains(e.Error(), "private-session") {
		t.Fatalf("cancellation=%v", e)
	}
}
