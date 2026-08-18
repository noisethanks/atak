package screens

import (
	"strings"
	"testing"
)

func TestMenuView_ShowsFontSizeTip(t *testing.T) {
	m := NewMenu()
	view := m.View()

	const want = "Ctrl"
	if !strings.Contains(view, want) {
		t.Errorf("menu view does not mention Ctrl +/- font size tip:\n%s", view)
	}
	if !strings.Contains(view, "font size") {
		t.Errorf("menu view does not mention font size:\n%s", view)
	}

	// The tip should render after the key legend, not before it — it's a
	// supplementary note, not part of the primary navigation hints.
	legendIdx := strings.Index(view, "navigate")
	tipIdx := strings.Index(view, "font size")
	if legendIdx == -1 {
		t.Fatalf("menu view does not contain the key legend at all:\n%s", view)
	}
	if tipIdx < legendIdx {
		t.Errorf("font size tip appears before the key legend, expected it after")
	}
}
