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

func pressKey(code rune) tea.KeyPressMsg  { return tea.KeyPressMsg{Code: code} }
func textKey(text string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: []rune(text)[0], Text: text} }

func installPrompt(m *model, kind promptKind, label string, options ...string) chan answer {
	reply := make(chan answer, 1)
	m.Update(promptMsg{kind: kind, label: label, options: options, reply: reply})
	return reply
}

func TestMenuFilterNavigationAndEscape(t *testing.T) {
	m := testModel(t)
	reply := installPrompt(m, selectPrompt, "选择操作", "列出在线终端", "设备概览", "系统设置", "退出")
	m.Update(textKey("/"))
	m.Update(tea.PasteMsg{Content: "系统"})
	if len(m.matches) != 1 || m.request.options[m.matches[0]] != "系统设置" {
		t.Fatalf("filtered options=%v", m.matches)
	}
	m.Update(pressKey(tea.KeyEnter))
	if got := <-reply; got.value != "系统设置" || got.err != nil {
		t.Fatalf("answer=%+v", got)
	}
	if m.request != nil {
		t.Fatal("duplicate submit remains possible")
	}
	m.Update(pressKey(tea.KeyEnter))
	if len(reply) != 0 {
		t.Fatal("submitted twice")
	}

	reply = installPrompt(m, selectPrompt, "系统设置", "当前版本", "设备时间", "← 返回")
	m.Update(textKey("/"))
	m.Update(tea.PasteMsg{Content: "不存在"})
	m.Update(pressKey(tea.KeyEnter))
	if len(reply) != 0 {
		t.Fatal("empty filter activated an option")
	}
	m.Update(pressKey(tea.KeyEscape))
	if len(m.matches) != 3 || m.searching {
		t.Fatal("escape did not clear search")
	}
	m.Update(pressKey(tea.KeyEscape))
	if got := <-reply; got.value != "← 返回" {
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
	m.Update(pressKey(tea.KeyEnter))
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
	m.Update(pressKey(tea.KeyEscape))
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
	m.Update(pressKey(tea.KeyEnter))
	if got := <-reply; got.value != "取消" {
		t.Fatalf("default confirmation=%+v", got)
	}
	reply = installPrompt(m, selectPrompt, "是否继续？", "取消", "确认执行")
	m.Update(pressKey(tea.KeyDown))
	m.Update(pressKey(tea.KeyEnter))
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
				options := []string{"在线终端", "设备概览", "系统设置", "退出"}
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
				if size[0] >= 40 && size[1] >= 18 {
					plain := ansi.Strip(view.Content)
					if strings.Contains(plain, m.title) {
						t.Fatal("program title still occupies a row")
					}
					lines := strings.Split(plain, "\n")
					if lines[len(lines)-1] != ansi.Strip(m.statusView()) || !strings.Contains(lines[len(lines)-2], ansi.Strip(m.help.ShortHelpView(m.helpBindings()))) {
						t.Fatalf("footer is not anchored to the bottom: %s", plain)
					}
					if confirm && (!strings.Contains(plain, "取消") || !strings.Contains(plain, "确认执行")) {
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
	m.Update(pressKey(tea.KeyTab))
	if !strings.Contains(m.View().Content, "查询结果") {
		t.Fatal("results inaccessible in short terminal")
	}
	m.Update(pressKey(tea.KeyTab))
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
	m.Update(pressKey(tea.KeyTab))
	m.Update(pressKey(tea.KeyPgDown))
	m.Update(pressKey(tea.KeyRight))
	if m.results.YOffset() == 0 || m.results.XOffset() == 0 {
		t.Fatal("results cannot scroll in both directions")
	}
	m.Update(pressKey(tea.KeyTab))
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

func TestResultsScrollbarFollowsViewport(t *testing.T) {
	m := testModel(t)
	installPrompt(m, selectPrompt, "选择操作", "系统设置", "退出")
	m.appendOutput(strings.Repeat("查询结果\n", 100))
	m.results.GotoTop()
	m.Update(pressKey(tea.KeyTab))
	thumbRows := func() []int {
		t.Helper()
		var rows []int
		for i, line := range strings.Split(ansi.Strip(m.resultsView()), "\n") {
			if strings.HasSuffix(line, "┃") {
				rows = append(rows, i)
			} else if !strings.HasSuffix(line, "│") {
				t.Fatalf("scrollbar missing from result row %d: %q", i, line)
			}
		}
		if len(rows) == 0 {
			t.Fatal("scrollbar has no thumb")
		}
		return rows
	}
	if rows := thumbRows(); rows[0] != 0 {
		t.Fatalf("thumb is not at the top: %v", rows)
	}
	m.Update(pressKey(tea.KeyPgDown))
	if rows := thumbRows(); rows[0] == 0 {
		t.Fatalf("thumb did not move after scrolling: %v", rows)
	}
	for !m.results.AtBottom() {
		m.Update(pressKey(tea.KeyPgDown))
	}
	if rows := thumbRows(); rows[len(rows)-1] != m.results.Height()-1 {
		t.Fatalf("thumb is not at the bottom: %v", rows)
	}
	if strings.Contains(ansi.Strip(m.View().Content), "%") {
		t.Fatal("scroll percentage still rendered")
	}
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 150})
	if strings.ContainsAny(ansi.Strip(m.resultsView()), "┃│") {
		t.Fatal("scrollbar rendered when all results fit")
	}
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 32})
	thumbRows()
	m.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	if strings.ContainsAny(ansi.Strip(m.resultsView()), "┃│") {
		t.Fatal("scrollbar remained after clearing results")
	}
}

func TestHelpReflectsAvailableActions(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(*model)
		want  []string
		omit  []string
	}{
		{
			name: "menu",
			setup: func(m *model) {
				installPrompt(m, selectPrompt, "选择操作", "设备概览", "退出")
			},
			want: []string{"↑↓/jk 选择", "enter 确认", "/ 搜索", "esc/← 返回", "tab 查看结果", "q 退出"},
			omit: []string{"清除搜索", "ctrl+u"},
		},
		{
			name: "search",
			setup: func(m *model) {
				installPrompt(m, selectPrompt, "选择操作", "设备概览", "退出")
				m.Update(textKey("/"))
			},
			want: []string{"↑↓ 选择", "enter 确认", "esc 清除搜索", "ctrl+c 退出"},
			omit: []string{"↑↓/jk", "q 退出", "/ 搜索"},
		},
		{
			name: "password",
			setup: func(m *model) {
				installPrompt(m, passwordPrompt, "输入设备密码")
			},
			want: []string{"enter 提交", "esc 取消", "ctrl+u 清空", "tab 查看结果", "ctrl+c 退出"},
			omit: []string{"q 退出", "/ 搜索"},
		},
		{
			name: "confirmation",
			setup: func(m *model) {
				installPrompt(m, selectPrompt, "是否继续？", "取消", "确认执行")
			},
			want: []string{"↑↓/jk 选择", "enter 确认", "esc 取消"},
			omit: []string{"tab", "/ 搜索"},
		},
		{
			name: "results",
			setup: func(m *model) {
				installPrompt(m, selectPrompt, "选择操作", "设备概览", "退出")
				m.Update(pressKey(tea.KeyTab))
			},
			want: []string{"↑↓/jk 滚动", "←→/hl 横移", "pgup/pgdn 翻页", "tab 返回操作"},
			omit: []string{"enter 确认", "/ 搜索"},
		},
		{
			name:  "busy",
			setup: func(*model) {},
			want:  []string{"tab 查看结果", "ctrl+l 清空历史", "q 退出"},
			omit:  []string{"enter 确认", "/ 搜索", "↑↓/jk 选择"},
		},
		{
			name: "done",
			setup: func(m *model) {
				m.Update(finishedMsg{})
			},
			want: []string{"↑↓/jk 滚动", "enter/esc/q 退出"},
			omit: []string{"tab", "/ 搜索"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel(t)
			tt.setup(m)
			help := ansi.Strip(m.help.ShortHelpView(m.helpBindings()))
			if !strings.Contains(help, " • ") || !strings.Contains(ansi.Strip(m.View().Content), help) {
				t.Fatalf("help was not rendered in the footer: %q", help)
			}
			for _, text := range tt.want {
				if !strings.Contains(help, text) {
					t.Errorf("missing %q from help %q", text, help)
				}
			}
			for _, text := range tt.omit {
				if strings.Contains(help, text) {
					t.Errorf("unavailable action %q in help %q", text, help)
				}
			}
		})
	}
}

func TestKeyBindingDrivesBothHelpAndInteraction(t *testing.T) {
	m := testModel(t)
	installPrompt(m, selectPrompt, "选择操作", "jkq", "other", "退出")
	m.keys.Search.SetKeys("ctrl+f")
	m.keys.Search.SetHelp("ctrl+f", "搜索")
	if !strings.Contains(ansi.Strip(m.View().Content), "ctrl+f 搜索") {
		t.Fatal("help did not use the updated binding")
	}
	m.Update(textKey("/"))
	if m.searching {
		t.Fatal("old search binding is still active")
	}
	m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	if !m.searching {
		t.Fatal("new search binding is not active")
	}
	for _, text := range []string{"j", "k", "q"} {
		m.Update(textKey(text))
	}
	if m.filter.Value() != "jkq" || len(m.matches) != 1 || m.ui.interrupted.Load() {
		t.Fatal("search consumed text as navigation or quit keys")
	}
}
