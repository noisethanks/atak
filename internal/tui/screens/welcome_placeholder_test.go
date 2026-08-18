package screens

import (
	"runtime"
	"strings"
	"testing"
)

func TestWelcomePlaceholders(t *testing.T) {
	tests := []struct {
		goos    string
		wantSep string // path separator style expected in both placeholders
		badSep  string // separator style that must NOT appear
	}{
		{goos: "windows", wantSep: `\`, badSep: "/"},
		{goos: "linux", wantSep: "/", badSep: `\`},
		{goos: "darwin", wantSep: "/", badSep: `\`},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			mods, backup := welcomePlaceholders(tt.goos)

			if mods == "" || backup == "" {
				t.Fatalf("welcomePlaceholders(%q) returned an empty placeholder: mods=%q backup=%q", tt.goos, mods, backup)
			}
			if !strings.Contains(mods, tt.wantSep) {
				t.Errorf("welcomePlaceholders(%q) mods = %q, want it to use %q-style separators", tt.goos, mods, tt.wantSep)
			}
			if strings.Contains(mods, tt.badSep) {
				t.Errorf("welcomePlaceholders(%q) mods = %q, should not contain %q-style separators", tt.goos, mods, tt.badSep)
			}
			if !strings.Contains(backup, tt.wantSep) {
				t.Errorf("welcomePlaceholders(%q) backup = %q, want it to use %q-style separators", tt.goos, backup, tt.wantSep)
			}

			// The mods placeholder should reflect the GAMMA directory
			// convention, not the older Anomaly-only one.
			if !strings.Contains(strings.ToLower(mods), "gamma") {
				t.Errorf("welcomePlaceholders(%q) mods = %q, want it to reference the GAMMA directory convention", tt.goos, mods)
			}
			if strings.Contains(strings.ToLower(mods), "anomaly") {
				t.Errorf("welcomePlaceholders(%q) mods = %q, should not reference Anomaly directly — GAMMA is the convention", tt.goos, mods)
			}
		})
	}
}

// TestNewWelcomeWithNotice_UsesHostPlaceholders is a light integration check
// that the constructor actually wires welcomePlaceholders(runtime.GOOS) into
// the two text inputs, so a future edit to NewWelcomeWithNotice can't drift
// from the pure function silently.
func TestNewWelcomeWithNotice_UsesHostPlaceholders(t *testing.T) {
	wantMods, wantBackup := welcomePlaceholders(runtime.GOOS)

	cfg := minConfig("", "")
	m := NewWelcomeWithNotice(cfg, "")

	if got := m.modsInput.Placeholder; got != wantMods {
		t.Errorf("modsInput.Placeholder = %q, want %q", got, wantMods)
	}
	if got := m.bkupInput.Placeholder; got != wantBackup {
		t.Errorf("bkupInput.Placeholder = %q, want %q", got, wantBackup)
	}
}
