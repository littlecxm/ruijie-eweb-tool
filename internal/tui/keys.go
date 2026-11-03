package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Up, Down, First, Last    key.Binding
	Accept, Cancel, Back     key.Binding
	Search, FilterMove, Page key.Binding
	Focus, ClearHistory      key.Binding
	Quit, Interrupt, Close   key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "上移"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "下移"),
		),
		First: key.NewBinding(
			key.WithKeys("home"),
			key.WithHelp("home", "首项"),
		),
		Last: key.NewBinding(
			key.WithKeys("end"),
			key.WithHelp("end", "末项"),
		),
		Accept: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "确认"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "取消"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc", "left"),
			key.WithHelp("esc/←", "返回"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "搜索"),
		),
		FilterMove: key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "选择"),
		),
		Page: key.NewBinding(
			key.WithKeys("pgup", "pgdown"),
			key.WithHelp("pgup/pgdn", "翻页"),
		),
		Focus: key.NewBinding(
			key.WithKeys("tab", "shift+tab"),
			key.WithHelp("tab", "切换焦点"),
		),
		ClearHistory: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "清空历史"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", "退出"),
		),
		Interrupt: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "退出"),
		),
		Close: key.NewBinding(
			key.WithKeys("enter", "esc", "q"),
			key.WithHelp("enter/esc/q", "退出"),
		),
	}
}

// Group related bindings for compact help while retaining their actual keys.
func helpGroup(display, description string, bindings ...key.Binding) key.Binding {
	var keys []string
	for _, binding := range bindings {
		if binding.Enabled() {
			keys = append(keys, binding.Keys()...)
		}
	}
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(display, description))
}

func describeKey(binding key.Binding, description string) key.Binding {
	binding.SetHelp(binding.Help().Key, description)
	return binding
}

func (m *model) helpBindings() []key.Binding {
	k := m.keys
	switch {
	case m.done, m.resultsFocused:
		v := m.results.KeyMap
		bindings := []key.Binding{
			helpGroup("↑↓/jk", "滚动", v.Up, v.Down),
			helpGroup("←→/hl", "横移", v.Left, v.Right),
			helpGroup(k.Page.Help().Key, "翻页", v.PageUp, v.PageDown),
		}
		if m.done {
			return append(bindings, k.Close)
		}
		return append(bindings, describeKey(k.Focus, "返回操作"), k.ClearHistory, k.Quit)
	case m.confirming():
		return []key.Binding{helpGroup("↑↓/jk", "选择", k.Up, k.Down), k.Accept, k.Cancel}
	case m.searching:
		return []key.Binding{k.FilterMove, k.Accept, describeKey(k.Cancel, "清除搜索"), k.Focus, k.Interrupt}
	case m.editing():
		clear := m.input.KeyMap.DeleteBeforeCursor
		clear.SetHelp("ctrl+u", "清空")
		return []key.Binding{describeKey(k.Accept, "提交"), k.Cancel, clear, describeKey(k.Focus, "查看结果"), k.Interrupt}
	case m.request == nil:
		return []key.Binding{describeKey(k.Focus, "查看结果"), k.ClearHistory, k.Quit, k.Interrupt}
	default:
		return []key.Binding{helpGroup("↑↓/jk", "选择", k.Up, k.Down), k.Accept, k.Search, k.Back, describeKey(k.Focus, "查看结果"), k.Quit}
	}
}
