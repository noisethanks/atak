package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/noisethanks/atak/internal/config"
	"github.com/noisethanks/atak/internal/scan"
	"github.com/noisethanks/atak/internal/tools"
	"github.com/noisethanks/atak/internal/tui/screens"
)

// Screen identifies which screen is currently active.
type Screen int

const (
	ScreenWelcome Screen = iota
	ScreenFirstRun
	ScreenMenu
	ScreenScan
	ScreenResults
	ScreenCompress
	ScreenSummary
	ScreenBackup
	ScreenSettings
	ScreenAbout
	ScreenUnmatched
)

// AppModel is the root Bubble Tea model. It owns shared state and delegates
// Update/View to the active sub-screen model.
type AppModel struct {
	screen    Screen
	cfg       *config.Config
	tools     *tools.EmbeddedTools
	version   string
	cancelOp  context.CancelFunc
	cancelMsg string
	width     int
	height    int

	scanAssets  []scan.Asset
	scanSkipped int

	// Screens.
	welcome    screens.WelcomeModel
	firstrun   screens.FirstRunModel
	about      screens.AboutModel
	menu       screens.MenuModel
	scanScreen screens.ScanModel
	results    screens.ResultsModel
	compress   screens.CompressModel
	summary    screens.SummaryModel
	backup     screens.BackupModel
	settings   screens.SettingsModel
	unmatched  screens.UnmatchedModel
}

// New creates the root model.
// profilesCreated: show first-run profiles notice.
// firstRun: show welcome/path-config screen.
func New(cfg *config.Config, t *tools.EmbeddedTools, firstRun bool, profilesCreated bool, version string) AppModel {
	startScreen := ScreenMenu
	switch {
	case profilesCreated:
		startScreen = ScreenFirstRun
	case firstRun:
		startScreen = ScreenWelcome
	}
	m := AppModel{
		screen:  startScreen,
		cfg:     cfg,
		tools:   t,
		version: version,
	}
	m.initScreens()
	return m
}

func (m *AppModel) initScreens() {
	cfgDir, _ := config.ConfigDir()
	m.welcome = screens.NewWelcome(m.cfg)
	m.firstrun = screens.NewFirstRun(cfgDir)
	m.about = screens.NewAbout(m.version, tools.LicenseText)
	m.menu = screens.NewMenu()
	m.backup = screens.NewBackup(m.cfg, m.tools)
	m.settings = screens.NewSettings(m.cfg)
}

func (m AppModel) Init() tea.Cmd {
	switch m.screen {
	case ScreenWelcome:
		return m.welcome.Init()
	case ScreenMenu:
		return m.menu.Init()
	default:
		return nil
	}
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m.propagateSize(msg)

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			if m.cancelOp != nil {
				m.cancelOp()
				m.cancelOp = nil
				m.screen = ScreenMenu
				m.menu = screens.NewMenuWithStatus(m.cancelMsg)
				return m, m.menu.Init()
			}
			return m, tea.Quit
		}

	case screens.NavigateMsg:
		return m.handleNavigate(msg)

	case screens.OperationStartedMsg:
		m.cancelOp = msg.Cancel
		m.cancelMsg = msg.CancelMsg
		if m.cancelMsg == "" {
			m.cancelMsg = "Operation cancelled"
		}
		return m, nil
	}

	return m.delegateUpdate(msg)
}

func (m AppModel) View() string {
	switch m.screen {
	case ScreenWelcome:
		return m.welcome.View()
	case ScreenFirstRun:
		return m.firstrun.View()
	case ScreenMenu:
		return m.menu.View()
	case ScreenScan:
		return m.scanScreen.View()
	case ScreenResults:
		return m.results.View()
	case ScreenCompress:
		return m.compress.View()
	case ScreenSummary:
		return m.summary.View()
	case ScreenBackup:
		return m.backup.View()
	case ScreenSettings:
		return m.settings.View()
	case ScreenAbout:
		return m.about.View()
	case ScreenUnmatched:
		return m.unmatched.View()
	default:
		return ""
	}
}

func (m AppModel) delegateUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.screen {
	case ScreenWelcome:
		m.welcome, cmd = m.welcome.Update(msg)
	case ScreenFirstRun:
		m.firstrun, cmd = m.firstrun.Update(msg)
	case ScreenMenu:
		m.menu, cmd = m.menu.Update(msg)
	case ScreenScan:
		m.scanScreen, cmd = m.scanScreen.Update(msg)
	case ScreenResults:
		m.results, cmd = m.results.Update(msg)
	case ScreenCompress:
		m.compress, cmd = m.compress.Update(msg)
	case ScreenSummary:
		m.summary, cmd = m.summary.Update(msg)
	case ScreenBackup:
		m.backup, cmd = m.backup.Update(msg)
	case ScreenSettings:
		m.settings, cmd = m.settings.Update(msg)
	case ScreenAbout:
		m.about, cmd = m.about.Update(msg)
	case ScreenUnmatched:
		m.unmatched, cmd = m.unmatched.Update(msg)
	}
	return m, cmd
}

func (m AppModel) propagateSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	// Forward size to every screen so they can adapt layout.
	m.welcome.SetSize(msg.Width, msg.Height)
	m.firstrun.SetSize(msg.Width, msg.Height)
	m.menu.SetSize(msg.Width, msg.Height)
	m.scanScreen.SetSize(msg.Width, msg.Height)
	m.results.SetSize(msg.Width, msg.Height)
	m.compress.SetSize(msg.Width, msg.Height)
	m.summary.SetSize(msg.Width, msg.Height)
	m.backup.SetSize(msg.Width, msg.Height)
	m.settings.SetSize(msg.Width, msg.Height)
	m.about.SetSize(msg.Width, msg.Height)
	m.unmatched.SetSize(msg.Width, msg.Height)
	return m, nil
}

func (m AppModel) handleNavigate(msg screens.NavigateMsg) (tea.Model, tea.Cmd) {
	m.cancelOp = nil
	switch msg.To {
	case screens.NavMenu:
		m.screen = ScreenMenu
		m.menu = screens.NewMenu()
		return m, m.menu.Init()

	case screens.NavScan:
		m.screen = ScreenScan
		m.scanScreen = screens.NewScan(m.cfg, m.tools)
		m.scanScreen.SetSize(m.width, m.height)
		return m, m.scanScreen.Init()

	case screens.NavResults:
		if data, ok := msg.Data.(screens.ScanResultData); ok {
			m.scanAssets = data.Assets
			m.scanSkipped = data.Skipped
		}
		m.screen = ScreenResults
		m.results = screens.NewResults(screens.ScanResultData{
			Assets:  m.scanAssets,
			Skipped: m.scanSkipped,
		}, m.cfg)
		m.results.SetSize(m.width, m.height)
		return m, m.results.Init()

	case screens.NavCompress:
		data, _ := msg.Data.(screens.CompressJobData)
		m.screen = ScreenCompress
		m.compress = screens.NewCompress(data, m.cfg, m.tools)
		m.compress.SetSize(m.width, m.height)
		return m, m.compress.Init()

	case screens.NavSummary:
		data, _ := msg.Data.(screens.SummaryData)
		m.screen = ScreenSummary
		m.summary = screens.NewSummary(data)
		m.summary.SetSize(m.width, m.height)
		return m, m.summary.Init()

	case screens.NavBackup:
		m.screen = ScreenBackup
		m.backup = screens.NewBackup(m.cfg, m.tools)
		m.backup.SetSize(m.width, m.height)
		return m, m.backup.Init()

	case screens.NavSettings:
		m.screen = ScreenSettings
		m.settings = screens.NewSettings(m.cfg)
		m.settings.SetSize(m.width, m.height)
		return m, m.settings.Init()

	case screens.NavAbout:
		m.screen = ScreenAbout
		m.about = screens.NewAbout(m.version, tools.LicenseText)
		m.about.SetSize(m.width, m.height)
		return m, m.about.Init()

	case screens.NavSaveConfig:
		if updated, ok := msg.Data.(*config.Config); ok {
			m.cfg = updated
			_ = config.Save(m.cfg)
		}
		m.screen = ScreenMenu
		m.menu = screens.NewMenu()
		return m, m.menu.Init()

	case screens.NavWelcome:
		m.screen = ScreenWelcome
		notice, _ := msg.Data.(string)
		m.welcome = screens.NewWelcomeWithNotice(m.cfg, notice)
		return m, m.welcome.Init()

	case screens.NavUnmatched:
		m.screen = ScreenUnmatched
		m.unmatched = screens.NewUnmatched(m.scanAssets)
		m.unmatched.SetSize(m.width, m.height)
		return m, m.unmatched.Init()

	case screens.NavQuit:
		return m, tea.Quit
	}
	return m, nil
}
