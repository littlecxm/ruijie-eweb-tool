// Package tui hosts the command workflows in one responsive Bubble Tea program.
package tui

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
)

var ErrCanceled = errors.New("操作已取消")

type promptKind uint8

const (
	selectPrompt promptKind = iota
	textPrompt
	passwordPrompt
)

type answer struct {
	value string
	err   error
}

type promptMsg struct {
	kind    promptKind
	label   string
	options []string
	value   string
	reply   chan answer
}

type outputMsg string
type finishedMsg struct{ err error }

// UI implements the workflows' prompts and io.Writer without writing directly
// to the terminal. Only the Bubble Tea event loop owns the visible state.
type UI struct {
	ctx         context.Context
	cancel      context.CancelFunc
	send        func(tea.Msg)
	interrupted atomic.Bool
}

func (u *UI) Select(label string, options []string) (string, error) {
	if len(options) == 0 {
		return "", errors.New("菜单没有可选项")
	}
	return u.ask(promptMsg{kind: selectPrompt, label: label, options: slices.Clone(options)})
}

func (u *UI) Input(label, value string) (string, error) {
	return u.ask(promptMsg{kind: textPrompt, label: label, value: value})
}

func (u *UI) Password(label string) (string, error) {
	return u.ask(promptMsg{kind: passwordPrompt, label: label})
}

func (u *UI) ask(request promptMsg) (string, error) {
	if err := u.ctx.Err(); err != nil {
		return "", err
	}
	request.reply = make(chan answer, 1)
	u.send(request)
	select {
	case response := <-request.reply:
		return response.value, response.err
	case <-u.ctx.Done():
		return "", u.ctx.Err()
	}
}

func (u *UI) Write(data []byte) (int, error) {
	if err := u.ctx.Err(); err != nil {
		return 0, err
	}
	u.send(outputMsg(string(data)))
	return len(data), nil
}

// Run keeps network and file work off the event loop. Cancellation waits for
// the workflow to unwind, so no output file continues changing after exit.
func Run(ctx context.Context, title string, out io.Writer, work func(context.Context, *UI) error, options ...tea.ProgramOption) error {
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ui := &UI{ctx: taskCtx, cancel: cancel}
	m := newModel(title, ui)
	opts := []tea.ProgramOption{tea.WithContext(taskCtx), tea.WithOutput(out)}
	opts = append(opts, options...)
	p := tea.NewProgram(m, opts...)
	ui.send = p.Send
	done := make(chan struct{})
	var workErr error
	go func() {
		defer close(done)
		workErr = work(taskCtx, ui)
		ui.send(finishedMsg{workErr})
	}()
	_, runErr := p.Run()
	cancel()
	<-done
	if ui.interrupted.Load() || errors.Is(workErr, ErrCanceled) {
		return nil
	}
	if runErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return runErr
	}
	return workErr
}
