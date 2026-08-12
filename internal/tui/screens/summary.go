package screens

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/noisethanks/atak/internal/compress"
	"github.com/noisethanks/atak/internal/tui/style"
)

// summaryFocus identifies which navigable list has keyboard focus. Errors and
// Backend-fallbacks are separate sections with separate cursors — a fallback
// succeeded and shouldn't share the failure list's semantics — but the same
// j/k navigation applies to whichever is focused. Tab toggles.
type summaryFocus int

const (
	focusErrors summaryFocus = iota
	focusFallbacks
)

// SummaryModel shows compression stats, per-file errors, and per-file backend
// fallbacks (files that succeeded via a non-primary backend or format).
type SummaryModel struct {
	data           SummaryData
	errorCursor    int
	fallbackCursor int
	focus          summaryFocus
	width          int
	height         int
}

func NewSummary(data SummaryData) SummaryModel {
	// Default focus to whichever section has content; prefer errors when both
	// have entries since failures usually need attention first.
	focus := focusErrors
	if len(data.Errors) == 0 && len(data.Fallbacks) > 0 {
		focus = focusFallbacks
	}
	return SummaryModel{data: data, focus: focus}
}

func (m SummaryModel) Init() tea.Cmd { return nil }

func (m SummaryModel) Update(msg tea.Msg) (SummaryModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.moveCursor(-1)
		case "down", "j":
			m.moveCursor(+1)
		case "tab":
			m.toggleFocus()
		case "enter", "m":
			return m, func() tea.Msg { return NavigateMsg{To: NavMenu} }
		case "r", "q":
			return m, func() tea.Msg { return NavigateMsg{To: NavResults} }
		case "ctrl+c":
			return m, func() tea.Msg { return NavigateMsg{To: NavQuit} }
		}
	}
	return m, nil
}

func (m *SummaryModel) moveCursor(delta int) {
	if m.focus == focusErrors {
		m.errorCursor = clampCursor(m.errorCursor+delta, len(m.data.Errors))
	} else {
		m.fallbackCursor = clampCursor(m.fallbackCursor+delta, len(m.data.Fallbacks))
	}
}

func (m *SummaryModel) toggleFocus() {
	// Only toggle when the target section has content — otherwise focus would
	// disappear into an empty pane and keys would silently no-op.
	if m.focus == focusErrors && len(m.data.Fallbacks) > 0 {
		m.focus = focusFallbacks
		return
	}
	if m.focus == focusFallbacks && len(m.data.Errors) > 0 {
		m.focus = focusErrors
	}
}

func clampCursor(v, n int) int {
	if v < 0 || n == 0 {
		return 0
	}
	if v >= n {
		return n - 1
	}
	return v
}

func (m SummaryModel) View() string {
	d := m.data
	var b strings.Builder
	b.WriteString(style.StyleTitle.Render("Done") + "\n\n")

	if d.OutputSkipped > 0 && d.Succeeded == 0 {
		b.WriteString(style.StyleWarning.Render("Nothing to compress — all files already exist in output folder.") + "\n")
		if d.OutputIsPattern {
			b.WriteString(style.StyleWarning.Render(fmt.Sprintf("Delete the existing output folders matching %s to force recompression.", d.OutputDir)) + "\n\n")
		} else {
			b.WriteString(style.StyleWarning.Render(fmt.Sprintf("Delete %s to force recompression.", d.OutputDir)) + "\n\n")
		}
	}

	b.WriteString(style.StyleSuccess.Render(fmt.Sprintf("✓  %d succeeded", d.Succeeded)) + "\n")
	if d.OutputSkipped > 0 {
		b.WriteString(style.StyleMuted.Render(fmt.Sprintf("⊘  %d already in output folder", d.OutputSkipped)) + "\n")
	}
	if d.Failed > 0 {
		b.WriteString(style.StyleDanger.Render(fmt.Sprintf("✗  %d failed", d.Failed)) + "\n")
	}
	// Per-reason counts: one line per fallback that fired at least once. A
	// clean run adds nothing here — no "0 fallbacks" noise.
	for _, reason := range sortedReasons(d.FallbackCounts) {
		b.WriteString(style.StyleWarning.Render(fmt.Sprintf(
			"↷  %d %s", d.FallbackCounts[reason], compress.FallbackLabel(reason),
		)) + "\n")
	}
	b.WriteString("\n")

	if d.TotalBefore > 0 {
		saved := d.TotalBefore - d.TotalAfter
		pct := float64(saved) / float64(d.TotalBefore) * 100
		b.WriteString(style.StyleBody.Render(fmt.Sprintf(
			"VRAM delta:  %s → %s  (saved %s, %.1f%%)",
			formatBytes(d.TotalBefore),
			formatBytes(d.TotalAfter),
			formatBytes(saved),
			pct,
		)) + "\n\n")
	}

	if len(d.Errors) > 0 {
		renderSectionList(&b, "Errors", d.Errors, m.errorCursor, m.focus == focusErrors, style.StyleDanger, style.StyleErrorItem)
	}
	if len(d.Fallbacks) > 0 {
		// Rendered separately from Errors and with StyleWarning (not Danger)
		// because a fallback succeeded — user just needs to know a
		// non-primary backend/format handled it.
		renderSectionList(&b, "Backend fallbacks", d.Fallbacks, m.fallbackCursor, m.focus == focusFallbacks, style.StyleWarning, style.StyleMuted)
	}

	if len(d.Errors) > 0 && len(d.Fallbacks) > 0 {
		b.WriteString(style.KeyHint("tab", "switch section") + "  ")
	}
	b.WriteString(style.KeyHint("m / enter", "main menu") + "  ")
	b.WriteString(style.KeyHint("r / q", "back to results"))
	return b.String()
}

// renderSectionList emits a header + windowed slice of items. focused=true
// gets an accent marker on the header so the user can tell which cursor
// tab/j/k will move; keeps existing 10-item windowing behavior.
func renderSectionList(b *strings.Builder, title string, items []string, cursor int, focused bool, headerStyle, itemStyle interface{ Render(...string) string }) {
	marker := "  "
	if focused {
		marker = "▸ "
	}
	b.WriteString(headerStyle.Render(fmt.Sprintf("%s%s (%d):", marker, title, len(items))) + "\n")
	visible := items
	if len(visible) > 10 {
		end := cursor + 10
		if end > len(visible) {
			end = len(visible)
		}
		visible = visible[cursor:end]
	}
	for _, e := range visible {
		b.WriteString(itemStyle.Render("  • "+e) + "\n")
	}
	b.WriteString("\n")
}

func sortedReasons(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (m *SummaryModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func formatBytes(n int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.2f GB", float64(n)/GB)
	case n >= MB:
		return fmt.Sprintf("%.1f MB", float64(n)/MB)
	case n >= KB:
		return fmt.Sprintf("%.0f KB", float64(n)/KB)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
