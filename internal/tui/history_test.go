package tui

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestPromptResponsesDoNotEchoToResults(t *testing.T) {
	m := testModel(t)
	m.appendOutput("已有结果\n")
	for _, tt := range []struct {
		kind    promptKind
		label   string
		options []string
	}{
		{selectPrompt, "选择操作", []string{"设备概览", "退出"}},
		{selectPrompt, "设备概览", []string{"当前版本", "返回上级菜单"}},
		{textPrompt, "设备网址", nil},
		{passwordPrompt, "设备密码", nil},
		{selectPrompt, "是否继续？", []string{"取消", "确认执行"}},
	} {
		reply := installPrompt(m, tt.kind, tt.label, tt.options...)
		if tt.kind != selectPrompt {
			m.Update(tea.PasteMsg{Content: "private-input"})
		}
		m.Update(pressKey(tea.KeyEnter))
		<-reply
	}
	installPrompt(m, selectPrompt, "选择操作", "设备概览", "退出")
	if m.history != "已有结果\n" || len(m.dividers) != 0 {
		t.Fatalf("prompts or navigation polluted command results: %q", m.history)
	}
}

func TestCommandCompletionAddsOneTimedDivider(t *testing.T) {
	for _, tt := range []struct {
		name string
		done bool
		err  error
	}{
		{name: "query returns to menu"},
		{name: "query failure returns to menu", err: errors.New("query failed")},
		{name: "workflow finishes", done: true},
		{name: "workflow fails", done: true, err: errors.New("command failed")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel(t)
			reply := installPrompt(m, selectPrompt, "选择操作", "当前版本", "退出")
			m.Update(pressKey(tea.KeyEnter))
			<-reply
			m.commandStarted = time.Now().Add(-1250 * time.Millisecond)
			m.Update(outputMsg("版本：2.430"))
			if tt.done {
				m.Update(finishedMsg{err: tt.err})
			} else {
				if tt.err != nil {
					m.Update(outputMsg("\n错误：" + tt.err.Error() + "\n"))
				}
				installPrompt(m, selectPrompt, "选择操作", "当前版本", "退出")
			}
			if len(m.dividers) != 1 || m.dividers[0].elapsed < 1250*time.Millisecond {
				t.Fatalf("completion did not record elapsed execution: %+v", m.dividers)
			}
			plain := ansi.Strip(m.results.View())
			footer := strings.Index(plain, "◇ 耗时 ")
			if footer <= strings.Index(plain, "版本：2.430") || !strings.Contains(plain[footer:], "─") {
				t.Fatalf("missing divider after command result: %s", plain)
			}
			if tt.err != nil && (strings.Index(plain, tt.err.Error()) < 0 || strings.Index(plain, tt.err.Error()) > footer) {
				t.Fatal("divider preceded command error")
			}
			installPrompt(m, selectPrompt, "选择操作", "当前版本", "退出")
			if strings.Count(m.history, "◇ 耗时 ") != 1 {
				t.Fatal("opening a menu duplicated the completion footer")
			}
		})
	}
}

func TestCommandTimeStartsAfterInput(t *testing.T) {
	m := testModel(t)
	reply := installPrompt(m, selectPrompt, "选择操作", "固件解密", "退出")
	m.Update(pressKey(tea.KeyEnter))
	<-reply
	m.commandStarted = time.Now().Add(-time.Hour)
	reply = installPrompt(m, textPrompt, "文件路径")
	if !m.commandStarted.IsZero() {
		t.Fatal("execution timer kept running while waiting for input")
	}
	m.Update(tea.PasteMsg{Content: "/tmp/firmware.bin"})
	m.Update(pressKey(tea.KeyEnter))
	<-reply
	m.Update(outputMsg("处理完成\n"))
	installPrompt(m, selectPrompt, "选择操作", "固件解密", "退出")
	if len(m.dividers) != 1 || m.dividers[0].elapsed >= time.Minute {
		t.Fatal("execution duration included time spent choosing or entering input")
	}
}

func TestDividersResizeAndRespectHistoryLimit(t *testing.T) {
	m := testModel(t)
	m.Update(outputMsg("查询结果\n"))
	installPrompt(m, selectPrompt, "选择操作", "当前版本", "退出")
	for _, width := range []int{40, 60, 110, 180} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 32})
		plain := ansi.Strip(m.results.View())
		found := false
		for _, line := range strings.Split(plain, "\n") {
			if strings.Contains(line, "◇ 耗时 ") {
				found = true
				if lipgloss.Width(line) != m.results.Width() || !strings.HasSuffix(line, "─") {
					t.Fatalf("divider did not fill resized viewport: %q", line)
				}
			}
		}
		if !found {
			t.Fatalf("divider disappeared after resize to %d columns", width)
		}
	}
	m.appendOutput(strings.Repeat("汉字🙂\n", historyLimit/4))
	if len(m.history) > historyLimit || !utf8.ValidString(m.history) || len(m.dividers) != 0 {
		t.Fatal("history limit did not evict old divider metadata safely")
	}
	m.Update(outputMsg("新结果\n"))
	installPrompt(m, selectPrompt, "选择操作", "当前版本", "退出")
	if len(m.dividers) != 1 || !strings.Contains(ansi.Strip(m.results.View()), "◇ 耗时 ") {
		t.Fatal("new divider was not rendered after history eviction")
	}
	m.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	installPrompt(m, selectPrompt, "选择操作", "当前版本", "退出")
	if m.history != "" || len(m.dividers) != 0 || strings.Contains(ansi.Strip(m.results.View()), "耗时") {
		t.Fatal("clearing history left command dividers behind")
	}
}
