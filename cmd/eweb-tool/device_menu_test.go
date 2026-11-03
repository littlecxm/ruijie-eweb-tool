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
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/littlecxm/ruijie-eweb-tool/internal/tui"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/client"
)

type canceledInputPrompt struct{ scriptedPrompt }

func (*canceledInputPrompt) Input(string, string) (string, error) { return "", tui.ErrCanceled }

func TestCanceledSettingReturnsToSubmenuWithoutRequest(t *testing.T) {
	c, _ := client.New("http://device")
	defer c.CloseIdleConnections()
	prompt := &canceledInputPrompt{scriptedPrompt{choices: []string{"系统设置", "设置 Web 语言", backMenu, exitWizard}}}
	if err := runDeviceMenu(context.Background(), c, "sid", prompt, io.Discard); err != nil || len(prompt.choices) != 0 {
		t.Fatalf("cancel setting: %v, choices=%v", err, prompt.choices)
	}
}

type scriptedPrompt struct {
	choices       []string
	inputs        []string
	confirmations []string
}

func (p *scriptedPrompt) Select(label string, options []string) (string, error) {
	if slices.Contains(options, confirmAction) {
		p.confirmations = append(p.confirmations, label)
		if options[0] != cancelAction {
			return "", errors.New("confirmation must default to cancel")
		}
	}
	if len(p.choices) == 0 {
		return "", io.EOF
	}
	choice := p.choices[0]
	p.choices = p.choices[1:]
	if !slices.Contains(options, choice) {
		return "", fmt.Errorf("unexpected option %q in %v", choice, options)
	}
	return choice, nil
}

func (p *scriptedPrompt) Input(_ string, value string) (string, error) {
	if len(p.inputs) == 0 {
		return value, nil
	}
	value = p.inputs[0]
	p.inputs = p.inputs[1:]
	return value, nil
}

func (*scriptedPrompt) Password(string) (string, error) {
	return "", errors.New("unexpected password prompt")
}

func menuServer(t *testing.T, handler http.HandlerFunc) (*client.Client, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	c, err := client.New(server.URL)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	return c, func() { c.CloseIdleConnections(); server.Close() }
}

func TestSubmenuNavigationAndQueryRecovery(t *testing.T) {
	var methods []string
	hostnameCalls := 0
	c, closeServer := menuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("auth") != "sid" {
			t.Error("wrong session")
		}
		var req client.ApiCommonReq
		json.NewDecoder(r.Body).Decode(&req)
		methods = append(methods, req.Method)
		data := `"2026/11/03 12:34:56"`
		if req.Method == "getHostName" {
			hostnameCalls++
			if hostnameCalls == 1 {
				io.WriteString(w, `{"code":0,"error":{"message":"temporary error"}}`)
				return
			}
			data = `{"hostname":"Ruijie-test"}`
		}
		io.WriteString(w, `{"code":0,"data":`+data+`}`)
	})
	defer closeServer()
	prompt := &scriptedPrompt{choices: []string{"设备概览", "主机名", "主机名", backMenu, "系统设置", "设备时间", backMenu, exitWizard}}
	var out bytes.Buffer
	if err := runDeviceMenu(context.Background(), c, "sid", prompt, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(methods, []string{"getHostName", "getHostName", "asyncTime"}) || len(prompt.choices) != 0 {
		t.Fatalf("requests=%v, remaining=%v", methods, prompt.choices)
	}
	for _, text := range []string{"主机名失败", "temporary error", "Ruijie-test", "2026/11/03 12:34:56"} {
		if !strings.Contains(out.String(), text) {
			t.Errorf("missing %q: %s", text, out.String())
		}
	}
}

func TestMaintenanceCancellationAndSessionEnd(t *testing.T) {
	for _, action := range []string{"重启设备", "恢复出厂配置"} {
		t.Run(action, func(t *testing.T) {
			var calls int
			c, closeServer := menuServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				var req client.ApiCommonReq
				json.NewDecoder(r.Body).Decode(&req)
				want := "reboot"
				if action == "恢复出厂配置" {
					want = "restoreConfiguration"
				}
				if req.Method != want {
					t.Errorf("method=%s", req.Method)
				}
				io.WriteString(w, `{"code":0,"data":{"code":0}}`)
			})
			defer closeServer()
			prompt := &scriptedPrompt{choices: []string{"系统设置", action, cancelAction, action, confirmAction}}
			var out bytes.Buffer
			if err := runDeviceMenu(context.Background(), c, "sid", prompt, &out); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || len(prompt.choices) != 0 || len(prompt.confirmations) != 2 || !strings.Contains(out.String(), "当前会话结束") {
				t.Fatalf("calls=%d, prompt=%+v, output=%s", calls, prompt, out.String())
			}
			if !strings.Contains(prompt.confirmations[0], c.Endpoint()) {
				t.Fatal("confirmation missing target device")
			}
		})
	}
}

func TestBackupRecoveryRequiresExistingFileAndSendsQuery(t *testing.T) {
	var calls []string
	c, closeServer := menuServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req client.ApiCommonReq
		json.NewDecoder(r.Body).Decode(&req)
		calls = append(calls, req.Method)
		if req.Method == "hasBackupFile" {
			io.WriteString(w, `{"code":0,"data":{"hasfile":true,"path":"/overlay/backup-config.tar.gz"}}`)
			return
		}
		if req.Method != "restoreConfig" || r.URL.Query().Get("localMac") != "1" || req.Params != nil {
			t.Errorf("restore request=%+v, query=%v", req, r.URL.Query())
		}
		io.WriteString(w, `{"code":0,"data":{"code":0}}`)
	})
	defer closeServer()
	prompt := &scriptedPrompt{choices: []string{"系统设置", "恢复设备备份", "启用 localMac 选项", confirmAction}}
	var out bytes.Buffer
	if err := runDeviceMenu(context.Background(), c, "sid", prompt, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"hasBackupFile", "restoreConfig"}) {
		t.Fatalf("calls=%v", calls)
	}
}

func TestUpgradeUsesTypedFlagsAndEndsSession(t *testing.T) {
	var params map[string]any
	c, closeServer := menuServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Method != "upgradeConfirm" {
			t.Errorf("method=%s", req.Method)
		}
		params = req.Params
		io.WriteString(w, `{"code":0,"data":{"code":0}}`)
	})
	defer closeServer()
	prompt := &scriptedPrompt{choices: []string{"系统设置", "提交固件升级", "不保留配置", "本机", confirmAction}}
	var out bytes.Buffer
	if err := runDeviceMenu(context.Background(), c, "sid", prompt, &out); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"path": "/tmp/firmware.img", "check": true, "isPersist": false, "isEstAll": false}
	if !reflect.DeepEqual(params, want) || !strings.Contains(out.String(), "当前会话结束") {
		t.Fatalf("params=%v, output=%s", params, out.String())
	}
	if len(prompt.confirmations) != 1 || !strings.Contains(prompt.confirmations[0], "不保留配置") {
		t.Fatal("confirmation omitted config loss")
	}
}

func TestMissingBackupAndCleanupDoNotMutateDevice(t *testing.T) {
	var methods []string
	c, closeServer := menuServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req client.ApiCommonReq
		json.NewDecoder(r.Body).Decode(&req)
		methods = append(methods, req.Method)
		switch req.Method {
		case "hasBackupFile":
			io.WriteString(w, `{"code":0,"data":{"hasfile":false}}`)
		case "hasKillRemaining":
			io.WriteString(w, `{"code":0,"data":null}`)
		default:
			t.Errorf("unexpected mutation %s", req.Method)
		}
	})
	defer closeServer()
	prompt := &scriptedPrompt{choices: []string{"系统设置", "恢复设备备份", "执行升级清理脚本", backMenu, exitWizard}}
	var out bytes.Buffer
	if err := runDeviceMenu(context.Background(), c, "sid", prompt, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(methods, []string{"hasBackupFile", "hasKillRemaining"}) || !strings.Contains(out.String(), "没有可恢复") || !strings.Contains(out.String(), "没有升级清理脚本") {
		t.Fatalf("requests=%v, output=%s", methods, out.String())
	}
}

func TestSubmenuInteractionErrorPreservesCause(t *testing.T) {
	c, _ := client.New("http://device")
	defer c.CloseIdleConnections()
	prompt := &scriptedPrompt{choices: []string{"系统设置", "设置 Web 语言"}, inputs: []string{"zh_cn"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runDeviceMenu(ctx, c, "sid", prompt, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
	prompt = &scriptedPrompt{choices: []string{"系统设置"}}
	if err := runDeviceMenu(context.Background(), c, "sid", prompt, io.Discard); !errors.Is(err, io.EOF) {
		t.Fatalf("prompt failure=%v", err)
	}
}
