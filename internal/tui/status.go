package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type statusStyles struct {
	Help                             lipgloss.Style
	SuccessIndicator, SuccessMessage lipgloss.Style
	InfoIndicator, InfoMessage       lipgloss.Style
	UpdateIndicator, UpdateMessage   lipgloss.Style
	WarnIndicator, WarnMessage       lipgloss.Style
	ErrorIndicator, ErrorMessage     lipgloss.Style
}

// Match Crush's s.Status badge/message pair and default Charmtone palette:
// https://github.com/charmbracelet/crush/blob/3b1f8604f0027cf5fa47f29a27655ef1aeed8d8f/internal/ui/styles/quickstyle.go#L1235-L1256
var statusTheme = func() statusStyles {
	char := lipgloss.Color("#3A3943")
	iron := lipgloss.Color("#4D4C57")
	pepper := lipgloss.Color("#201F26")
	indicator := lipgloss.NewStyle().Foreground(char).Background(lipgloss.Color("#00FFB2")).Padding(0, 1).Bold(true)
	message := lipgloss.NewStyle().Foreground(char).Background(lipgloss.Color("#12C78F")).Padding(0, 1)
	return statusStyles{
		Help:             lipgloss.NewStyle().Foreground(muted).Padding(0, 1),
		SuccessIndicator: indicator, SuccessMessage: message,
		InfoIndicator: indicator, InfoMessage: message,
		UpdateIndicator: indicator, UpdateMessage: message,
		WarnIndicator:  indicator.Foreground(iron).Background(lipgloss.Color("#F5EF34")),
		WarnMessage:    message.Foreground(iron).Background(lipgloss.Color("#E8FE96")),
		ErrorIndicator: indicator.Foreground(pepper).Background(lipgloss.Color("#FF577D")),
		ErrorMessage:   message.Foreground(lipgloss.Color("#FFFAF1")).Background(lipgloss.Color("#EB4268")),
	}
}()

func (m *model) statusView() string {
	indicator, message := statusTheme.InfoIndicator, statusTheme.InfoMessage
	label, text := "就绪", "选择操作；Tab 查看结果，Ctrl+L 清空历史"
	switch {
	case m.err != nil:
		indicator, message = statusTheme.ErrorIndicator, statusTheme.ErrorMessage
		label, text = "错误", m.err.Error()
	case m.done:
		indicator, message = statusTheme.SuccessIndicator, statusTheme.SuccessMessage
		label, text = "完成", "操作已结束，Enter / q 退出"
	case m.confirming():
		indicator, message = statusTheme.WarnIndicator, statusTheme.WarnMessage
		label, text = "确认", m.request.label
	case m.request == nil:
		indicator, message = statusTheme.UpdateIndicator, statusTheme.UpdateMessage
		label, text = safeText(m.spinner.View())+" 处理中", "正在处理，请稍候；Ctrl+C 退出"
	case m.request.kind == passwordPrompt:
		text = "输入设备密码；输入内容不会记录到结果历史"
	case m.request.kind == textPrompt:
		text = m.request.label + "；Enter 提交，Esc 取消"
	}
	badge := indicator.Render(label)
	remaining := m.width - lipgloss.Width(badge)
	if remaining <= message.GetHorizontalPadding() {
		return ansi.Truncate(badge, m.width, "")
	}
	available := remaining - message.GetHorizontalPadding()
	text = strings.Join(strings.Fields(safeText(text)), " ")
	text = ansi.Truncate(text, available, "…")
	text += strings.Repeat(" ", max(0, available-lipgloss.Width(text)))
	return badge + message.Render(text)
}
