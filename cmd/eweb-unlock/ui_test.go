package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestUnlockInputsAndSpinner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := newUnlockModel(ctx, cancel, nil)
	m.Init()
	if m.inputs[0].Value() != defaultEndpoint || m.View().AltScreen {
		t.Fatal("unexpected endpoint default or fullscreen UI")
	}
	m.inputs[0].SetValue("https://device/path")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.formError == nil || m.running || m.focus != 0 {
		t.Fatal("invalid endpoint was submitted")
	}
	m.inputs[0].SetValue(defaultEndpoint)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.formError == nil || m.running {
		t.Fatal("empty password was submitted")
	}
	const secret = " 密码🙂 with spaces "
	m.Update(tea.PasteMsg{Content: secret})
	view := ansi.Strip(m.View().Content)
	if m.inputs[1].Value() != secret || strings.Contains(view, secret) || !strings.Contains(view, "••") {
		t.Fatalf("password masking or whitespace preservation failed: %s", view)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focus != 0 {
		t.Fatal("Tab did not return to endpoint")
	}
	m.running = true
	m.Update(progressMsg("正在登录并获取设备信息…"))
	before := m.spinner.View()
	_, cmd := m.Update(spinner.TickMsg{ID: m.spinner.ID(), Time: time.Now()})
	if cmd == nil || m.spinner.View() == before {
		t.Fatal("spinner did not advance while working")
	}
	m.Update(progressMsg("正在开启开发者模式…"))
	view = ansi.Strip(m.View().Content)
	if strings.Contains(view, secret) || !strings.Contains(view, "✓ 正在登录并获取设备信息…") || !strings.Contains(view, "正在开启开发者模式…") {
		t.Fatalf("incorrect progress view: %s", view)
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil || !errors.Is(m.err, context.Canceled) || m.inputs[1].Value() != "" {
		t.Fatal("cancel did not clear password or quit")
	}
	// A worker finishing at the same time must not turn cancellation into success.
	m.Update(finishedMsg{result: &unlockResult{host: "device", password: "ssh-secret"}})
	if !errors.Is(m.err, context.Canceled) || m.result != nil {
		t.Fatal("late worker result overrode cancellation")
	}
}

func submitUnlockForm(m *unlockModel, password string) {
	m.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.send(tea.PasteMsg{Content: password})
	m.send(tea.KeyPressMsg{Code: tea.KeyEnter})
}

func TestUnlockUIWorkflowAndFinalResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out bytes.Buffer
	started := false
	var statuses []string
	work := func(_ context.Context, endpoint, password string, progress func(string)) (*unlockResult, error) {
		if endpoint != defaultEndpoint || password != "web-secret" {
			return nil, errors.New("incorrect form values")
		}
		progress("正在登录并获取设备信息…")
		progress("正在开启开发者模式…")
		return &unlockResult{host: "2001:db8::1", password: "ssh-secret"}, nil
	}
	err := runUnlockUI(ctx, nil, &out, work, tea.WithWindowSize(80, 24), tea.WithoutRenderer(), tea.WithFilter(func(state tea.Model, msg tea.Msg) tea.Msg {
		m := state.(*unlockModel)
		if strings.Contains(m.View().Content, "web-secret") || strings.Contains(m.View().Content, "ssh-secret") {
			t.Error("UI exposed a password before final output")
		}
		if _, ok := msg.(tea.WindowSizeMsg); ok && !started {
			started = true
			go submitUnlockForm(m, "web-secret")
		}
		if status, ok := msg.(progressMsg); ok {
			statuses = append(statuses, string(status))
		}
		return msg
	}))
	if err != nil {
		t.Fatal(err)
	}
	plain := ansi.Strip(out.String())
	if len(statuses) != 2 || !strings.HasPrefix(out.String(), "解锁成功\n") || !strings.Contains(plain, "SSH 密码: ssh-secret") || !strings.Contains(plain, "'root@[2001:db8::1]'") || strings.Contains(plain, "web-secret") {
		t.Fatalf("incorrect retained result: %s; statuses = %v", out.String(), statuses)
	}
}

func TestUnlockUICancellationWaitsForCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var cleaned atomic.Bool
	started := false
	work := func(ctx context.Context, _, _ string, progress func(string)) (*unlockResult, error) {
		defer cleaned.Store(true)
		progress("正在登录并获取设备信息…")
		<-ctx.Done()
		return nil, ctx.Err()
	}
	err := runUnlockUI(ctx, nil, io.Discard, work, tea.WithWindowSize(80, 24), tea.WithoutRenderer(), tea.WithFilter(func(state tea.Model, msg tea.Msg) tea.Msg {
		m := state.(*unlockModel)
		if _, ok := msg.(tea.WindowSizeMsg); ok && !started {
			started = true
			go submitUnlockForm(m, "web-secret")
		}
		if _, ok := msg.(progressMsg); ok {
			go m.send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		}
		return msg
	}))
	if !errors.Is(err, context.Canceled) || !cleaned.Load() {
		t.Fatalf("worker cleanup: error = %v, cleaned = %v", err, cleaned.Load())
	}
}
