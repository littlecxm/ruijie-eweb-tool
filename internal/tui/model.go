package tui

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const historyLimit = 256 << 10

type model struct {
	ui             *UI
	title          string
	width, height  int
	keys           keyMap
	help           help.Model
	request        *promptMsg
	input, filter  textinput.Model
	searching      bool
	selected       int
	matches        []int
	resultsFocused bool
	results        viewport.Model
	spinner        spinner.Model
	history        string
	dividers       []resultDivider
	commandStarted time.Time
	commandOutput  bool
	lastChoice     string
	done           bool
	err            error
}

func newModel(title string, ui *UI) *model {
	input := textinput.New()
	input.Prompt = "› "
	input.SetVirtualCursor(true)
	input.SetStyles(textinput.DefaultDarkStyles())
	filter := textinput.New()
	filter.Prompt = "/ "
	filter.Placeholder = "搜索菜单"
	filter.SetVirtualCursor(true)
	filter.SetStyles(textinput.DefaultDarkStyles())
	results := viewport.New()
	results.SoftWrap = false
	results.SetHorizontalStep(8)
	helpView := help.New()
	helpView.Styles.ShortKey = lipgloss.NewStyle().Foreground(accent).Bold(true)
	helpView.Styles.ShortDesc = lipgloss.NewStyle().Foreground(muted)
	helpView.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(border)
	helpView.Styles.Ellipsis = lipgloss.NewStyle().Foreground(muted)
	m := &model{
		ui: ui, title: title, width: 110, height: 16,
		keys: defaultKeyMap(), help: helpView,
		input: input, filter: filter, results: results,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(accent))),
	}
	m.resize()
	return m
}

func (m *model) Init() tea.Cmd { return m.spinner.Tick }

func (m *model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		// A zero-sized event means the terminal size is not available yet.
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		m.resize()
		return m, nil
	case promptMsg:
		m.finishCommand()
		m.request = &msg
		m.selected = 0
		m.resultsFocused = false
		m.searching = false
		m.filter.Reset()
		m.filter.Blur()
		m.input.Reset()
		m.input.EchoMode = textinput.EchoNormal
		if msg.kind == passwordPrompt {
			m.input.EchoMode = textinput.EchoPassword
			m.input.EchoCharacter = '•'
		}
		m.input.SetValue(msg.value)
		m.refilter()
		m.resize()
		if msg.kind != selectPrompt {
			return m, m.input.Focus()
		}
		m.input.Blur()
		return m, nil
	case outputMsg:
		if m.commandStarted.IsZero() {
			m.commandStarted = time.Now()
		}
		m.appendOutput(string(msg))
		return m, nil
	case finishedMsg:
		m.done, m.err, m.request = true, msg.err, nil
		m.input.Reset()
		m.input.Blur()
		m.resultsFocused = true
		if errors.Is(msg.err, ErrCanceled) || (msg.err == nil && m.lastChoice == "退出") {
			return m, tea.Quit
		}
		if msg.err != nil {
			m.appendOutput("\n错误：" + msg.err.Error() + "\n")
		}
		m.finishCommand()
		m.resize()
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseWheelMsg:
		m.results, _ = m.results.Update(msg)
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		var cmd tea.Cmd
		if m.request != nil && m.request.kind != selectPrompt && !m.resultsFocused {
			m.input, cmd = m.input.Update(msg)
		} else if m.searching && !m.resultsFocused {
			m.filter, cmd = m.filter.Update(msg)
			m.refilter()
		}
		return m, cmd
	}
}

func (m *model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Interrupt) {
		return m.stop()
	}
	if m.done && key.Matches(msg, m.keys.Close) {
		return m, tea.Quit
	}
	if key.Matches(msg, m.keys.ClearHistory) {
		m.history = ""
		m.dividers = nil
		m.commandOutput = false
		m.results.SetContent("")
		m.results.SetXOffset(0)
		return m, nil
	}
	if key.Matches(msg, m.keys.Quit) && !m.editing() {
		return m.stop()
	}
	if m.width < 40 || m.height < 18 {
		return m, nil
	}
	if key.Matches(msg, m.keys.Focus) {
		if m.confirming() {
			return m, nil
		}
		m.resultsFocused = !m.resultsFocused
		m.resize()
		if m.resultsFocused {
			m.input.Blur()
			m.filter.Blur()
		} else if m.request != nil && m.request.kind != selectPrompt {
			return m, m.input.Focus()
		} else if m.searching {
			return m, m.filter.Focus()
		}
		return m, nil
	}
	if key.Matches(msg, m.keys.Page) || m.resultsFocused || m.done {
		if key.Matches(msg, m.keys.Cancel, m.keys.Accept) {
			m.resultsFocused = false
			m.resize()
			if m.request != nil && m.request.kind != selectPrompt {
				return m, m.input.Focus()
			}
			if m.searching {
				return m, m.filter.Focus()
			}
			return m, nil
		}
		m.results, _ = m.results.Update(msg)
		return m, nil
	}
	if m.request == nil {
		return m, nil
	}
	if m.request.kind != selectPrompt {
		switch {
		case key.Matches(msg, m.keys.Accept):
			m.respond(answer{value: m.input.Value()})
			return m, nil
		case key.Matches(msg, m.keys.Cancel):
			m.respond(answer{err: ErrCanceled})
			return m, nil
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}
	if m.searching && !key.Matches(msg, m.keys.FilterMove, m.keys.Accept) {
		if key.Matches(msg, m.keys.Cancel) {
			m.searching = false
			m.filter.Reset()
			m.filter.Blur()
			m.refilter()
			return m, nil
		}
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.selected = 0
		m.refilter()
		return m, cmd
	}
	switch {
	case key.Matches(msg, m.keys.Up):
		m.selected = max(0, m.selected-1)
	case key.Matches(msg, m.keys.Down):
		m.selected = min(max(0, len(m.matches)-1), m.selected+1)
	case key.Matches(msg, m.keys.First):
		m.selected = 0
	case key.Matches(msg, m.keys.Last):
		m.selected = max(0, len(m.matches)-1)
	case key.Matches(msg, m.keys.Search):
		if !m.confirming() {
			m.searching = true
			return m, m.filter.Focus()
		}
	case key.Matches(msg, m.keys.Accept):
		if len(m.matches) > 0 {
			choice := m.request.options[m.matches[m.selected]]
			m.lastChoice = choice
			m.respond(answer{value: choice})
		}
	case key.Matches(msg, m.keys.Back):
		for _, back := range []string{"取消", "← 返回", "返回上级菜单", "退出"} {
			if slices.Contains(m.request.options, back) {
				m.lastChoice = back
				m.respond(answer{value: back})
				return m, nil
			}
		}
		m.respond(answer{err: ErrCanceled})
	}
	return m, nil
}

func (m *model) respond(response answer) {
	if response.err == nil {
		m.commandStarted = time.Now()
	}
	m.request.reply <- response
	m.request = nil
	m.input.Reset()
	m.input.Blur()
	m.filter.Blur()
	m.resize()
}

func (m *model) stop() (tea.Model, tea.Cmd) {
	m.ui.interrupted.Store(true)
	m.ui.cancel()
	return m, tea.Quit
}

func (m *model) editing() bool {
	return !m.resultsFocused && m.request != nil && (m.request.kind != selectPrompt || m.searching)
}

func (m *model) confirming() bool {
	return m.request != nil && len(m.request.options) == 2 && m.request.options[0] == "取消" && m.request.options[1] == "确认执行"
}

func (m *model) refilter() {
	m.matches = nil
	if m.request == nil {
		return
	}
	filter := strings.ToLower(m.filter.Value())
	for i, option := range m.request.options {
		if strings.Contains(strings.ToLower(option), filter) {
			m.matches = append(m.matches, i)
		}
	}
	m.selected = min(m.selected, max(0, len(m.matches)-1))
}

func (m *model) appendOutput(text string) {
	atBottom := m.results.AtBottom()
	text = safeText(text)
	if strings.TrimSpace(text) != "" && !m.commandStarted.IsZero() {
		m.commandOutput = true
	}
	m.history += text
	if len(m.history) > historyLimit {
		previousLen := len(m.history)
		start := len(m.history) - historyLimit
		if next := strings.IndexByte(m.history[start:], '\n'); next >= 0 {
			m.history = m.history[start+next+1:]
		} else {
			for start < len(m.history) && !utf8.RuneStart(m.history[start]) {
				start++
			}
			m.history = m.history[start:]
		}
		removed := previousLen - len(m.history)
		kept := m.dividers[:0]
		for _, divider := range m.dividers {
			if divider.start >= removed {
				divider.start -= removed
				divider.end -= removed
				kept = append(kept, divider)
			}
		}
		m.dividers = kept
	}
	m.refreshResults()
	if atBottom {
		m.results.GotoBottom()
	}
}

func safeText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, ansi.Strip(text))
}
