package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/client"
)

const defaultEndpoint = "https://192.168.110.1"

type unlockWork func(context.Context, string, string, func(string)) (*unlockResult, error)
type progressMsg string
type finishedMsg struct {
	result *unlockResult
	err    error
}

type unlockModel struct {
	ctx       context.Context
	cancel    context.CancelFunc
	work      unlockWork
	send      func(tea.Msg)
	workDone  chan struct{}
	inputs    [2]textinput.Model
	focus     int
	spinner   spinner.Model
	status    string
	completed []string
	running   bool
	done      bool
	formError error
	err       error
	result    *unlockResult
}

func newUnlockModel(ctx context.Context, cancel context.CancelFunc, work unlockWork) *unlockModel {
	m := &unlockModel{
		ctx: ctx, cancel: cancel, work: work,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(keywordStyle)),
	}
	for i := range m.inputs {
		m.inputs[i] = textinput.New()
		m.inputs[i].Prompt = "› "
		m.inputs[i].SetVirtualCursor(true)
		m.inputs[i].SetWidth(60)
	}
	m.inputs[0].SetValue(defaultEndpoint)
	m.inputs[1].EchoMode = textinput.EchoPassword
	m.inputs[1].EchoCharacter = '•'
	return m
}

func (m *unlockModel) Init() tea.Cmd { return m.inputs[0].Focus() }

func (m *unlockModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			for i := range m.inputs {
				m.inputs[i].SetWidth(max(1, min(70, msg.Width-4)))
			}
		}
		return m, nil
	case progressMsg:
		if !m.done && string(msg) != m.status {
			if m.status != "" {
				m.completed = append(m.completed, m.status)
			}
			m.status = string(msg)
		}
		return m, nil
	case finishedMsg:
		if m.done && errors.Is(m.err, context.Canceled) {
			return m, tea.Quit
		}
		m.running, m.done = false, true
		m.result, m.err = msg.result, msg.err
		if msg.err == nil && m.status != "" {
			m.completed = append(m.completed, m.status)
			m.status = ""
		}
		return m, tea.Quit
	case spinner.TickMsg:
		if m.running {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.inputs[1].Reset()
			m.cancel()
			m.running, m.done, m.err = false, true, context.Canceled
			return m, tea.Quit
		}
		if m.running || m.done {
			return m, nil
		}
		switch msg.String() {
		case "tab", "shift+tab", "up", "down":
			return m, m.focusInput(1 - m.focus)
		case "enter":
			if m.formError = validateEndpoint(m.inputs[0].Value()); m.formError != nil {
				return m, m.focusInput(0)
			}
			if m.focus == 0 {
				return m, m.focusInput(1)
			}
			password := m.inputs[1].Value()
			if password == "" || !utf8.ValidString(password) || len(utf16.Encode([]rune(password))) > 64 {
				m.formError = errors.New("密码须为 1 至 64 个 UTF-16 字符单位")
				return m, nil
			}
			m.startWork(m.inputs[0].Value(), password)
			return m, m.spinner.Tick
		}
	}
	if !m.running && !m.done {
		var cmd tea.Cmd
		m.inputs[m.focus], cmd = m.inputs[m.focus].Update(message)
		return m, cmd
	}
	return m, nil
}

func (m *unlockModel) focusInput(index int) tea.Cmd {
	m.inputs[m.focus].Blur()
	m.focus = index
	return m.inputs[m.focus].Focus()
}

func (m *unlockModel) startWork(endpoint, password string) {
	m.running = true
	m.inputs[0].Blur()
	m.inputs[1].Reset()
	m.inputs[1].Blur()
	m.status = "正在登录并获取设备信息…"
	m.workDone = make(chan struct{})
	// Start the worker before returning from Update so runUnlockUI can always
	// wait for cleanup, even if cancellation arrives before the next command.
	go func() {
		result, err := m.work(m.ctx, endpoint, password, func(status string) {
			m.send(progressMsg(status))
		})
		close(m.workDone)
		m.send(finishedMsg{result: result, err: err})
	}()
}

func (m *unlockModel) View() tea.View {
	var view strings.Builder
	if m.running || m.done {
		for _, status := range m.completed {
			fmt.Fprintln(&view, "✓ "+status)
		}
		if m.running {
			fmt.Fprintln(&view, m.spinner.View()+" "+m.status)
			fmt.Fprintln(&view, "Ctrl+C 取消")
		} else if m.status != "" && !errors.Is(m.err, context.Canceled) {
			fmt.Fprintln(&view, "✗ "+m.status)
		}
	} else {
		fmt.Fprint(&view, "eweb-unlock\n\n")
		for i, label := range []string{"设备网址", "设备密码（admin）"} {
			if i == m.focus {
				label = keywordStyle.Render(label)
			}
			fmt.Fprintf(&view, "%s\n%s\n\n", label, m.inputs[i].View())
		}
		if m.formError != nil {
			fmt.Fprintln(&view, m.formError)
		}
		fmt.Fprintln(&view, "Tab 切换 · Enter 继续 / 解锁 · Esc / Ctrl+C 退出")
	}
	return tea.NewView(view.String())
}

func validateEndpoint(value string) error {
	c, err := client.New(value)
	if err == nil {
		c.CloseIdleConnections()
	}
	return err
}

func runUnlockUI(ctx context.Context, in io.Reader, out io.Writer, work unlockWork, options ...tea.ProgramOption) error {
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	m := newUnlockModel(taskCtx, cancel, work)
	opts := []tea.ProgramOption{tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out), tea.WithoutSignalHandler()}
	opts = append(opts, options...)
	p := tea.NewProgram(m, opts...)
	m.send = p.Send
	_, err := p.Run()
	cancel()
	if m.workDone != nil {
		<-m.workDone
	}
	m.inputs[1].Reset()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	if m.err != nil {
		return m.err
	}
	if m.result != nil {
		_, err = fmt.Fprint(out, renderUnlockResult(m.result))
	}
	return err
}
