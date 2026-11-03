package tui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Keep footer ranges in the plain history so device output stays sanitized and
// divider lines can be styled and resized separately from command results.
type resultDivider struct {
	start, end int
	elapsed    time.Duration
}

func (m *model) finishCommand() {
	if m.commandStarted.IsZero() {
		return
	}
	elapsed := time.Since(m.commandStarted)
	hasOutput := m.commandOutput
	m.commandStarted = time.Time{}
	m.commandOutput = false
	if !hasOutput {
		return
	}
	padding := "\n"
	if !strings.HasSuffix(m.history, "\n") {
		padding += "\n"
	}
	caption := "◇ 耗时 " + formatElapsed(elapsed)
	m.appendOutput(padding + caption + "\n\n")
	end := len(m.history) - 2
	m.dividers = append(m.dividers, resultDivider{start: end - len(caption), end: end, elapsed: elapsed})
	m.refreshResults()
}

func formatElapsed(elapsed time.Duration) string {
	if elapsed < time.Millisecond {
		return "<1ms"
	}
	if elapsed < time.Second {
		return elapsed.Round(time.Millisecond).String()
	}
	return elapsed.Round(100 * time.Millisecond).String()
}

func (m *model) dividerView(elapsed time.Duration) string {
	width := max(1, m.results.Width())
	caption := "◇ 耗时 " + formatElapsed(elapsed)
	// Crush's assistant metadata uses subtle text followed by a separator.
	text := lipgloss.NewStyle().Foreground(lipgloss.Color("#858392"))
	line := lipgloss.NewStyle().Foreground(lipgloss.Color("#3A3943"))
	remaining := width - lipgloss.Width(caption) - 1
	if remaining < 1 {
		return text.Render(ansi.Truncate(caption, width, "…"))
	}
	return text.Render(caption) + " " + line.Render(strings.Repeat("─", remaining))
}

func (m *model) refreshResults() {
	var content strings.Builder
	start := 0
	for _, divider := range m.dividers {
		content.WriteString(m.history[start:divider.start])
		content.WriteString(m.dividerView(divider.elapsed))
		start = divider.end
	}
	content.WriteString(m.history[start:])
	m.results.SetContent(content.String())
}
