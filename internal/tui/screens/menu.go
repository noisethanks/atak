package screens

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/noisethanks/atak/internal/tui/style"
)

type menuItem struct {
	label string
	desc  string
	nav   NavTarget
}

var menuItems = []menuItem{
	{label: "Scan & Compress", desc: "Walk mods dir, identify and compress textures", nav: NavScan},
	{label: "Backup Manager", desc: "Create, restore, verify, and delete backups", nav: NavBackup},
	{label: "Settings", desc: "Configure paths and worker threads", nav: NavSettings},
	{label: "About", desc: "Version and third-party licenses", nav: NavAbout},
	{label: "Quit", desc: "", nav: NavQuit},
}

// MenuModel is the main hub screen.
type MenuModel struct {
	cursor    int
	statusMsg string
	width     int
	height    int
}

func NewMenu() MenuModel {
	return MenuModel{}
}

func NewMenuWithStatus(msg string) MenuModel {
	return MenuModel{statusMsg: msg}
}

func (m MenuModel) Init() tea.Cmd { return nil }

func (m MenuModel) Update(msg tea.Msg) (MenuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(menuItems)-1 {
				m.cursor++
			}
		case "enter", " ":
			item := menuItems[m.cursor]
			return m, func() tea.Msg { return NavigateMsg{To: item.nav} }
		}
	}
	return m, nil
}

func (m MenuModel) View() string {
	var b strings.Builder
	b.WriteString(style.StyleTitle.Render("atak") + "\n")
	b.WriteString(style.StyleSubtitle.Render("S.T.A.L.K.E.R. Anomaly texture compressor & backup tool") + "\n\n")

	if m.statusMsg != "" {
		b.WriteString(style.StyleMuted.Render(m.statusMsg) + "\n\n")
	}

	for i, item := range menuItems {
		if i == m.cursor {
			b.WriteString(style.StyleSelected.Render("▶ " + item.label))
		} else {
			b.WriteString(style.StyleBody.Render("  " + item.label))
		}
		if item.desc != "" {
			b.WriteString("  " + style.StyleMuted.Render(item.desc))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n" + style.KeyHint("↑↓", "navigate") + "  " + style.KeyHint("enter", "select"))
	b.WriteString("\n" + style.StyleMuted.Render("Tip: on most terminals, Ctrl + and Ctrl - change the font size"))
	return b.String()
}

func (m *MenuModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}
