package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	accent     = lipgloss.Color("#7DD3FC")
	muted      = lipgloss.Color("#94A3B8")
	border     = lipgloss.Color("#334155")
	danger     = lipgloss.Color("#FDA4AF")
	background = lipgloss.Color("#0B1018")
	foreground = lipgloss.Color("#E2E8F0")
)

type layout struct {
	navW, navH, resultW, resultH int
	sideBySide                   bool
}

func (m *model) layout() layout {
	bodyH := max(8, m.height-6)
	if m.confirming() || m.height < 22 {
		if m.resultsFocused && !m.confirming() {
			return layout{resultW: m.width, resultH: bodyH}
		}
		return layout{navW: m.width, navH: bodyH}
	}
	if m.width >= 104 && m.height >= 22 {
		navW := min(42, m.width/3)
		return layout{navW, bodyH, m.width - navW - 1, bodyH, true}
	}
	navH := min(11, max(6, bodyH/2))
	return layout{m.width, navH, m.width, max(5, bodyH-navH-1), false}
}

func (m *model) resize() {
	l := m.layout()
	m.help.SetWidth(max(1, m.width-statusTheme.Help.GetHorizontalPadding()))
	m.results.SetWidth(max(1, l.resultW-4))
	m.results.SetHeight(max(1, l.resultH-4))
	m.refreshResults()
	inputWidth := l.navW
	if inputWidth == 0 {
		inputWidth = m.width
	}
	m.input.SetWidth(max(1, inputWidth-7))
	m.filter.SetWidth(max(1, inputWidth-7))
}

func (m *model) View() tea.View {
	var content string
	if m.width < 40 || m.height < 18 {
		content = "请扩大终端至至少 40×18\n" + m.help.ShortHelpView([]key.Binding{m.keys.Interrupt})
	} else {
		header := lipgloss.NewStyle().Bold(true).Foreground(accent).Render(clip(m.title, m.width)) + "\n"
		// header += "\n" + lipgloss.NewStyle().Foreground(muted).Render(clip(m.subtitle, m.width)) + "\n\n"
		l := m.layout()
		nav := ""
		if l.navW > 0 {
			nav = m.pane("操作", m.navigation(max(1, l.navW-4), max(1, l.navH-4)), l.navW, l.navH, !m.resultsFocused)
		}
		body := nav
		if l.resultW > 0 {
			resultTitle := fmt.Sprintf("%.0f%%", m.results.ScrollPercent()*100)
			result := m.results.View()
			if m.history == "" {
				result = lipgloss.NewStyle().Foreground(muted).Render("操作结果会显示在这里。\nTab 切换到结果，方向键滚动。")
			}
			results := m.pane(resultTitle, result, l.resultW, l.resultH, m.resultsFocused)
			if l.navW == 0 {
				body = results
			} else if l.sideBySide {
				body = lipgloss.JoinHorizontal(lipgloss.Top, nav, " ", results)
			} else {
				body = nav + "\n" + results
			}
		}
		footer := "\n" + statusTheme.Help.Width(m.width).Render(m.help.ShortHelpView(m.helpBindings())) + "\n" + m.statusView()
		content = header + body + footer
	}
	v := tea.NewView(fit(content, m.width, m.height))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = m.title
	v.BackgroundColor = background
	v.ForegroundColor = foreground
	return v
}

func (m *model) pane(title, content string, width, height int, focused bool) string {
	color := border
	if focused {
		color = accent
	}
	if m.confirming() {
		color = danger
	}
	innerW := max(1, width-4)
	heading := lipgloss.NewStyle().Bold(true).Foreground(color).Render(clip(title, innerW))
	inside := heading + "\n\n" + fit(content, innerW, max(1, height-4))
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(color).Padding(0, 1).Width(width).Height(height).Render(inside)
}

func (m *model) navigation(width, height int) string {
	if m.done {
		text := "操作已结束。\n\n请查看结果，按 Enter 或 q 退出。"
		if m.err != nil {
			text = "操作失败。\n\n详细原因见结果区域。"
		}
		return text
	}
	if m.request == nil {
		return m.spinner.View() + " 正在处理，请稍候…"
	}
	label := lipgloss.NewStyle().Width(width).Bold(true).Render(safeText(m.request.label))
	// Reserve visible space for options and navigation even for long prompts.
	label = fit(label, width, min(max(1, height-4), lipgloss.Height(label)))
	if m.request.kind != selectPrompt {
		hint := "支持粘贴；可用 Ctrl+U 清空。"
		if m.request.kind == passwordPrompt {
			hint = "密码仅用于登录，不显示在结果中。"
		}
		return label + "\n\n" + m.input.View() + "\n\n" + lipgloss.NewStyle().Foreground(muted).Render(clip(hint, width))
	}
	lines := []string{label, ""}
	if m.searching || m.filter.Value() != "" {
		lines = append(lines, m.filter.View())
	}
	available := max(1, height-lipgloss.Height(strings.Join(lines, "\n"))-1)
	if len(m.matches) == 0 {
		return strings.Join(lines, "\n") + "\n没有匹配的菜单项"
	}
	start := max(0, m.selected-available+1)
	end := min(len(m.matches), start+available)
	for i := start; i < end; i++ {
		text := "  " + clip(safeText(m.request.options[m.matches[i]]), max(1, width-2))
		if i == m.selected {
			text = "› " + clip(safeText(m.request.options[m.matches[i]]), max(1, width-2))
			text = lipgloss.NewStyle().Foreground(accent).Background(lipgloss.Color("#14364B")).Bold(true).Width(width).Render(text)
		}
		lines = append(lines, text)
	}
	if len(m.matches) > available {
		lines = append(lines, lipgloss.NewStyle().Foreground(muted).Render(fmt.Sprintf("%d / %d", m.selected+1, len(m.matches))))
	}
	return strings.Join(lines, "\n")
}

func clip(text string, width int) string { return ansi.Truncate(text, max(1, width), "…") }

func fit(text string, width, height int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], max(1, width), "")
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
