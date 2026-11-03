package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func testModel(t *testing.T) *model {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	m := newModel("eweb-tool", &UI{ctx: ctx, cancel: cancel})
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 32})
	return m
}

func key(code rune) tea.KeyPressMsg       { return tea.KeyPressMsg{Code: code} }
func textKey(text string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: []rune(text)[0], Text: text} }

func installPrompt(m *model, kind promptKind, label string, options ...string) chan answer {
	reply := make(chan answer, 1)
	m.Update(promptMsg{kind: kind, label: label, options: options, reply: reply})
	return reply
}

func TestMenuFilterNavigationAndEscape(t *testing.T) {
	m := testModel(t)
	reply := installPrompt(m, selectPrompt, "选择操作", "列出在线终端", "设备概览", "系统状态", "退出")
	m.Update(textKey("/"))
	m.Update(tea.PasteMsg{Content: "系统"})
	if len(m.matches) != 1 || m.request.options[m.matches[0]] != "系统状态" {
		t.Fatalf("filtered options=%v", m.matches)
	}
	m.Update(key(tea.KeyEnter))
	if got := <-reply; got.value != "系统状态" || got.err != nil {
		t.Fatalf("answer=%+v", got)
	}
	if m.request != nil {
		t.Fatal("duplicate submit remains possible")
	}
	m.Update(key(tea.KeyEnter))
	if len(reply) != 0 {
		t.Fatal("submitted twice")
	}

	reply = installPrompt(m, selectPrompt, "系统状态", "当前版本", "设备时间", "返回上级菜单")
	m.Update(textKey("/"))
	m.Update(tea.PasteMsg{Content: "不存在"})
	m.Update(key(tea.KeyEnter))
	if len(reply) != 0 {
		t.Fatal("empty filter activated an option")
	}
	m.Update(key(tea.KeyEscape))
	if len(m.matches) != 3 || m.searching {
		t.Fatal("escape did not clear search")
	}
	m.Update(key(tea.KeyEscape))
	if got := <-reply; got.value != "返回上级菜单" {
		t.Fatalf("back=%+v", got)
	}
}

func TestPasswordMaskAndInputCancellation(t *testing.T) {
	m := testModel(t)
	reply := installPrompt(m, passwordPrompt, "输入设备密码")
	secret := "密钥🙂-q-secret"
	m.Update(tea.PasteMsg{Content: secret})
	view := ansi.Strip(m.View().Content)
	if strings.Contains(view, secret) || !strings.Contains(view, "••") || m.input.Value() != secret {
		t.Fatalf("password not masked: %s", view)
	}
	m.Update(key(tea.KeyEnter))
	if got := <-reply; got.value != secret {
		t.Fatalf("password changed: %+v", got)
	}
	if strings.Contains(m.history, secret) || m.input.Value() != "" {
		t.Fatal("password retained in UI history or input")
	}
	reply = installPrompt(m, textPrompt, "设备网址")
	m.Update(textKey("q"))
	if m.ui.interrupted.Load() || m.input.Value() != "q" {
		t.Fatal("q in text input quit the program")
	}
	m.Update(key(tea.KeyEscape))
	if got := <-reply; got.err != ErrCanceled {
		t.Fatalf("input cancel=%+v", got)
	}
}

func TestConfirmationDefaultsToCancelAndCannotBeFiltered(t *testing.T) {
	m := testModel(t)
	reply := installPrompt(m, selectPrompt, "设备 https://192.168.110.1：将清空设备配置并可能重启，是否继续？", "取消", "确认执行")
	m.Update(textKey("/"))
	if m.searching {
		t.Fatal("confirmation can be filtered to destructive option")
	}
	m.Update(key(tea.KeyEnter))
	if got := <-reply; got.value != "取消" {
		t.Fatalf("default confirmation=%+v", got)
	}
	reply = installPrompt(m, selectPrompt, "是否继续？", "取消", "确认执行")
	m.Update(key(tea.KeyDown))
	m.Update(key(tea.KeyEnter))
	if got := <-reply; got.value != "确认执行" {
		t.Fatalf("explicit confirmation=%+v", got)
	}
}

func TestResponsiveLayoutFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{20, 8}, {40, 18}, {60, 24}, {80, 30}, {104, 22}, {120, 36}, {180, 50}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := testModel(t)
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			for _, confirm := range []bool{false, true} {
				options := []string{"列出在线终端", "设备概览", "系统状态", "系统设置", "备份与恢复", "固件升级", "设备维护", "退出"}
				label := "选择操作"
				if confirm {
					options = []string{"取消", "确认执行"}
					label = "设备 https://192.168.110.1：将清空设备配置并可能重启，管理地址可能变化，是否继续？"
				}
				installPrompt(m, selectPrompt, label, options...)
				l := m.layout()
				if l.navW > 0 {
					pane := m.pane("操作", m.navigation(l.navW-4, l.navH-4), l.navW, l.navH, true)
					if size[0] >= 40 && size[1] >= 18 && (lipgloss.Width(pane) != l.navW || lipgloss.Height(pane) != l.navH) {
						t.Fatalf("pane dimensions=%dx%d, want %dx%d", lipgloss.Width(pane), lipgloss.Height(pane), l.navW, l.navH)
					}
				}
				view := m.View()
				if !view.AltScreen || view.MouseMode != tea.MouseModeCellMotion {
					t.Fatal("missing full-screen mode")
				}
				if lipgloss.Height(view.Content) > size[1] {
					t.Errorf("view exceeds height: %d", lipgloss.Height(view.Content))
				}
				for _, line := range strings.Split(view.Content, "\n") {
					if lipgloss.Width(line) > size[0] {
						t.Errorf("line exceeds width %d: %s", size[0], line)
					}
				}
				if size[0] >= 40 && size[1] >= 18 && confirm {
					plain := ansi.Strip(view.Content)
					if !strings.Contains(plain, "取消") || !strings.Contains(plain, "确认执行") {
						t.Fatalf("confirmation options hidden: %s", plain)
					}
				}
			}
		})
	}
}

func TestShortTerminalKeepsInputAndResultsAccessible(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 18})
	installPrompt(m, textPrompt, "设备网址")
	m.Update(tea.PasteMsg{Content: "https://device"})
	if !strings.Contains(m.View().Content, "https://device") {
		t.Fatal("input hidden in short terminal")
	}
	m.appendOutput("查询结果")
	m.Update(key(tea.KeyTab))
	if !strings.Contains(m.View().Content, "查询结果") {
		t.Fatal("results inaccessible in short terminal")
	}
	m.Update(key(tea.KeyTab))
	if !strings.Contains(m.View().Content, "https://device") {
		t.Fatal("Tab did not restore input")
	}
}

func TestResultScrollingAndBoundedSafeHistory(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	installPrompt(m, selectPrompt, "选择操作", "设备概览", "退出")
	m.appendOutput(strings.Repeat(strings.Repeat("终端", 100)+"\n", 100))
	m.results.GotoTop()
	m.Update(key(tea.KeyTab))
	m.Update(key(tea.KeyPgDown))
	m.Update(key(tea.KeyRight))
	if m.results.YOffset() == 0 || m.results.XOffset() == 0 {
		t.Fatal("results cannot scroll in both directions")
	}
	m.Update(key(tea.KeyTab))
	if m.resultsFocused {
		t.Fatal("Tab did not return focus to menu")
	}
	m.appendOutput("\x1b]52;c;ZXZpbA==\a\x1b[2J\r数据\n")
	if strings.ContainsAny(m.history, "\x1b\a\r") || !strings.Contains(m.history, "数据") {
		t.Fatal("unsafe terminal sequences retained")
	}
	m.appendOutput(strings.Repeat("汉字🙂", historyLimit))
	if len(m.history) > historyLimit || !utf8.ValidString(m.history) {
		t.Fatal("history limit broke Unicode or memory bound")
	}
}
