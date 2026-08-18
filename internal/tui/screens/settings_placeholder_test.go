package screens

import (
	"runtime"
	"strings"
	"testing"
)

func TestSettingsModlistPlaceholder(t *testing.T) {
	tests := []struct {
		goos    string
		wantSep string
		badSep  string
	}{
		{goos: "windows", wantSep: `\`, badSep: "/"},
		{goos: "linux", wantSep: "/", badSep: `\`},
		{goos: "darwin", wantSep: "/", badSep: `\`},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			got := settingsModlistPlaceholder(tt.goos)

			if got == "" {
				t.Fatalf("settingsModlistPlaceholder(%q) returned empty string", tt.goos)
			}
			if !strings.Contains(got, tt.wantSep) {
				t.Errorf("settingsModlistPlaceholder(%q) = %q, want it to use %q-style separators", tt.goos, got, tt.wantSep)
			}
			if strings.Contains(got, tt.badSep) {
				t.Errorf("settingsModlistPlaceholder(%q) = %q, should not contain %q-style separators", tt.goos, got, tt.badSep)
			}
			if !strings.HasSuffix(got, "modlist.txt") {
				t.Errorf("settingsModlistPlaceholder(%q) = %q, want it to end in modlist.txt", tt.goos, got)
			}
			if !strings.Contains(strings.ToLower(got), "gamma") {
				t.Errorf("settingsModlistPlaceholder(%q) = %q, want it to reference the GAMMA directory convention", tt.goos, got)
			}
		})
	}
}

// TestNewSettings_UsesHostModlistPlaceholder is a light integration check
// that NewSettings actually wires settingsModlistPlaceholder(runtime.GOOS)
// into the modlist input, so a future edit can't drift from the pure
// function silently.
func TestNewSettings_UsesHostModlistPlaceholder(t *testing.T) {
	want := settingsModlistPlaceholder(runtime.GOOS)

	cfg := minConfig("", "")
	m := NewSettings(cfg)

	if got := m.inputs[5].Placeholder; got != want {
		t.Errorf("inputs[5] (modlistPath) Placeholder = %q, want %q", got, want)
	}
}
