package screens

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/noisethanks/atak/internal/config"
	"github.com/noisethanks/atak/internal/modlist"
	"github.com/noisethanks/atak/internal/scan"
	"github.com/noisethanks/atak/internal/tools"
	"github.com/noisethanks/atak/internal/tui/style"
)

// assetFoundMsg carries one discovered asset and the channels to continue reading.
type assetFoundMsg struct {
	asset     scan.Asset
	ch        <-chan scan.Asset
	skippedCh <-chan int
}

// scanCompleteMsg signals the walker is finished.
type scanCompleteMsg struct {
	total   int
	skipped int
	err     string // non-empty aborts to error display without navigating to results
}

// ScanModel shows a live counter while the walker runs.
type ScanModel struct {
	cfg     *config.Config
	tools   *tools.EmbeddedTools
	spinner spinner.Model
	ctx     context.Context
	cancel  context.CancelFunc
	assets []scan.Asset
	found  int
	done   bool
	err     string
	width   int
	height  int
}

func NewScan(cfg *config.Config, t *tools.EmbeddedTools) ScanModel {
	ctx, cancel := context.WithCancel(context.Background())
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return ScanModel{cfg: cfg, tools: t, spinner: sp, ctx: ctx, cancel: cancel}
}

func (m ScanModel) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return OperationStartedMsg{Cancel: m.cancel, CancelMsg: "Scan cancelled"} },
		m.spinner.Tick,
		m.startScan(),
	)
}

func (m ScanModel) startScan() tea.Cmd {
	cfg := m.cfg
	ctx := m.ctx
	return func() tea.Msg {
		profiles, excludePatterns, minFileSize, _, err := config.LoadProfiles()
		if err != nil || len(profiles) == 0 {
			return scanCompleteMsg{total: 0}
		}

		const (
			modlistErrNoPath  = "⚠ Mod Output Mode is enabled but no modlist.txt path is configured.\n  Go to Settings to set your MO2 modlist.txt path,\n  or disable Mod Output Mode to compress in-place."
			modlistErrBadPath = "⚠ Mod Output Mode is enabled but modlist.txt could not be read.\n  Check the path in Settings, or disable Mod Output Mode to compress in-place."
		)

		if cfg.ModOutputMode {
			// Never silently fall back to in-place when Mod Output Mode is on.
			if cfg.ModlistPath == "" {
				return NavigateMsg{To: NavResults, Data: ScanResultData{ModlistError: modlistErrNoPath}}
			}
			modList, parseErr := modlist.ParseModList(cfg.ModlistPath)
			if parseErr != nil || len(modList) == 0 {
				return NavigateMsg{To: NavResults, Data: ScanResultData{ModlistError: modlistErrBadPath}}
			}
			// Auto-exclude the output folder so it's never scanned.
			exclusions := append(cfg.ScanExclusions, cfg.ModOutputName)
			if cfg.PerCategoryModOutput || cfg.PerModModOutput {
				exclusions = append(exclusions, cfg.ModOutputName+" - *")
			}
			// Filter excluded mods before building the virtual FS so they never
			// win conflicts — analogous to Walk's filepath.SkipDir on directories.
			// Only whole-mod (name) exclusions apply here; path patterns target
			// nested directories and are applied per file inside WalkVirtual.
			var filteredList []string
			for _, mod := range modList {
				if !scan.ExcludesMod(mod, exclusions) {
					filteredList = append(filteredList, mod)
				}
			}
			virtualFS, buildErr := modlist.BuildVirtualFS(cfg.ModsDir, filteredList)
			if buildErr != nil {
				return NavigateMsg{To: NavResults, Data: ScanResultData{ModlistError: modlistErrBadPath}}
			}
			ch, skippedCh, _ := scan.WalkVirtual(virtualFS, cfg.ModsDir, profiles, excludePatterns, exclusions, minFileSize)
			return readNextAsset(ctx, ch, skippedCh)
		}

		ch, skippedCh, _ := scan.Walk(cfg.ModsDir, profiles, excludePatterns, cfg.ScanExclusions, minFileSize)
		return readNextAsset(ctx, ch, skippedCh)
	}
}

func readNextAsset(ctx context.Context, ch <-chan scan.Asset, skippedCh <-chan int) tea.Msg {
	select {
	case <-ctx.Done():
		go func() { for range ch {} }() // drain so walker goroutine exits
		return scanCompleteMsg{}
	case asset, ok := <-ch:
		if !ok {
			return scanCompleteMsg{skipped: <-skippedCh}
		}
		return assetFoundMsg{asset: asset, ch: ch, skippedCh: skippedCh}
	}
}

func (m ScanModel) Update(msg tea.Msg) (ScanModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case assetFoundMsg:
		m.assets = append(m.assets, msg.asset)
		m.found++
		ch, skippedCh := msg.ch, msg.skippedCh
		ctx := m.ctx
		return m, func() tea.Msg { return readNextAsset(ctx, ch, skippedCh) }

	case scanCompleteMsg:
		m.done = true
		if msg.err != "" {
			m.err = msg.err
			return m, nil
		}
		if m.found == 0 {
			m.err = "No .dds files found in " + m.cfg.ModsDir
			return m, nil
		}
		assets, skipped := m.assets, msg.skipped
		return m, func() tea.Msg {
			return NavigateMsg{To: NavResults, Data: ScanResultData{Assets: assets, Skipped: skipped}}
		}
	}
	return m, nil
}

func (m ScanModel) View() string {
	var b strings.Builder
	b.WriteString(style.StyleTitle.Render("Scanning...") + "\n\n")
	b.WriteString(m.spinner.View() + "  ")
	b.WriteString(style.StyleBody.Render(fmt.Sprintf("Found %d textures", m.found)) + "\n\n")
	if m.err != "" {
		b.WriteString(style.StyleDanger.Render(m.err) + "\n")
	}
	b.WriteString(style.StyleMuted.Render(m.cfg.ModsDir) + "\n\n")
	b.WriteString(style.KeyHint("ctrl+c", "cancel"))
	return b.String()
}

func (m *ScanModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}
