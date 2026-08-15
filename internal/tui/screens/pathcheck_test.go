package screens

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/noisethanks/atak/internal/config"
	"github.com/noisethanks/atak/internal/tools"
)

// minConfig returns a Config with all required non-path fields populated.
func minConfig(modsDir, backupDir string) *config.Config {
	return &config.Config{
		ModsDir:            modsDir,
		BackupDir:          backupDir,
		WorkerCount:        1,
		BackupLevel:        6,
		ScanExclusions:     []string{},
		ModOutputMode:      false,
		ModOutputName:      "ATAK",
		ModlistPath:        "",
		CompressionBackend: config.BackendTexconv,
	}
}

// --- dirExists ---

func TestDirExists(t *testing.T) {
	dir := t.TempDir()

	if !dirExists(dir) {
		t.Error("existing dir should return true")
	}
	if dirExists("") {
		t.Error("empty string should return false")
	}
	if dirExists("/definitely/does/not/exist/xyzzy") {
		t.Error("nonexistent path should return false")
	}

	// File path (not a directory) must return false.
	f, _ := os.CreateTemp(dir, "file-*")
	f.Close()
	if dirExists(f.Name()) {
		t.Error("file path should return false (not a directory)")
	}

	// Removed directory must return false.
	gone := t.TempDir()
	os.Remove(gone)
	if dirExists(gone) {
		t.Error("removed dir should return false")
	}
}

// --- ScanModel ---

func TestScanNavWelcomeEmptyModsDir(t *testing.T) {
	cfg := minConfig("", "")
	m := NewScan(cfg, &tools.EmbeddedTools{})
	msg := m.startScan()()

	nav, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("expected NavigateMsg, got %T", msg)
	}
	if nav.To != NavWelcome {
		t.Errorf("expected NavWelcome, got %v", nav.To)
	}
	// Empty modsDir → no notice (first-run message, not a "bad path" message).
	notice, _ := nav.Data.(string)
	if notice != "" {
		t.Errorf("empty modsDir should produce empty notice, got %q", notice)
	}
}

func TestScanNavWelcomeNonexistentModsDir(t *testing.T) {
	dir := t.TempDir()
	os.Remove(dir) // simulate "was valid, now isn't"

	cfg := minConfig(dir, "")
	m := NewScan(cfg, &tools.EmbeddedTools{})
	msg := m.startScan()()

	nav, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("expected NavigateMsg, got %T", msg)
	}
	if nav.To != NavWelcome {
		t.Errorf("expected NavWelcome, got %v", nav.To)
	}
	notice, _ := nav.Data.(string)
	if !strings.Contains(notice, dir) {
		t.Errorf("notice must name the bad path %q, got %q", dir, notice)
	}
	if !strings.Contains(notice, "not found") {
		t.Errorf("notice must contain 'not found', got %q", notice)
	}
}

// Confirm empty and nonexistent cases produce distinct notices.
func TestScanNoticeDistinct(t *testing.T) {
	dir := t.TempDir()
	os.Remove(dir)

	emptyCfg := minConfig("", "")
	invalidCfg := minConfig(dir, "")

	emptyMsg := NewScan(emptyCfg, &tools.EmbeddedTools{}).startScan()()
	invalidMsg := NewScan(invalidCfg, &tools.EmbeddedTools{}).startScan()()

	emptyNav := emptyMsg.(NavigateMsg)
	invalidNav := invalidMsg.(NavigateMsg)

	emptyNotice, _ := emptyNav.Data.(string)
	invalidNotice, _ := invalidNav.Data.(string)

	if emptyNotice == invalidNotice {
		t.Errorf("empty and invalid modsDir must produce different notices, both got %q", emptyNotice)
	}
	if emptyNotice != "" {
		t.Errorf("empty modsDir notice should be empty string, got %q", emptyNotice)
	}
	if !strings.Contains(invalidNotice, dir) {
		t.Errorf("invalid modsDir notice should contain path, got %q", invalidNotice)
	}
}

// Valid modsDir must not route to NavWelcome.
func TestScanNoRouteWithValidModsDir(t *testing.T) {
	dir := t.TempDir()
	cfg := minConfig(dir, "")
	m := NewScan(cfg, &tools.EmbeddedTools{})
	// Cancel context immediately so the scan Cmd exits without blocking.
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.cancel()
	msg := m.startScan()()

	if nav, ok := msg.(NavigateMsg); ok && nav.To == NavWelcome {
		t.Error("valid modsDir must not route to NavWelcome")
	}
}

// --- BackupModel Init ---

func TestBackupInitNavWelcomeEmptyBackupDir(t *testing.T) {
	cfg := minConfig("", "")
	m := NewBackup(cfg, &tools.EmbeddedTools{})
	msg := m.Init()()

	nav, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("expected NavigateMsg, got %T", msg)
	}
	if nav.To != NavWelcome {
		t.Errorf("expected NavWelcome, got %v", nav.To)
	}
	notice, _ := nav.Data.(string)
	if notice != "" {
		t.Errorf("empty backupDir should produce empty notice, got %q", notice)
	}
}

func TestBackupInitNavWelcomeNonexistentBackupDir(t *testing.T) {
	dir := t.TempDir()
	os.Remove(dir)

	cfg := minConfig("", dir)
	m := NewBackup(cfg, &tools.EmbeddedTools{})
	msg := m.Init()()

	nav, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("expected NavigateMsg, got %T", msg)
	}
	if nav.To != NavWelcome {
		t.Errorf("expected NavWelcome, got %v", nav.To)
	}
	notice, _ := nav.Data.(string)
	if !strings.Contains(notice, dir) {
		t.Errorf("notice must name the bad path %q, got %q", dir, notice)
	}
	if !strings.Contains(notice, "not found") {
		t.Errorf("notice must contain 'not found', got %q", notice)
	}
}

// Confirm empty and nonexistent backupDir produce distinct notices.
func TestBackupInitNoticeDistinct(t *testing.T) {
	dir := t.TempDir()
	os.Remove(dir)

	emptyNav := NewBackup(minConfig("", ""), &tools.EmbeddedTools{}).Init()().(NavigateMsg)
	invalidNav := NewBackup(minConfig("", dir), &tools.EmbeddedTools{}).Init()().(NavigateMsg)

	emptyNotice, _ := emptyNav.Data.(string)
	invalidNotice, _ := invalidNav.Data.(string)

	if emptyNotice == invalidNotice {
		t.Errorf("empty and invalid backupDir must produce different notices, both got %q", emptyNotice)
	}
	if emptyNotice != "" {
		t.Errorf("empty backupDir notice should be empty string, got %q", emptyNotice)
	}
	if !strings.Contains(invalidNotice, dir) {
		t.Errorf("invalid backupDir notice should contain path, got %q", invalidNotice)
	}
}

// --- BackupModel Init: ModsDir checks ---

func TestBackupInitNavWelcomeEmptyModsDir(t *testing.T) {
	backupDir := t.TempDir()
	cfg := minConfig("", backupDir)
	m := NewBackup(cfg, &tools.EmbeddedTools{})
	msg := m.Init()()

	nav, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("expected NavigateMsg, got %T", msg)
	}
	if nav.To != NavWelcome {
		t.Errorf("expected NavWelcome, got %v", nav.To)
	}
	notice, _ := nav.Data.(string)
	if notice != "" {
		t.Errorf("empty modsDir should produce empty notice, got %q", notice)
	}
}

func TestBackupInitNavWelcomeNonexistentModsDir(t *testing.T) {
	backupDir := t.TempDir()
	modsDir := t.TempDir()
	os.Remove(modsDir)

	cfg := minConfig(modsDir, backupDir)
	m := NewBackup(cfg, &tools.EmbeddedTools{})
	msg := m.Init()()

	nav, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("expected NavigateMsg, got %T", msg)
	}
	if nav.To != NavWelcome {
		t.Errorf("expected NavWelcome, got %v", nav.To)
	}
	notice, _ := nav.Data.(string)
	if !strings.Contains(notice, modsDir) {
		t.Errorf("notice must name the bad modsDir %q, got %q", modsDir, notice)
	}
	if !strings.Contains(notice, "not found") {
		t.Errorf("notice must contain 'not found', got %q", notice)
	}
}

// --- BackupModel selectArchiveOrPick: Restore Single Mod worst case ---

// Scenario: empty modsDir → selecting Restore Single Mod routes to Welcome
// before archive picker, before ModPicker, before confirmation. This is the
// silent-extract-to-CWD bug confirmed during testing.
func TestBackupRestoreSingleEmptyModsDirRoutes(t *testing.T) {
	backupDir := t.TempDir()
	cfg := minConfig("", backupDir)
	m := NewBackup(cfg, &tools.EmbeddedTools{})
	// Simulate user selecting Restore Single Mod from the menu.
	_, cmd := m.selectArchiveOrPick(pendingRestoreSingle)
	msg := cmd()

	nav, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("expected NavigateMsg, got %T", msg)
	}
	if nav.To != NavWelcome {
		t.Errorf("expected NavWelcome, got %v", nav.To)
	}
	notice, _ := nav.Data.(string)
	if notice != "" {
		t.Errorf("empty modsDir should produce empty notice (first-run message), got %q", notice)
	}
}

// --- BackupModel selectArchiveOrPick: Restore All worst case ---

// Scenario: invalid (nonexistent) modsDir → Restore All routes to Welcome
// with a message naming the bad path. This prevented the silent filepath.Dir("")
// = "." extraction bug.
func TestBackupRestoreAllInvalidModsDirRoutes(t *testing.T) {
	backupDir := t.TempDir()
	modsDir := t.TempDir()
	os.Remove(modsDir)

	cfg := minConfig(modsDir, backupDir)
	m := NewBackup(cfg, &tools.EmbeddedTools{})
	_, cmd := m.selectArchiveOrPick(pendingRestoreAll)
	msg := cmd()

	nav, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("expected NavigateMsg, got %T", msg)
	}
	if nav.To != NavWelcome {
		t.Errorf("expected NavWelcome, got %v", nav.To)
	}
	notice, _ := nav.Data.(string)
	if !strings.Contains(notice, modsDir) {
		t.Errorf("notice must name the bad modsDir %q, got %q", modsDir, notice)
	}
}

// Verify and Delete do not depend on ModsDir; selectArchiveOrPick must NOT
// route to Welcome for them even when ModsDir is invalid.
func TestBackupVerifyDeleteNotBlockedByBadModsDir(t *testing.T) {
	backupDir := t.TempDir()
	modsDir := t.TempDir()
	os.Remove(modsDir)

	cfg := minConfig(modsDir, backupDir)
	m := NewBackup(cfg, &tools.EmbeddedTools{})

	for _, action := range []backupPendingAction{pendingVerify, pendingDelete} {
		_, cmd := m.selectArchiveOrPick(action)
		// cmd is nil when backups list is empty — that's the no-backups short-circuit,
		// not a NavWelcome route. Only NavWelcome would be wrong here.
		if cmd != nil {
			msg := cmd()
			if nav, ok := msg.(NavigateMsg); ok && nav.To == NavWelcome {
				t.Errorf("action %v must not route to NavWelcome on invalid modsDir (no ModsDir dependency)", action)
			}
		}
	}
}

// --- BackupModel startBackup ---

func TestBackupStartBackupNavWelcomeNonexistentModsDir(t *testing.T) {
	backupDir := t.TempDir()
	modsDir := t.TempDir()
	os.Remove(modsDir)

	cfg := minConfig(modsDir, backupDir)
	m := NewBackup(cfg, &tools.EmbeddedTools{})
	_, cmd := m.startBackup()
	msg := cmd()

	nav, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("expected NavigateMsg, got %T", msg)
	}
	if nav.To != NavWelcome {
		t.Errorf("expected NavWelcome, got %v", nav.To)
	}
	notice, _ := nav.Data.(string)
	if !strings.Contains(notice, modsDir) {
		t.Errorf("notice must name the bad modsDir %q, got %q", modsDir, notice)
	}
}
