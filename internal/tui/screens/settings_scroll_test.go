package screens

import (
	"strings"
	"testing"

	"github.com/noisethanks/atak/internal/config"
)

func makeLines(n int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = "line"
	}
	return lines
}

func TestScrollToFocused_FitsWithinAvail_ReturnsUnchanged(t *testing.T) {
	lines := makeLines(10)
	got := scrollToFocused(lines, fieldModsDir, 20)
	if len(got) != 10 {
		t.Fatalf("content shorter than avail should be returned unchanged, got %d lines", len(got))
	}
}

func TestScrollToFocused_TopFieldReachable(t *testing.T) {
	lines := makeLines(40)
	lines[0] = "Anomaly Mods Directory"
	// Focus starts deep in the list (simulating tab wrapping around to the
	// top field), matching the bug report: with no windowing, a top field
	// scrolled off by earlier fields was unreachable regardless of focus.
	got := scrollToFocused(lines, fieldModsDir, 10)
	found := false
	for _, l := range got {
		if l == "Anomaly Mods Directory" {
			found = true
			break
		}
	}
	if !found {
		t.Error("focused top-field label not present in windowed output — unreachable, same as the reported bug")
	}
}

func TestScrollToFocused_BottomFieldReachable(t *testing.T) {
	lines := makeLines(40)
	lines[39] = "MO2 modlist.txt Path"
	got := scrollToFocused(lines, fieldModlistPath, 10)
	found := false
	for _, l := range got {
		if l == "MO2 modlist.txt Path" {
			found = true
			break
		}
	}
	if !found {
		t.Error("focused bottom-field label not present in windowed output")
	}
}

func TestScrollToFocused_WindowNeverExceedsAvail(t *testing.T) {
	lines := makeLines(100)
	for _, f := range []settingsField{fieldModsDir, fieldStripMips, fieldModOutputMode, fieldModlistPath} {
		got := scrollToFocused(lines, f, 15)
		if len(got) != 15 {
			t.Errorf("field %v: window size = %d, want exactly avail (15)", f, len(got))
		}
	}
}

func TestScrollToFocused_ZeroAvail_ReturnsUnchanged(t *testing.T) {
	lines := makeLines(5)
	got := scrollToFocused(lines, fieldModsDir, 0)
	if len(got) != len(lines) {
		t.Error("avail<=0 (height not yet known) should not clip content")
	}
}

// TestSettingsView_NeverExceedsHeight guards the actual bug: scrollToFocused
// itself was correct, but View()'s reserved-lines accounting undercounted
// the hardcoded blank-line separator between the body and the footer by 2
// lines. On a terminal just barely too short for the content, that overflow
// scrolled the whole alt-screen buffer, pushing the title (the very first
// lines of output) off the top — exactly the "no title, just blank space"
// screenshot that surfaced this. This test renders the real View() output
// end to end and checks its total line count never exceeds m.height, for
// every field position and both with/without Mod Output Mode's extra
// conditional fields.
func TestSettingsView_NeverExceedsHeight(t *testing.T) {
	cfg := &config.Config{ModsDir: "/mods", BackupDir: "/backups", WorkerCount: 4, BackupLevel: 6}

	for _, modOutputMode := range []bool{false, true} {
		cfg := *cfg
		cfg.ModOutputMode = modOutputMode
		m := NewSettings(&cfg)
		m.SetSize(90, 15)

		for f := settingsField(0); f < fieldCount; f++ {
			if !m.isVisible(f) {
				continue
			}
			m.focused = f
			view := m.View()
			totalLines := strings.Count(view, "\n") + 1
			if totalLines > m.height {
				t.Errorf("modOutputMode=%v field=%v: rendered %d lines, terminal only has %d — title/top content will scroll off screen",
					modOutputMode, f, totalLines, m.height)
			}
			if !strings.Contains(view, "Settings") {
				t.Errorf("modOutputMode=%v field=%v: title \"Settings\" missing from rendered output entirely", modOutputMode, f)
			}
		}
	}
}
