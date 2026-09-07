package screens

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/noisethanks/atak/internal/scan"
	"github.com/noisethanks/atak/internal/tui/style"
)

// UnmatchedModel is read-only. Unmatched files bypass profiles.json by definition —
// adding a compress action here would recreate the removed Auto bucket's crash surface.
type UnmatchedModel struct {
	items  []assetRef
	cursor int
	width  int
	height int
}

func NewUnmatched(assets []scan.Asset) UnmatchedModel {
	var items []assetRef
	for _, a := range assets {
		if a.ProfileMatch == "Unmatched" || a.ProfileMatch == "" {
			items = append(items, assetRef{
				Path:           a.Path,
				ModName:        a.ModName,
				CurrentFmt:     a.CurrentFmt,
				Width:          a.Width,
				Height:         a.Height,
				SourceMipCount: a.SourceMipCount,
				VirtualRelPath: a.VirtualRelPath,
			})
		}
	}
	return UnmatchedModel{items: items}
}

func (m UnmatchedModel) Init() tea.Cmd { return nil }

func (m UnmatchedModel) Update(msg tea.Msg) (UnmatchedModel, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyMsg); ok {
		switch kmsg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "esc", "q":
			return m, func() tea.Msg { return NavigateMsg{To: NavResults} }
		}
	}
	return m, nil
}

func (m UnmatchedModel) View() string {
	var b strings.Builder
	b.WriteString(style.StyleTitle.Render("Unmatched Files") + "\n\n")

	if len(m.items) == 0 {
		b.WriteString(style.StyleMuted.Render("No unmatched files.") + "\n\n")
		b.WriteString(style.KeyHint("q", "back"))
		return b.String()
	}

	displayItems := make([]string, len(m.items))
	for i, item := range m.items {
		displayItems[i] = truncateAssetRow(item, m.width)
	}
	// Reuse the same j/k window-of-10 component used by the error list and
	// Backend fallbacks in the summary screen.
	renderSectionList(&b, "Unmatched", displayItems, m.cursor, true, style.StyleWarning, style.StyleMuted)

	// Full detail for the selected item (cursor = window start = selected).
	sel := m.items[m.cursor]
	rel := modRelPathFrom(sel)
	b.WriteString(style.StyleBody.Render(
		"  Mod:    "+sel.ModName+"\n"+
			"  Path:   "+rel+"\n"+
			"  Format: "+sel.CurrentFmt+"\n"+
			"  Size:   "+fmt.Sprintf("%dx%d", sel.Width, sel.Height)+"\n"+
			"  Mips:   "+fmt.Sprintf("%d", sel.SourceMipCount),
	) + "\n\n")

	b.WriteString(style.StyleMuted.Render("  Add matching patterns to profiles.json to compress these.") + "\n\n")
	b.WriteString(style.KeyHint("j/k", "navigate") + "  ")
	b.WriteString(style.KeyHint("q", "back"))
	return b.String()
}

func (m *UnmatchedModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// truncateAssetRow formats one list row. Mod name and filename stay visible;
// middle path segments are elided with "…" when the full string exceeds width.
func truncateAssetRow(ref assetRef, width int) string {
	rel := modRelPathFrom(ref)
	modName := ref.ModName
	if modName == "" {
		modName = "?"
	}
	full := modName + " / " + filepath.ToSlash(rel)

	const listPrefix = 4 // "  • " prepended by renderSectionList
	avail := width - listPrefix
	if avail <= 0 || len(full) <= avail {
		return full
	}

	filename := filepath.Base(rel)
	short := modName + " / … / " + filename
	if len(short) <= avail {
		return short
	}
	// Terminal too narrow even for modname+…+filename. Truncate mod name.
	minSuffix := "… / " + filename
	if avail <= len(minSuffix) {
		return minSuffix
	}
	return modName[:avail-len(minSuffix)] + minSuffix
}

// modRelPathFrom returns ref's path relative to its mod root.
// VirtualRelPath is used when set (Mod Output Mode). For in-place Walk scans,
// modName is the first directory segment under modsDir, so it appears as an
// exact directory boundary in the absolute path exactly once.
func modRelPathFrom(ref assetRef) string {
	if ref.VirtualRelPath != "" {
		return filepath.ToSlash(ref.VirtualRelPath)
	}
	if ref.ModName != "" {
		modMarker := string(filepath.Separator) + ref.ModName + string(filepath.Separator)
		if idx := strings.Index(ref.Path, modMarker); idx >= 0 {
			return filepath.ToSlash(ref.Path[idx+len(modMarker):])
		}
	}
	return filepath.Base(ref.Path)
}
