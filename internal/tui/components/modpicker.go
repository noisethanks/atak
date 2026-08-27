package components

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/noisethanks/atak/internal/tui/style"
)

// ModSelectedMsg is sent when the user confirms a mod selection.
type ModSelectedMsg struct{ Mod string }

// ModPickerCancelledMsg is sent when the user cancels with Esc.
type ModPickerCancelledMsg struct{}

type modItem struct{ name string }

func (i modItem) Title() string       { return i.name }
func (i modItem) Description() string { return "" }
func (i modItem) FilterValue() string { return i.name }

// ModPicker is a fuzzy-searchable mod list component.
type ModPicker struct {
	list list.Model
}

// NewModPicker creates a ModPicker populated with the given mod names.
func NewModPicker(mods []string, width, height int) ModPicker {
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(1)
	delegate.SetSpacing(0)
	delegate.ShowDescription = false
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("#E8A020"))

	items := make([]list.Item, len(mods))
	for i, name := range mods {
		items[i] = modItem{name: name}
	}

	l := list.New(items, delegate, width, height)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)

	return ModPicker{list: l}
}

func (m ModPicker) Init() tea.Cmd { return nil }

func (m ModPicker) Update(msg tea.Msg) (ModPicker, tea.Cmd) {
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "enter":
			if item, ok := m.list.SelectedItem().(modItem); ok {
				return m, func() tea.Msg {
					return ModSelectedMsg{Mod: item.name}
				}
			}
		case "esc":
			return m, func() tea.Msg {
				return ModPickerCancelledMsg{}
			}
		}
	}
	return m, cmd
}

func (m ModPicker) View() string {
	return m.list.View() + "\n" + style.KeyHint("/", "filter")
}

func (m *ModPicker) SetSize(w, h int) {
	m.list.SetSize(w, h)
}
