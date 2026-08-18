package screens

import (
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/noisethanks/atak/internal/config"
	"github.com/noisethanks/atak/internal/tui/style"
)

// WelcomeModel handles first-run path configuration and re-configuration when a
// previously set path is no longer valid.
type WelcomeModel struct {
	cfg       *config.Config
	modsInput textinput.Model
	bkupInput textinput.Model
	focused   int // 0 = mods, 1 = backup
	notice    string
	width     int
	height    int
	err       string
}

func NewWelcome(cfg *config.Config) WelcomeModel {
	return NewWelcomeWithNotice(cfg, "")
}

// welcomePlaceholders returns the OS-appropriate placeholder text for the mods
// and backup directory inputs. Pulled out as a pure function of goos (rather
// than reading runtime.GOOS inline) so both branches can be exercised from a
// single test run regardless of which OS is actually running the tests.
func welcomePlaceholders(goos string) (mods, backup string) {
	if goos == "windows" {
		return `C:\Games\GAMMA\mods`, `C:\Users\user\backups`
	}
	return "/home/user/Games/GAMMA/mods", "/home/user/backups"
}

// NewWelcomeWithNotice creates the welcome screen with an optional notice shown
// above the path inputs. Used when routing here because a configured path is no
// longer valid; pass an empty string for the normal first-run case.
func NewWelcomeWithNotice(cfg *config.Config, notice string) WelcomeModel {
	// Placeholders mirror the GAMMA-convention candidates detectModsDir()
	// already looks for, per-OS, so first-run users see a path shaped like
	// the one the tool would actually have auto-detected.
	modsPlaceholder, bkupPlaceholder := welcomePlaceholders(runtime.GOOS)

	mods := textinput.New()
	mods.Placeholder = modsPlaceholder
	mods.SetValue(cfg.ModsDir)
	mods.Focus()
	mods.Width = 60

	bkup := textinput.New()
	bkup.Placeholder = bkupPlaceholder
	bkup.SetValue(cfg.BackupDir)
	bkup.Width = 60

	return WelcomeModel{
		cfg:       cfg,
		modsInput: mods,
		bkupInput: bkup,
		notice:    notice,
	}
}

func (m WelcomeModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m WelcomeModel) Update(msg tea.Msg) (WelcomeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			m.focused = (m.focused + 1) % 2
			if m.focused == 0 {
				m.modsInput.Focus()
				m.bkupInput.Blur()
			} else {
				m.bkupInput.Focus()
				m.modsInput.Blur()
			}
		case "shift+tab", "up":
			m.focused = (m.focused + 1) % 2
			if m.focused == 0 {
				m.modsInput.Focus()
				m.bkupInput.Blur()
			} else {
				m.bkupInput.Focus()
				m.modsInput.Blur()
			}
		case "enter":
			if m.focused == 0 {
				// Move to backup field.
				m.focused = 1
				m.bkupInput.Focus()
				m.modsInput.Blur()
				return m, nil
			}
			// Validate and save.
			modsDir := strings.TrimSpace(m.modsInput.Value())
			bkupDir := strings.TrimSpace(m.bkupInput.Value())
			if modsDir == "" {
				m.err = "Mods directory is required."
				return m, nil
			}
			m.cfg.ModsDir = modsDir
			m.cfg.BackupDir = bkupDir
			return m, func() tea.Msg {
				return NavigateMsg{To: NavSaveConfig, Data: m.cfg}
			}
		case "ctrl+c", "q":
			return m, func() tea.Msg { return NavigateMsg{To: NavQuit} }
		}
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd
	if m.focused == 0 {
		m.modsInput, cmd = m.modsInput.Update(msg)
	} else {
		m.bkupInput, cmd = m.bkupInput.Update(msg)
	}
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m WelcomeModel) View() string {
	var b strings.Builder
	b.WriteString(style.StyleTitle.Render("atak") + "\n")
	b.WriteString(style.StyleSubtitle.Render("S.T.A.L.K.E.R. Anomaly texture compressor & backup tool") + "\n\n")
	if m.notice != "" {
		b.WriteString(style.StyleWarning.Render(m.notice) + "\n\n")
	} else {
		b.WriteString(style.StyleBody.Render("Welcome! Let's set up your paths before we begin.") + "\n\n")
	}

	b.WriteString(style.StyleSelected.Render("Anomaly Mods Directory") + "\n")
	b.WriteString(m.modsInput.View() + "\n\n")

	b.WriteString(style.StyleSelected.Render("Backup Directory") + "\n")
	b.WriteString(m.bkupInput.View() + "\n\n")

	if m.err != "" {
		b.WriteString(style.StyleDanger.Render("  "+m.err) + "\n\n")
	}

	b.WriteString(style.StyleKeyHint.Render("tab") + style.StyleMuted.Render(" next field  ") +
		style.StyleKeyHint.Render("enter") + style.StyleMuted.Render(" confirm  ") +
		style.StyleKeyHint.Render("q") + style.StyleMuted.Render(" quit"))

	return b.String()
}

func (m *WelcomeModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}
