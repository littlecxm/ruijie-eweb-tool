package tui

import (
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const historyLimit = 256 << 10

type model struct {
	ui              *UI
	title, subtitle string
	width, height   int
	request         *promptMsg
	input, filter   textinput.Model
	searching       bool
	selected        int
	matches         []int
	resultsFocused  bool
	results         viewport.Model
	spinner         spinner.Model
	history         string
	lastChoice      string
	done            bool
	err             error
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
	m := &model{
		ui: ui, title: title, width: 110, height: 16,
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
	key := msg.String()
	if key == "ctrl+c" {
		return m.stop()
	}
	if m.done && (key == "enter" || key == "esc" || key == "q") {
		return m, tea.Quit
	}
	if key == "ctrl+l" {
		m.history = ""
		m.results.SetContent("")
		m.results.SetXOffset(0)
		return m, nil
	}
	if key == "q" && !m.editing() {
		return m.stop()
	}
	if m.width < 40 || m.height < 18 {
		return m, nil
	}
	if key == "tab" || key == "shift+tab" {
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
	if key == "pgup" || key == "pgdown" || m.resultsFocused || m.done {
		if key == "esc" || key == "enter" {
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
		switch key {
		case "enter":
			m.respond(answer{value: m.input.Value()})
			return m, nil
		case "esc":
			m.respond(answer{err: ErrCanceled})
			return m, nil
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}
	if m.searching && key != "up" && key != "down" && key != "enter" {
		if key == "esc" {
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
	switch key {
	case "up", "k":
		m.selected = max(0, m.selected-1)
	case "down", "j":
		m.selected = min(max(0, len(m.matches)-1), m.selected+1)
	case "home":
		m.selected = 0
	case "end":
		m.selected = max(0, len(m.matches)-1)
	case "/":
		if !m.confirming() {
			m.searching = true
			return m, m.filter.Focus()
		}
	case "enter":
		if len(m.matches) > 0 {
			choice := m.request.options[m.matches[m.selected]]
			m.lastChoice = choice
			m.appendOutput("\n› " + m.request.label + " · " + choice + "\n")
			m.respond(answer{value: choice})
		}
	case "esc", "left":
		for _, back := range []string{"取消", "返回上级菜单", "退出"} {
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
	m.history += safeText(text)
	if len(m.history) > historyLimit {
		start := len(m.history) - historyLimit
		if next := strings.IndexByte(m.history[start:], '\n'); next >= 0 {
			m.history = m.history[start+next+1:]
		} else {
			for start < len(m.history) && !utf8.RuneStart(m.history[start]) {
				start++
			}
			m.history = m.history[start:]
		}
	}
	m.results.SetContent(m.history)
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
