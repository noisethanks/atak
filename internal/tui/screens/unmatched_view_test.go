package screens

import (
	"strings"
	"testing"

	"github.com/noisethanks/atak/internal/scan"
)

func TestRenderSectionListSelectedRow(t *testing.T) {
	assets := []scan.Asset{
		{Path: "/mods/MyMod/gamedata/textures/foo/bar/texture_d.dds", ModName: "MyMod", ProfileMatch: "Unmatched", CurrentFmt: "DXT1", Width: 512, Height: 512, SourceMipCount: 1},
		{Path: "/mods/MyMod/gamedata/textures/other.dds", ModName: "MyMod", ProfileMatch: "Unmatched", CurrentFmt: "DXT5", Width: 1024, Height: 1024, SourceMipCount: 4},
		{Path: "/mods/AnotherMod/gamedata/textures/ui/icon.dds", ModName: "AnotherMod", ProfileMatch: "Unmatched", CurrentFmt: "A8R8G8B8", Width: 256, Height: 256, SourceMipCount: 1},
	}
	m := NewUnmatched(assets)
	m.SetSize(120, 40)

	view := m.View()
	if !strings.Contains(view, "▶") {
		t.Error("▶ marker not found in Unmatched view — top row not highlighted")
	}

	// cursor=0 detail should describe MyMod
	if !strings.Contains(view, "MyMod") {
		t.Error("detail panel missing mod name")
	}

	// After j, cursor=1, detail should still show MyMod (second asset)
	m2 := m
	m2.cursor = 1
	view2 := m2.View()
	if !strings.Contains(view2, "▶") {
		t.Error("▶ marker missing after cursor advance")
	}
}

func TestRenderSectionListBulletForNonSelected(t *testing.T) {
	var b strings.Builder
	items := []string{"alpha", "beta", "gamma"}
	// cursor=0: first item selected, rest bullets
	renderSectionList(&b, "Test", items, 0, true, noopStyle{}, noopStyle{})
	out := b.String()
	
	lines := strings.Split(out, "\n")
	var selectedLines, bulletLines []string
	for _, l := range lines {
		s := stripTestANSI(l)
		if strings.Contains(s, "▶ alpha") {
			selectedLines = append(selectedLines, l)
		}
		if strings.HasPrefix(strings.TrimSpace(s), "•") {
			bulletLines = append(bulletLines, l)
		}
	}
	if len(selectedLines) != 1 {
		t.Errorf("want 1 selected row, got %d; output:\n%s", len(selectedLines), out)
	}
	if len(bulletLines) != 2 {
		t.Errorf("want 2 bullet rows (beta, gamma), got %d; output:\n%s", len(bulletLines), out)
	}
}

type noopStyle struct{}
func (noopStyle) Render(s ...string) string { return strings.Join(s, "") }

func stripTestANSI(s string) string {
	var out strings.Builder
	inESC := false
	for _, r := range s {
		if r == '\x1b' { inESC = true; continue }
		if inESC { if r == 'm' { inESC = false }; continue }
		out.WriteRune(r)
	}
	return out.String()
}
