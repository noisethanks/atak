package screens

import (
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/noisethanks/atak/internal/config"
	"github.com/noisethanks/atak/internal/tui/style"
)

type settingsField int

const (
	fieldModsDir settingsField = iota
	fieldBackupDir
	fieldWorkers
	fieldBackupLevel
	fieldStripMips          // bool toggle — no text input
	fieldCompressionBackend // two-way selector, hidden on darwin (no compressonator build)
	fieldModOutputMode      // bool toggle — no text input
	fieldModOutputName      // text input, shown only when ModOutputMode is on
	fieldModlistPath        // text input, shown only when ModOutputMode is on
	fieldCount
)

// inputIdx maps a field to its index in SettingsModel.inputs.
// Returns -1 for fields that are not text inputs (e.g. the toggle).
func inputIdx(f settingsField) int {
	switch f {
	case fieldModsDir:
		return 0
	case fieldBackupDir:
		return 1
	case fieldWorkers:
		return 2
	case fieldBackupLevel:
		return 3
	case fieldModOutputName:
		return 4
	case fieldModlistPath:
		return 5
	}
	return -1
}

// SettingsModel handles configuring user preferences.
type SettingsModel struct {
	cfg           *config.Config
	inputs        [6]textinput.Model // modsDir, backupDir, workers, backupLevel, modOutputName, modlistPath
	modOutputMode bool
	focused       settingsField
	errMsg        string
	width         int
	height        int
	// stripMips mirrors cfg.StripMipsWhenDisabled while the toggle is being edited.
	stripMips bool
	// compressionBackend mirrors cfg.CompressionBackend while the selector is
	// being edited. On darwin this stays "texconv" — the row is hidden and
	// there's no way to change it.
	compressionBackend string
}

func NewSettings(cfg *config.Config) SettingsModel {
	mods := textinput.New()
	mods.SetValue(cfg.ModsDir)
	mods.Width = 60
	mods.Focus()

	backup := textinput.New()
	backup.SetValue(cfg.BackupDir)
	backup.Width = 60

	workers := textinput.New()
	workers.SetValue(strconv.Itoa(cfg.WorkerCount))
	workers.Width = 6

	backupLvl := textinput.New()
	backupLvl.SetValue(strconv.Itoa(cfg.BackupLevel))
	backupLvl.Width = 6

	modOutputName := textinput.New()
	modOutputName.SetValue(cfg.ModOutputName)
	modOutputName.Width = 30

	modlistPath := textinput.New()
	modlistPath.SetValue(cfg.ModlistPath)
	modlistPath.Width = 60

	backend := cfg.CompressionBackend
	if backend == "" {
		backend = config.BackendTexconv
	}

	return SettingsModel{
		cfg:                cfg,
		inputs:             [6]textinput.Model{mods, backup, workers, backupLvl, modOutputName, modlistPath},
		modOutputMode:      cfg.ModOutputMode,
		stripMips:          cfg.StripMipsWhenDisabled,
		compressionBackend: backend,
		focused:            fieldModsDir,
	}
}

func (m SettingsModel) Init() tea.Cmd { return textinput.Blink }

func (m SettingsModel) Update(msg tea.Msg) (SettingsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			m.focused = m.nextField()
			m.refocus()
			return m, nil
		case "shift+tab", "up":
			m.focused = m.prevField()
			m.refocus()
			return m, nil
		case "enter":
			return m.save()
		case " ", "left", "right":
			if m.focused == fieldModOutputMode {
				m.modOutputMode = !m.modOutputMode
				return m, nil
			}
			if m.focused == fieldStripMips {
				m.stripMips = !m.stripMips
				return m, nil
			}
			if m.focused == fieldCompressionBackend {
				if m.compressionBackend == config.BackendTexconv {
					m.compressionBackend = config.BackendCompressonatorBc7e
				} else {
					m.compressionBackend = config.BackendTexconv
				}
				return m, nil
			}
		case "esc", "q":
			return m, func() tea.Msg { return NavigateMsg{To: NavMenu} }
		}
	}

	idx := inputIdx(m.focused)
	if idx >= 0 {
		var cmd tea.Cmd
		m.inputs[idx], cmd = m.inputs[idx].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *SettingsModel) refocus() {
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
	idx := inputIdx(m.focused)
	if idx >= 0 {
		m.inputs[idx].Focus()
	}
}

// nextField returns the next visible field after m.focused, wrapping around.
func (m SettingsModel) nextField() settingsField {
	for delta := 1; delta < int(fieldCount); delta++ {
		next := settingsField((int(m.focused) + delta) % int(fieldCount))
		if m.isVisible(next) {
			return next
		}
	}
	return m.focused
}

// prevField returns the previous visible field before m.focused, wrapping around.
func (m SettingsModel) prevField() settingsField {
	for delta := 1; delta < int(fieldCount); delta++ {
		prev := settingsField((int(m.focused) - delta + int(fieldCount)) % int(fieldCount))
		if m.isVisible(prev) {
			return prev
		}
	}
	return m.focused
}

func (m SettingsModel) isVisible(f settingsField) bool {
	if f == fieldModOutputName || f == fieldModlistPath {
		return m.modOutputMode
	}
	if f == fieldCompressionBackend {
		return runtime.GOOS != "darwin"
	}
	return true
}

func (m SettingsModel) save() (SettingsModel, tea.Cmd) {
	workers, err := strconv.Atoi(strings.TrimSpace(m.inputs[2].Value()))
	if err != nil || workers < 1 {
		workers = max(1, runtime.NumCPU()/4)
	}
	backupLevel, err := strconv.Atoi(strings.TrimSpace(m.inputs[3].Value()))
	if err != nil || backupLevel < 1 || backupLevel > 9 {
		backupLevel = m.cfg.BackupLevel
	}
	modOutputName := strings.TrimSpace(m.inputs[4].Value())
	if modOutputName == "" {
		modOutputName = "ATAK"
	}
	updated := *m.cfg
	updated.ModsDir = strings.TrimSpace(m.inputs[0].Value())
	updated.BackupDir = strings.TrimSpace(m.inputs[1].Value())
	updated.WorkerCount = workers
	updated.BackupLevel = backupLevel
	updated.ModOutputMode = m.modOutputMode
	updated.ModOutputName = modOutputName
	updated.ModlistPath = strings.TrimSpace(m.inputs[5].Value())
	updated.StripMipsWhenDisabled = m.stripMips
	if runtime.GOOS == "darwin" {
		updated.CompressionBackend = config.BackendTexconv
	} else {
		updated.CompressionBackend = m.compressionBackend
	}
	return m, func() tea.Msg {
		return NavigateMsg{To: NavSaveConfig, Data: &updated}
	}
}

// settingsFieldLabel maps a field to the exact label text its View() block
// writes, so scrollToFocused can locate the focused field's line without
// duplicating the render logic. Kept in sync with the labels below by hand —
// there's no field this doesn't cover, since every visible field has one.
var settingsFieldLabel = map[settingsField]string{
	fieldModsDir:            "Anomaly Mods Directory",
	fieldBackupDir:          "Backup Directory",
	fieldWorkers:            "Worker Threads",
	fieldBackupLevel:        "Backup Compression Level",
	fieldStripMips:          "Strip Mips When Disabled",
	fieldCompressionBackend: "Compression Backend",
	fieldModOutputMode:      "Mod Output Mode",
	fieldModOutputName:      "Output Mod Name",
	fieldModlistPath:        "MO2 modlist.txt Path",
}

// scrollToFocused windows body (already split into lines) around whichever
// line contains the focused field's label, centering it in avail lines. body
// taller than the terminal was the original bug — nothing scrolled, so
// fields past the bottom (or the top one, once focus wrapped past it) were
// simply unreachable. Stateless by design, like renderSectionList's cursor
// window in summary.go: recomputed fresh every render from m.focused alone,
// no persisted offset to keep in sync.
func scrollToFocused(lines []string, focused settingsField, avail int) []string {
	total := len(lines)
	if avail <= 0 || total <= avail {
		return lines
	}
	focusedLine := 0
	if label, ok := settingsFieldLabel[focused]; ok {
		for i, line := range lines {
			if strings.Contains(line, label) {
				focusedLine = i
				break
			}
		}
	}
	offset := focusedLine - avail/2
	if offset < 0 {
		offset = 0
	}
	if maxOffset := total - avail; offset > maxOffset {
		offset = maxOffset
	}
	end := offset + avail
	visible := append([]string(nil), lines[offset:end]...)
	if offset > 0 {
		visible[0] = style.StyleMuted.Render("↑ more above")
	}
	if end < total {
		visible[len(visible)-1] = style.StyleMuted.Render("↓ more below")
	}
	return visible
}

func (m SettingsModel) View() string {
	var b strings.Builder

	type row struct {
		label   string
		field   settingsField
		content string
		hint    string
	}

	rows := []row{
		{"Anomaly Mods Directory", fieldModsDir, m.inputs[0].View(), ""},
		{"Backup Directory", fieldBackupDir, m.inputs[1].View(), ""},
		{"Worker Threads", fieldWorkers, m.inputs[2].View(), "Conservative default (CPU/4). Increase if compression feels slow and your system has headroom."},
		{"Backup Compression Level", fieldBackupLevel, m.inputs[3].View(),
			"1–9  ·  3 = Fast  ·  6 = Balanced (default)  ·  9 = Maximum"},
	}

	for _, r := range rows {
		label := style.StyleBody.Render(r.label)
		if m.focused == r.field {
			label = style.StyleSelected.Render(r.label)
		}
		b.WriteString(label + "\n" + r.content + "\n")
		if r.hint != "" {
			b.WriteString(style.StyleMuted.Render(r.hint) + "\n")
		}
		b.WriteString("\n")
	}

	// Strip Mips When Disabled toggle.
	{
		toggleLabel := "Strip Mips When Disabled"
		toggleValue := "[ off ]"
		if m.stripMips {
			toggleValue = style.StyleSuccess.Render("[ on  ]")
		}
		if m.focused == fieldStripMips {
			b.WriteString(style.StyleSelected.Render(toggleLabel) + "\n")
		} else {
			b.WriteString(style.StyleBody.Render(toggleLabel) + "\n")
		}
		b.WriteString(toggleValue + "\n")
		b.WriteString(style.StyleMuted.Render("When on, profiles with generateMips=false skip mips entirely, dropping any chain the\n  source shipped. When off (default), a mipped source keeps its chain (flares, reticles).") + "\n\n")
	}

	// Compression backend selector — hidden on macOS since compressonator-bc7e
	// isn't built for darwin.
	if runtime.GOOS != "darwin" {
		label := "Compression Backend"
		var value string
		if m.compressionBackend == config.BackendCompressonatorBc7e {
			value = style.StyleSuccess.Render("[ compressonator-bc7e ]") + "   texconv"
		} else {
			value = "compressonator-bc7e   " + style.StyleSuccess.Render("[ texconv ]")
		}
		if m.focused == fieldCompressionBackend {
			b.WriteString(style.StyleSelected.Render(label) + "\n")
		} else {
			b.WriteString(style.StyleBody.Render(label) + "\n")
		}
		b.WriteString(value + "\n")
		b.WriteString(style.StyleMuted.Render("compressonator-bc7e: CPU only, all 5 BC formats, deterministic across platforms.\n  texconv: GPU-accelerated on Windows for BC7 (much faster there); CPU on Linux.\n  space/←/→ to switch.") + "\n\n")
	}

	// Mod Output Mode toggle.
	{
		toggleLabel := "Mod Output Mode"
		toggleValue := "[ off ]"
		if m.modOutputMode {
			toggleValue = style.StyleSuccess.Render("[ on  ]")
		}
		if m.focused == fieldModOutputMode {
			b.WriteString(style.StyleSelected.Render(toggleLabel) + "\n")
		} else {
			b.WriteString(style.StyleBody.Render(toggleLabel) + "\n")
		}
		b.WriteString(toggleValue + "\n")
		b.WriteString(style.StyleMuted.Render("Output compressed textures to a single MO2-compatible mod folder instead of compressing in-place.") + "\n\n")
	}

	// Conditional fields — shown only when Mod Output Mode is on.
	if m.modOutputMode {
		modOutputNameLabel := style.StyleBody.Render("Output Mod Name")
		if m.focused == fieldModOutputName {
			modOutputNameLabel = style.StyleSelected.Render("Output Mod Name")
		}
		b.WriteString(modOutputNameLabel + "\n" + m.inputs[4].View() + "\n")
		b.WriteString(style.StyleMuted.Render("Name of the output mod folder created inside your mods directory.") + "\n")
		outputFolder := filepath.Join(m.cfg.ModsDir, m.inputs[4].Value())
		b.WriteString(style.StyleMuted.Render("Output folder: "+outputFolder) + "\n")
		b.WriteString(style.StyleMuted.Render("Delete this folder to force recompression on next run.") + "\n\n")

		modlistLabel := style.StyleBody.Render("MO2 modlist.txt Path")
		if m.focused == fieldModlistPath {
			modlistLabel = style.StyleSelected.Render("MO2 modlist.txt Path")
		}
		b.WriteString(modlistLabel + "\n" + m.inputs[5].View() + "\n")
		b.WriteString(style.StyleMuted.Render("Full path to your MO2 profile's modlist.txt. Leave empty to scan all mods without priority merging.") + "\n\n")
	}

	var footer strings.Builder
	if m.errMsg != "" {
		footer.WriteString(style.StyleDanger.Render(m.errMsg) + "\n\n")
	}
	footer.WriteString(style.KeyHint("tab", "next") + "  ")
	footer.WriteString(style.KeyHint("space", "toggle") + "  ")
	footer.WriteString(style.KeyHint("enter", "save") + "  ")
	footer.WriteString(style.KeyHint("q", "cancel"))

	const bodyFooterSeparator = "\n\n"
	title := style.StyleTitle.Render("Settings") + "\n\n"
	bodyLines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")

	// Reserve room for everything that isn't the scrollable body: title lines,
	// the hardcoded blank-line separator before the footer, and the footer
	// itself. Must match the final concatenation below exactly — undercounting
	// here means the assembled string is taller than the terminal, which
	// silently scrolls the title (the very first lines) off the top instead
	// of clipping the body like it's supposed to. m.height is 0 until the
	// first WindowSizeMsg arrives — show everything unclipped rather than
	// guessing a height.
	if m.height > 0 {
		reserved := strings.Count(title, "\n") + strings.Count(bodyFooterSeparator, "\n") + strings.Count(footer.String(), "\n")
		avail := m.height - reserved
		bodyLines = scrollToFocused(bodyLines, m.focused, avail)
	}

	return title + strings.Join(bodyLines, "\n") + bodyFooterSeparator + footer.String()
}

func (m *SettingsModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}
