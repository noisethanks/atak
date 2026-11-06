package screens

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/noisethanks/atak/internal/config"
)

// typeKeys sends s one key at a time the way a terminal delivers typing: a
// space arrives as KeySpace, everything else as a single-rune KeyRunes.
func typeKeys[M interface {
	Update(tea.Msg) (M, tea.Cmd)
}](m M, s string) M {
	for _, r := range s {
		k := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		if r == ' ' {
			k = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		}
		m, _ = m.Update(k)
	}
	return m
}

// paste delivers s as one bracketed-paste key message, as Windows Terminal does
// for a dragged-in file.
func paste[M interface {
	Update(tea.Msg) (M, tea.Cmd)
}](m M, s string) M {
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s), Paste: true})
	return m
}

func key[M interface {
	Update(tea.Msg) (M, tea.Cmd)
}](m M, t tea.KeyType) (M, tea.Cmd) {
	return m.Update(tea.KeyMsg{Type: t})
}

// savedConfig runs the Cmd a save returns and extracts the config it carries.
func savedConfig(t *testing.T, cmd tea.Cmd) *config.Config {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command returned; expected NavSaveConfig")
	}
	nav, ok := cmd().(NavigateMsg)
	if !ok || nav.To != NavSaveConfig {
		t.Fatalf("expected NavSaveConfig, got %#v", nav)
	}
	return nav.Data.(*config.Config)
}

// Paths typed key by key. They include spaces and the letter q, both of which
// the screens bind to actions on non-text rows.
var (
	typedMods    = filepath.Join("Games", "Anomaly GAMMA", "mods")
	typedBackup  = filepath.Join("Backups", "atak quick saves")
	typedModlist = filepath.Join("GAMMA", "profiles", "My Quest Profile", "modlist.txt")
)

func settingsWithModOutput() SettingsModel {
	cfg := minConfig("", "")
	cfg.ModOutputMode = true
	return NewSettings(cfg)
}

// focusSettings moves focus to field f with tab, the way a user gets there.
func focusSettings(t *testing.T, m SettingsModel, f settingsField) SettingsModel {
	t.Helper()
	for i := 0; m.focused != f; i++ {
		if i > int(fieldCount) {
			t.Fatalf("could not tab to field %d", f)
		}
		m, _ = key(m, tea.KeyTab)
	}
	return m
}

func TestSettingsTypedPathsKeepSpaces(t *testing.T) {
	m := settingsWithModOutput()
	stripBefore, backendBefore := m.stripMips, m.compressionBackend

	m = typeKeys(focusSettings(t, m, fieldModsDir), typedMods)
	m = typeKeys(focusSettings(t, m, fieldBackupDir), typedBackup)
	m = typeKeys(focusSettings(t, m, fieldModlistPath), typedModlist)
	_, cmd := key(m, tea.KeyEnter)
	cfg := savedConfig(t, cmd)

	if cfg.ModsDir != typedMods {
		t.Errorf("ModsDir = %q, typed %q", cfg.ModsDir, typedMods)
	}
	if cfg.BackupDir != typedBackup {
		t.Errorf("BackupDir = %q, typed %q", cfg.BackupDir, typedBackup)
	}
	if cfg.ModlistPath != typedModlist {
		t.Errorf("ModlistPath = %q, typed %q", cfg.ModlistPath, typedModlist)
	}
	// Spaces typed into text fields must not reach the toggle rows.
	if cfg.StripMipsWhenDisabled != stripBefore || cfg.CompressionBackend != backendBefore || !cfg.ModOutputMode {
		t.Errorf("typing toggled a setting: strip=%v backend=%q modOutput=%v", cfg.StripMipsWhenDisabled, cfg.CompressionBackend, cfg.ModOutputMode)
	}
}

// Toggle keys still work on toggle rows.
func TestSettingsSpaceStillTogglesToggleRows(t *testing.T) {
	m := settingsWithModOutput()
	strip := m.stripMips
	m = typeKeys(focusSettings(t, m, fieldStripMips), " ")
	if m.stripMips == strip {
		t.Error("space did not toggle Strip Mips")
	}
	backend := m.compressionBackend
	m = typeKeys(focusSettings(t, m, fieldCompressionBackend), " ")
	if m.compressionBackend == backend {
		t.Error("space did not switch the backend")
	}
	m = focusSettings(t, m, fieldModOutputMode)
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}); cmd == nil {
		t.Error("q on a toggle row should leave Settings")
	}
}

func TestSettingsPastedQuotedPath(t *testing.T) {
	m := settingsWithModOutput()
	m = paste(focusSettings(t, m, fieldModlistPath), `"`+typedModlist+`"`)
	m = paste(focusSettings(t, m, fieldModsDir), `'`+typedMods+`' `)
	_, cmd := key(m, tea.KeyEnter)
	cfg := savedConfig(t, cmd)
	if cfg.ModlistPath != typedModlist {
		t.Errorf("ModlistPath = %q, want %q", cfg.ModlistPath, typedModlist)
	}
	if cfg.ModsDir != typedMods {
		t.Errorf("ModsDir = %q, want %q", cfg.ModsDir, typedMods)
	}
}

func TestWelcomeTypedPathsKeepSpaces(t *testing.T) {
	m := NewWelcome(minConfig("", ""))
	m = typeKeys(m, typedMods)
	m, _ = key(m, tea.KeyTab)
	m = typeKeys(m, typedBackup)
	_, cmd := key(m, tea.KeyEnter)
	cfg := savedConfig(t, cmd)
	if cfg.ModsDir != typedMods {
		t.Errorf("ModsDir = %q, typed %q", cfg.ModsDir, typedMods)
	}
	if cfg.BackupDir != typedBackup {
		t.Errorf("BackupDir = %q, typed %q", cfg.BackupDir, typedBackup)
	}
}

func TestWelcomePastedQuotedPath(t *testing.T) {
	m := NewWelcome(minConfig("", ""))
	m = paste(m, `"`+typedMods+`"`)
	m, _ = key(m, tea.KeyTab)
	m = paste(m, ` "`+typedBackup+`"`)
	_, cmd := key(m, tea.KeyEnter)
	cfg := savedConfig(t, cmd)
	if cfg.ModsDir != typedMods {
		t.Errorf("ModsDir = %q, want %q", cfg.ModsDir, typedMods)
	}
	if cfg.BackupDir != typedBackup {
		t.Errorf("BackupDir = %q, want %q", cfg.BackupDir, typedBackup)
	}
}
