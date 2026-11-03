package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestRunBridgesWorkflowAndPreservesError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wantErr := errors.New("device unavailable")
	secret := "private-password-🙂"
	var leaked bool
	err := Run(ctx, "test", io.Discard, func(ctx context.Context, ui *UI) error {
		choice, err := ui.Select("设备", []string{"设备一", "退出"})
		if err != nil {
			return err
		}
		if choice != "设备一" {
			return fmt.Errorf("wrong selection: %s", choice)
		}
		password, err := ui.Password("密码")
		if err != nil {
			return err
		}
		if password != secret {
			return errors.New("password changed")
		}
		fmt.Fprintln(ui, "已查询设备")
		return wantErr
	}, tea.WithWindowSize(110, 32), tea.WithInput(nil), tea.WithoutRenderer(), tea.WithoutSignalHandler(), tea.WithFilter(func(state tea.Model, msg tea.Msg) tea.Msg {
		m := state.(*model)
		if strings.Contains(m.View().Content, secret) || strings.Contains(m.history, secret) {
			leaked = true
		}
		switch value := msg.(type) {
		case promptMsg:
			go func() {
				if value.kind == passwordPrompt {
					m.ui.send(tea.PasteMsg{Content: secret})
				}
				m.ui.send(key(tea.KeyEnter))
			}()
		case finishedMsg:
			go m.ui.send(key(tea.KeyEnter))
		}
		return msg
	}))
	if !errors.Is(err, wantErr) {
		t.Fatalf("workflow error=%v", err)
	}
	if leaked {
		t.Fatal("password leaked to rendered UI or history")
	}
}

func TestRunCancellationWaitsForWorkerCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var cleaned atomic.Bool
	err := Run(ctx, "test", io.Discard, func(ctx context.Context, ui *UI) error {
		fmt.Fprintln(ui, "任务开始")
		<-ctx.Done()
		defer cleaned.Store(true)
		_, err := ui.Input("已取消的输入", "")
		return err
	}, tea.WithWindowSize(110, 32), tea.WithInput(nil), tea.WithoutRenderer(), tea.WithoutSignalHandler(), tea.WithFilter(func(state tea.Model, msg tea.Msg) tea.Msg {
		if _, ok := msg.(outputMsg); ok {
			go state.(*model).ui.send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		}
		return msg
	}))
	if err != nil || !cleaned.Load() {
		t.Fatalf("worker did not unwind before return: err=%v, cleaned=%v", err, cleaned.Load())
	}
}

func TestRunExitAndCanceledParent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := Run(ctx, "test", io.Discard, func(ctx context.Context, ui *UI) error {
		_, err := ui.Select("菜单", []string{"退出"})
		return err
	}, tea.WithWindowSize(110, 32), tea.WithInput(nil), tea.WithoutRenderer(), tea.WithoutSignalHandler(), tea.WithFilter(func(state tea.Model, msg tea.Msg) tea.Msg {
		if _, ok := msg.(promptMsg); ok {
			go state.(*model).ui.send(key(tea.KeyEnter))
		}
		return msg
	}))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	err = Run(ctx, "test", io.Discard, func(ctx context.Context, ui *UI) error { _, err := ui.Password("密码"); return err }, tea.WithInput(nil), tea.WithoutRenderer(), tea.WithoutSignalHandler())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("parent cancellation=%v", err)
	}
}
