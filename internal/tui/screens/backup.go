package screens

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/noisethanks/atak/internal/archive"
	"github.com/noisethanks/atak/internal/config"
	"github.com/noisethanks/atak/internal/tools"
	"github.com/noisethanks/atak/internal/tui/components"
	"github.com/noisethanks/atak/internal/tui/style"
)

type backupState int

const (
	backupStateMenu backupState = iota
	backupStatePickArchive
	backupStateOperation
	backupStatePickMod
	backupStateConfirmRestore
	backupStateRestoreDone
	backupStateConfirmRestoreAll
	backupStateRestoringAllDone
	backupStateConfirmDelete
)

type backupPendingAction int

const (
	pendingRestoreSingle backupPendingAction = iota
	pendingRestoreAll
	pendingVerify
	pendingDelete
)

type backupActiveOp int

const (
	activeOpBackup backupActiveOp = iota
	activeOpRestoreSingle
	activeOpRestoreAll
	activeOpVerify
)

type backupsLoadedMsg struct{ backups []archive.BackupInfo }

type modsListedMsg struct {
	mods       []string
	backupName string
	backupPath string
}

var backupMenuItems = []string{
	"Create New Backup",
	"Restore Single Mod",
	"Restore All",
	"Verify Archive",
	"Delete Backup",
}

// BackupModel manages all archive operations: create, restore single, restore all, verify, delete.
type BackupModel struct {
	cfg           *config.Config
	tools         *tools.EmbeddedTools
	state         backupState
	backups       []archive.BackupInfo
	menuCursor    int
	cursor        int
	pendingAction backupPendingAction
	activeOp      backupActiveOp
	modPicker     components.ModPicker
	selectedMod   string
	selectedBak   archive.BackupInfo
	opScreen      components.OperationScreen
	statusMsg     string
	errMsg        string
	width         int
	height        int
}

func NewBackup(cfg *config.Config, t *tools.EmbeddedTools) BackupModel {
	return BackupModel{
		cfg:       cfg,
		tools:     t,
		modPicker: components.NewModPicker(nil, 60, 20),
	}
}

func (m BackupModel) Init() tea.Cmd {
	if !dirExists(m.cfg.BackupDir) {
		notice := ""
		if m.cfg.BackupDir != "" {
			notice = "Backup directory not found: " + m.cfg.BackupDir
		}
		return func() tea.Msg { return NavigateMsg{To: NavWelcome, Data: notice} }
	}
	if !dirExists(m.cfg.ModsDir) {
		notice := ""
		if m.cfg.ModsDir != "" {
			notice = "Mods directory not found: " + m.cfg.ModsDir
		}
		return func() tea.Msg { return NavigateMsg{To: NavWelcome, Data: notice} }
	}
	return m.loadBackups()
}

func (m BackupModel) loadBackups() tea.Cmd {
	dir := m.cfg.BackupDir
	return func() tea.Msg {
		backups, _ := archive.ListBackups(dir)
		return backupsLoadedMsg{backups: backups}
	}
}

func (m BackupModel) Update(msg tea.Msg) (BackupModel, tea.Cmd) {
	if m.state == backupStateOperation {
		var cmd tea.Cmd
		m.opScreen, cmd = m.opScreen.Update(msg)
		if m.opScreen.IsDone() {
			return m.handleOperationDone()
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case backupsLoadedMsg:
		m.backups = msg.backups
		if m.cursor >= len(m.backups) && len(m.backups) > 0 {
			m.cursor = len(m.backups) - 1
		}
		return m, nil

	case modsListedMsg:
		m.modPicker = components.NewModPicker(msg.mods, m.width-4, m.height-6)
		m.selectedBak = archive.BackupInfo{Name: msg.backupName, Path: msg.backupPath}
		for _, b := range m.backups {
			if b.Path == msg.backupPath {
				m.selectedBak = b
				break
			}
		}
		m.state = backupStatePickMod
		return m, nil

	case components.ModSelectedMsg:
		m.selectedMod = msg.Mod
		m.state = backupStateConfirmRestore
		return m, nil

	case components.ModPickerCancelledMsg:
		m.state = backupStatePickArchive
		return m, nil

	case tea.KeyMsg:
		if m.state == backupStatePickMod {
			var cmd tea.Cmd
			m.modPicker, cmd = m.modPicker.Update(msg)
			return m, cmd
		}
		return m.handleKey(msg.String())
	}

	if m.state == backupStatePickMod {
		var cmd tea.Cmd
		m.modPicker, cmd = m.modPicker.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m BackupModel) handleKey(key string) (BackupModel, tea.Cmd) {
	switch m.state {
	case backupStateMenu:
		m.statusMsg = ""
		m.errMsg = ""
		switch key {
		case "up", "k":
			if m.menuCursor > 0 {
				m.menuCursor--
			}
		case "down", "j":
			if m.menuCursor < len(backupMenuItems)-1 {
				m.menuCursor++
			}
		case "enter", " ":
			m.cursor = 0
			switch m.menuCursor {
			case 0:
				return m.startBackup()
			case 1:
				return m.selectArchiveOrPick(pendingRestoreSingle)
			case 2:
				return m.selectArchiveOrPick(pendingRestoreAll)
			case 3:
				return m.selectArchiveOrPick(pendingVerify)
			case 4:
				return m.selectArchiveOrPick(pendingDelete)
			}
		case "esc", "q":
			return m, func() tea.Msg { return NavigateMsg{To: NavMenu} }
		}

	case backupStatePickArchive:
		switch key {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.backups)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.backups) == 0 {
				return m, nil
			}
			return m.handleSelectedArchive(m.backups[m.cursor])
		case "esc", "q":
			m.state = backupStateMenu
		}

	case backupStateConfirmRestore:
		switch key {
		case "y", "enter":
			return m.startRestore()
		case "n", "esc":
			m.state = backupStatePickMod
		}

	case backupStateRestoreDone, backupStateRestoringAllDone:
		m.state = backupStateMenu
		return m, m.loadBackups()

	case backupStateConfirmRestoreAll:
		switch key {
		case "y", "enter":
			return m.startRestoreAll()
		case "n", "esc":
			m.state = backupStatePickArchive
		}

	case backupStateConfirmDelete:
		switch key {
		case "y":
			if err := archive.Delete(m.selectedBak.Path); err != nil {
				m.errMsg = err.Error()
			} else {
				m.statusMsg = "Deleted."
				if m.cursor > 0 {
					m.cursor--
				}
			}
			m.state = backupStateMenu
			return m, m.loadBackups()
		case "n", "esc":
			m.state = backupStatePickArchive
		}
	}
	return m, nil
}

func (m BackupModel) handleOperationDone() (BackupModel, tea.Cmd) {
	err := m.opScreen.Err()
	switch m.activeOp {
	case activeOpBackup:
		if err != nil {
			m.errMsg = err.Error()
		} else {
			m.statusMsg = "Backup complete."
		}
		m.state = backupStateMenu
		return m, m.loadBackups()
	case activeOpRestoreSingle:
		if err != nil {
			m.errMsg = err.Error()
		} else {
			m.statusMsg = fmt.Sprintf("Restored %q successfully.", m.selectedMod)
		}
		m.state = backupStateRestoreDone
		return m, nil
	case activeOpRestoreAll:
		if err != nil {
			m.errMsg = err.Error()
		} else {
			m.statusMsg = "Restore all complete."
		}
		m.state = backupStateRestoringAllDone
		return m, nil
	case activeOpVerify:
		if err != nil {
			m.errMsg = "Verify failed: " + err.Error()
		} else {
			m.statusMsg = "Archive verified OK."
		}
		m.state = backupStateMenu
		return m, nil
	}
	return m, nil
}

func (m BackupModel) selectArchiveOrPick(action backupPendingAction) (BackupModel, tea.Cmd) {
	// Restore actions extract into ModsDir; block before archive picker is shown.
	if action == pendingRestoreSingle || action == pendingRestoreAll {
		if !dirExists(m.cfg.ModsDir) {
			notice := ""
			if m.cfg.ModsDir != "" {
				notice = "Mods directory not found: " + m.cfg.ModsDir
			}
			return m, func() tea.Msg { return NavigateMsg{To: NavWelcome, Data: notice} }
		}
	}
	m.pendingAction = action
	if len(m.backups) == 0 {
		return m, nil
	}
	if len(m.backups) == 1 {
		m.cursor = 0
		return m.handleSelectedArchive(m.backups[0])
	}
	m.state = backupStatePickArchive
	return m, nil
}

func (m BackupModel) handleSelectedArchive(bk archive.BackupInfo) (BackupModel, tea.Cmd) {
	switch m.pendingAction {
	case pendingRestoreSingle:
		szPath := m.tools.SevenZipPath
		return m, func() tea.Msg {
			mods, err := archive.ListMods(szPath, bk.Path)
			if err != nil {
				return modsListedMsg{backupName: bk.Name, backupPath: bk.Path}
			}
			return modsListedMsg{mods: mods, backupName: bk.Name, backupPath: bk.Path}
		}
	case pendingRestoreAll:
		m.selectedBak = bk
		m.state = backupStateConfirmRestoreAll
	case pendingVerify:
		m.selectedBak = bk
		return m.startVerify()
	case pendingDelete:
		m.selectedBak = bk
		m.state = backupStateConfirmDelete
	}
	return m, nil
}

func (m BackupModel) startBackup() (BackupModel, tea.Cmd) {
	if !dirExists(m.cfg.ModsDir) {
		notice := ""
		if m.cfg.ModsDir != "" {
			notice = "Mods directory not found: " + m.cfg.ModsDir
		}
		return m, func() tea.Msg { return NavigateMsg{To: NavWelcome, Data: notice} }
	}
	ctx, cancel := context.WithCancel(context.Background())
	outPath := filepath.Join(m.cfg.BackupDir, fmt.Sprintf(
		"gamma-backup-%s.7z", time.Now().Format("2006-01-02-150405")))
	sizeFunc := func() int64 {
		info, err := os.Stat(outPath)
		if err != nil {
			return 0
		}
		return info.Size()
	}
	progCh, doneCh := archive.Backup(ctx, m.tools.SevenZipPath, m.cfg.ModsDir, outPath, m.cfg.BackupLevel)
	ch := archiveToCh(progCh, doneCh, sizeFunc)
	m.opScreen = components.NewOperationScreen("Creating Backup…", ch, cancel)
	m.opScreen.SetSize(m.width, m.height)
	m.state = backupStateOperation
	m.activeOp = activeOpBackup
	return m, tea.Batch(
		func() tea.Msg { return OperationStartedMsg{Cancel: cancel} },
		m.opScreen.Init(),
	)
}

func (m BackupModel) startRestore() (BackupModel, tea.Cmd) {
	if !dirExists(m.cfg.ModsDir) {
		notice := ""
		if m.cfg.ModsDir != "" {
			notice = "Mods directory not found: " + m.cfg.ModsDir
		}
		return m, func() tea.Msg { return NavigateMsg{To: NavWelcome, Data: notice} }
	}
	ctx, cancel := context.WithCancel(context.Background())
	modName := m.selectedMod
	modDir := m.cfg.ModsDir
	restoreTarget := filepath.Join(modDir, modName)
	sizeFunc := func() int64 {
		var total int64
		_ = filepath.Walk(restoreTarget, func(_ string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			total += info.Size()
			return nil
		})
		return total
	}
	progCh, doneCh := archive.Restore(ctx, m.tools.SevenZipPath, m.selectedBak.Path, modName, modDir)
	ch := archiveToCh(progCh, doneCh, sizeFunc)
	m.opScreen = components.NewOperationScreen("Restoring: "+modName+"…", ch, cancel)
	m.opScreen.SetSize(m.width, m.height)
	m.state = backupStateOperation
	m.activeOp = activeOpRestoreSingle
	return m, tea.Batch(
		func() tea.Msg { return OperationStartedMsg{Cancel: cancel} },
		m.opScreen.Init(),
	)
}

func (m BackupModel) startRestoreAll() (BackupModel, tea.Cmd) {
	if !dirExists(m.cfg.ModsDir) {
		notice := ""
		if m.cfg.ModsDir != "" {
			notice = "Mods directory not found: " + m.cfg.ModsDir
		}
		return m, func() tea.Msg { return NavigateMsg{To: NavWelcome, Data: notice} }
	}
	ctx, cancel := context.WithCancel(context.Background())
	modsDir := m.cfg.ModsDir
	sizeFunc := func() int64 {
		var total int64
		_ = filepath.Walk(modsDir, func(_ string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			total += info.Size()
			return nil
		})
		return total
	}
	progCh, doneCh := archive.RestoreAll(ctx, m.tools.SevenZipPath, m.selectedBak.Path, filepath.Dir(modsDir))
	ch := archiveToCh(progCh, doneCh, sizeFunc)
	m.opScreen = components.NewOperationScreen("Restoring All Mods…", ch, cancel)
	m.opScreen.SetSize(m.width, m.height)
	m.state = backupStateOperation
	m.activeOp = activeOpRestoreAll
	return m, tea.Batch(
		func() tea.Msg { return OperationStartedMsg{Cancel: cancel} },
		m.opScreen.Init(),
	)
}

func (m BackupModel) startVerify() (BackupModel, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := verifyToCh(ctx, m.tools.SevenZipPath, m.selectedBak.Path)
	m.opScreen = components.NewOperationScreen("Verifying: "+m.selectedBak.Name, ch, cancel)
	m.opScreen.SetSize(m.width, m.height)
	m.state = backupStateOperation
	m.activeOp = activeOpVerify
	return m, tea.Batch(
		func() tea.Msg { return OperationStartedMsg{Cancel: cancel} },
		m.opScreen.Init(),
	)
}

// archiveToCh bridges archive progress channels to OperationProgressMsg.
// sizeFunc is called on each progress event and on a 1-second ticker; pass nil to skip size reporting.
func archiveToCh(
	progCh <-chan archive.ProgressMsg,
	doneCh <-chan error,
	sizeFunc func() int64,
) <-chan components.OperationProgressMsg {
	out := make(chan components.OperationProgressMsg, 32)
	go func() {
		defer close(out)
		var lastPct int
		var lastStatus string
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case p, ok := <-progCh:
				if !ok {
					err := <-doneCh
					size := int64(0)
					if sizeFunc != nil {
						size = sizeFunc()
					}
					out <- components.OperationProgressMsg{
						Done: true, Err: err,
						Percent: lastPct, Status: lastStatus, Size: size,
					}
					return
				}
				lastPct = p.Percent
				lastStatus = p.CurrentFile
				size := int64(0)
				if sizeFunc != nil {
					size = sizeFunc()
				}
				out <- components.OperationProgressMsg{
					Percent: lastPct,
					Status:  lastStatus,
					Size:    size,
				}
			case err := <-doneCh:
				size := int64(0)
				if sizeFunc != nil {
					size = sizeFunc()
				}
				out <- components.OperationProgressMsg{
					Done: true, Err: err,
					Percent: lastPct, Size: size,
				}
				return
			case <-ticker.C:
				if sizeFunc != nil {
					size := sizeFunc()
					out <- components.OperationProgressMsg{
						Percent: lastPct,
						Status:  lastStatus,
						Size:    size,
					}
				}
			}
		}
	}()
	return out
}

// verifyToCh wraps the synchronous archive.Verify into a OperationProgressMsg channel.
func verifyToCh(ctx context.Context, szPath, archivePath string) <-chan components.OperationProgressMsg {
	out := make(chan components.OperationProgressMsg, 1)
	go func() {
		defer close(out)
		err := archive.Verify(szPath, archivePath)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		out <- components.OperationProgressMsg{Done: true, Err: err}
	}()
	return out
}

func (m BackupModel) View() string {
	if m.state == backupStateOperation {
		return m.opScreen.View()
	}

	var b strings.Builder
	b.WriteString(style.StyleTitle.Render("Backup Manager") + "\n\n")

	switch m.state {
	case backupStateMenu:
		b.WriteString(style.StyleMuted.Render("Backups in "+m.cfg.BackupDir+":") + "\n\n")
		if len(m.backups) == 0 {
			b.WriteString(style.StyleMuted.Render("  No backups found") + "\n")
		} else {
			for _, bk := range m.backups {
				b.WriteString(fmt.Sprintf("  %-40s  %s  %s\n",
					bk.Name, formatBytes(bk.Size), bk.ModTime.Format("Jan 02 15:04")))
			}
		}
		b.WriteString("\n")
		if m.errMsg != "" {
			b.WriteString(style.StyleDanger.Render("Error: "+m.errMsg) + "\n\n")
		}
		if m.statusMsg != "" {
			b.WriteString(style.StyleSuccess.Render(m.statusMsg) + "\n\n")
		}
		for i, item := range backupMenuItems {
			if i == m.menuCursor {
				b.WriteString(style.StyleSelected.Render("▶ "+item) + "\n")
			} else {
				b.WriteString(style.StyleBody.Render("  "+item) + "\n")
			}
		}
		b.WriteString("\n" + style.KeyHint("↑↓", "navigate") + "  " + style.KeyHint("enter", "select") + "  " + style.KeyHint("q", "back"))

	case backupStatePickArchive:
		if len(m.backups) == 0 {
			b.WriteString(style.StyleMuted.Render("No backups found in "+m.cfg.BackupDir) + "\n\n")
			b.WriteString(style.KeyHint("esc", "back"))
			return b.String()
		}
		b.WriteString(style.StyleBody.Render("Select backup:") + "\n\n")
		for i, bk := range m.backups {
			prefix := "  "
			if i == m.cursor {
				prefix = style.StyleSelected.Render("▶ ")
			}
			b.WriteString(fmt.Sprintf("%s%-40s  %s  %s\n",
				prefix, bk.Name, formatBytes(bk.Size),
				bk.ModTime.Format("2006-01-02 15:04"),
			))
		}
		b.WriteString("\n" + style.KeyHint("enter", "select") + "  " + style.KeyHint("esc", "back"))

	case backupStatePickMod:
		b.WriteString(style.StyleMuted.Render("Archive: "+m.selectedBak.Name) + "\n")
		b.WriteString(m.modPicker.View())

	case backupStateConfirmRestore:
		b.WriteString(style.StyleBody.Render(fmt.Sprintf(
			"Restore %q from %q?\n\nThis will overwrite existing files.",
			m.selectedMod, m.selectedBak.Name,
		)) + "\n\n")
		b.WriteString(style.KeyHint("y/enter", "yes") + "  " + style.KeyHint("n/esc", "no"))

	case backupStateRestoreDone:
		if m.errMsg != "" {
			b.WriteString(style.StyleDanger.Render("Error: "+m.errMsg) + "\n")
		} else {
			b.WriteString(style.StyleSuccess.Render(m.statusMsg) + "\n")
		}
		b.WriteString("\n" + style.KeyHint("any key", "back"))

	case backupStateConfirmRestoreAll:
		modsParentDir := filepath.Dir(m.cfg.ModsDir)
		b.WriteString(style.StyleWarning.Render(fmt.Sprintf(
			"Extract entire archive %q to %q?\n\nThis will overwrite all existing mod files.",
			m.selectedBak.Name, modsParentDir,
		)) + "\n\n")
		b.WriteString(style.KeyHint("y/enter", "yes") + "  " + style.KeyHint("n/esc", "no"))

	case backupStateRestoringAllDone:
		if m.errMsg != "" {
			b.WriteString(style.StyleDanger.Render("Error: "+m.errMsg) + "\n")
		} else {
			b.WriteString(style.StyleSuccess.Render(m.statusMsg) + "\n")
		}
		b.WriteString("\n" + style.KeyHint("any key", "back"))

	case backupStateConfirmDelete:
		b.WriteString(style.StyleWarning.Render(
			"Delete "+m.selectedBak.Name+"? (y/n)",
		) + "\n")
	}

	return b.String()
}

func (m *BackupModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.modPicker.SetSize(w-4, h-6)
	m.opScreen.SetSize(w, h)
}
