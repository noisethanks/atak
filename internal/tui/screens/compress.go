package screens

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/noisethanks/atak/internal/compress"
	"github.com/noisethanks/atak/internal/config"
	"github.com/noisethanks/atak/internal/scan"
	"github.com/noisethanks/atak/internal/tools"
	"github.com/noisethanks/atak/internal/tui/components"
	"github.com/noisethanks/atak/internal/tui/style"
)

// compressReadyMsg is returned once RunPool and channel bridges are set up.
type compressReadyMsg struct {
	opCh  <-chan components.OperationProgressMsg
	sumCh <-chan SummaryData
}

// CompressModel shows a shared OperationScreen during compression.
type CompressModel struct {
	data        CompressJobData
	cfg         *config.Config
	tools       *tools.EmbeddedTools
	ctx         context.Context
	cancel      context.CancelFunc
	total       int
	backendName string // display label — includes CPU/GPU hint so mid-run timing expectations are legible
	opScreen    components.OperationScreen
	summaryCh   <-chan SummaryData
	ready       bool
	width       int
	height      int
}

func NewCompress(data CompressJobData, cfg *config.Config, t *tools.EmbeddedTools) CompressModel {
	ctx, cancel := context.WithCancel(context.Background())
	total := 0
	for _, g := range data.Groups {
		total += len(g.Paths)
	}
	return CompressModel{
		data:        data,
		cfg:         cfg,
		tools:       t,
		ctx:         ctx,
		cancel:      cancel,
		total:       total,
		backendName: backendDisplayName(cfg, t),
	}
}

// backendDisplayName resolves which backend the run will actually use (falling
// back to texconv when compressonator was selected but isn't available on this
// platform) and returns a compact label for the run status line. The CPU/GPU
// hint matters because texconv is GPU-accelerated for BC7 on Windows but not
// Linux — a user seeing a slow BC7 run should be able to tell at a glance
// whether they're on the CPU path.
func backendDisplayName(cfg *config.Config, t *tools.EmbeddedTools) string {
	name := cfg.CompressionBackend
	if name == config.BackendCompressonatorBc7e && t.CompressonatorPath == "" {
		name = config.BackendTexconv
	}
	if name == "" {
		name = config.BackendTexconv
	}
	switch name {
	case config.BackendCompressonatorBc7e:
		return "compressonator-bc7e (CPU)"
	case config.BackendTexconv:
		if runtime.GOOS == "windows" {
			return "texconv (GPU for BC7)"
		}
		return "texconv (CPU)"
	}
	return name
}

func (m CompressModel) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return OperationStartedMsg{Cancel: m.cancel} },
		m.startCompression(),
	)
}

func (m CompressModel) startCompression() tea.Cmd {
	data := m.data
	t := m.tools
	cfg := m.cfg
	ctx := m.ctx
	total := m.total
	return func() tea.Msg {
		jobs := buildJobs(data)
		primary, fallback := selectBackends(cfg, t)
		resultCh := compress.RunPool(ctx, primary, fallback, jobs, data.WorkerCount)
		opCh, sumCh := compressToOpCh(resultCh, total, data.ModOutputDir)
		return compressReadyMsg{opCh: opCh, sumCh: sumCh}
	}
}

// selectBackends picks the primary backend from cfg and, when primary is
// compressonator, also supplies a texconv fallback for the maxTextureSize
// resize case (compressonator has no exact-size resize flag). When
// compressonator was selected but its binary isn't extracted, fall back
// cleanly to texconv rather than erroring — the fallback path is already the
// safe choice.
func selectBackends(cfg *config.Config, t *tools.EmbeddedTools) (primary, fallback compress.Backend) {
	tex := compress.NewTexconvBackend(t.TexconvPath)
	if cfg.CompressionBackend == config.BackendCompressonatorBc7e && t.CompressonatorPath != "" {
		return compress.NewCompressonatorBackend(t.CompressonatorPath), tex
	}
	return tex, nil
}

func (m CompressModel) Update(msg tea.Msg) (CompressModel, tea.Cmd) {
	if m.ready {
		var cmd tea.Cmd
		m.opScreen, cmd = m.opScreen.Update(msg)
		if m.opScreen.IsDone() {
			sumCh := m.summaryCh
			return m, func() tea.Msg {
				data := <-sumCh
				return NavigateMsg{To: NavSummary, Data: data}
			}
		}
		return m, cmd
	}

	if msg, ok := msg.(compressReadyMsg); ok {
		title := "Compressing Textures…  ·  Backend: " + m.backendName
		m.opScreen = components.NewOperationScreen(title, msg.opCh, m.cancel)
		m.opScreen.SetSize(m.width, m.height)
		m.summaryCh = msg.sumCh
		m.ready = true
		return m, m.opScreen.Init()
	}

	return m, nil
}

func (m CompressModel) View() string {
	if !m.ready {
		return style.StyleTitle.Render("Compressing Textures") + "\n\nStarting…"
	}
	return m.opScreen.View()
}

func (m *CompressModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.opScreen.SetSize(w, h)
}

// compressToOpCh bridges compress.CompressionResult into OperationProgressMsg.
// It accumulates per-file stats and errors, sends progress on opCh, and delivers
// SummaryData on sumCh before closing opCh (guaranteeing sumCh is readable on Done).
func compressToOpCh(
	resultCh <-chan compress.CompressionResult,
	total int,
	modOutputDir string,
) (<-chan components.OperationProgressMsg, <-chan SummaryData) {
	opCh := make(chan components.OperationProgressMsg, 32)
	sumCh := make(chan SummaryData, 1)
	go func() {
		var done, succeeded, failed, outputSkipped int
		var totalBefore, totalAfter int64
		var errors, fallbacks []string
		fallbackCounts := map[string]int{}
		for r := range resultCh {
			done++
			totalBefore += r.Before
			totalAfter += r.After
			switch {
			case r.OutputSkipped:
				outputSkipped++
			case r.Success:
				succeeded++
				if r.FallbackReason != "" {
					fallbackCounts[r.FallbackReason]++
					line := fmt.Sprintf("%s\n  reason: %s", filepath.Base(r.Asset.Path), compress.FallbackLabel(r.FallbackReason))
					if r.Stderr != "" {
						line += "\n  " + strings.TrimSpace(r.Stderr)
					}
					fallbacks = append(fallbacks, line)
				}
			default:
				failed++
				errLine := fmt.Sprintf("%s: %v", filepath.Base(r.Asset.Path), r.Err)
				if r.Stderr != "" {
					errLine += "\n  " + strings.TrimSpace(r.Stderr)
				}
				errors = append(errors, errLine)
			}
			saved := totalBefore - totalAfter
			if saved < 0 {
				saved = 0
			}
			opCh <- components.OperationProgressMsg{
				Percent:       done * 100 / max(1, total),
				Status:        filepath.Base(r.Asset.Path),
				Size:          saved,
				OutputSkipped: outputSkipped,
			}
		}
		sumCh <- SummaryData{
			Succeeded:      succeeded,
			Failed:         failed,
			OutputSkipped:  outputSkipped,
			OutputDir:      modOutputDir,
			TotalBefore:    totalBefore,
			TotalAfter:     totalAfter,
			Errors:         errors,
			FallbackCounts: fallbackCounts,
			Fallbacks:      fallbacks,
		}
		close(opCh)
	}()
	return opCh, sumCh
}

// buildJobs converts ConfiguredGroups into compress.Jobs.
func buildJobs(data CompressJobData) []compress.Job {
	var jobs []compress.Job
	for _, g := range data.Groups {
		for i, path := range g.Paths {
			// GenerateMips is resolved per file and parallel to Paths; default to a full
			// chain if the slices ever fall out of sync (safe for world textures).
			genMips := true
			if i < len(g.GenerateMips) {
				genMips = g.GenerateMips[i]
			}
			var w, h int
			if i < len(g.Widths) {
				w = g.Widths[i]
			}
			if i < len(g.Heights) {
				h = g.Heights[i]
			}
			job := compress.Job{
				Asset:          scan.Asset{Path: path, Width: w, Height: h},
				Format:         g.Format,
				GenerateMips:   genMips,
				MaxTextureSize: g.MaxTextureSize,
				OutputDir:      filepath.Dir(path),
			}
			if data.ModOutputDir != "" && i < len(g.RelPaths) && g.RelPaths[i] != "" {
				job.RelPath = g.RelPaths[i]
				job.ModOutputDir = data.ModOutputDir
			}
			jobs = append(jobs, job)
		}
	}
	return jobs
}
