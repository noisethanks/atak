package screens

import "context"

// OperationStartedMsg is sent by operational screens when a long-running process begins.
// AppModel stores Cancel so Ctrl+C can abort the operation cleanly.
type OperationStartedMsg struct {
	Cancel    context.CancelFunc
	CancelMsg string // shown on menu after cancel; defaults to "Operation cancelled" if empty
}

// NavTarget identifies where a NavigateMsg should take the app.
type NavTarget int

const (
	NavMenu NavTarget = iota
	NavScan
	NavResults
	NavCompress
	NavSummary
	NavBackup
	NavSettings
	NavAbout
	NavSaveConfig
	NavQuit
)

// NavigateMsg is sent by any sub-screen to request a screen transition.
// Data carries screen-specific payload (asset slice, job data, etc.).
type NavigateMsg struct {
	To   NavTarget
	Data any
}

// AssetGroup holds assets sharing the same compression profile.
type AssetGroup struct {
	ProfileName  string
	SuggestedFmt string
	Assets       []assetRef
}

type assetRef struct {
	Path           string
	ModName        string
	CurrentFmt     string
	Width          int
	Height         int
	Compressed     bool
	SourceMipCount int    // mip levels in the source DDS; drives per-file mip policy
	VirtualRelPath string // non-empty when sourced from WalkVirtual
}

// CompressJobData is passed from CompressConfig → Compress.
type CompressJobData struct {
	Groups             []ConfiguredGroup
	WorkerCount        int
	ModsDir            string // needed to compute RelPath in mod output mode
	ModOutputDir       string // non-empty enables mod output mode (e.g. /mods/ATAK)
	ModOutputIsPattern bool   // true when ModOutputDir is a display pattern (e.g. "ATAK - *"), not a real single folder
}

// ConfiguredGroup is a profile group with user-confirmed settings.
type ConfiguredGroup struct {
	ProfileName    string
	Format         string
	GenerateMips   []bool // parallel to Paths; per-file mip decision (profile policy OR source has mips)
	MaxTextureSize int
	Paths          []string
	RelPaths       []string // parallel to Paths; non-empty element = VirtualRelPath for that asset
	Widths         []int    // parallel to Paths; source texture width in pixels
	Heights        []int    // parallel to Paths; source texture height in pixels
	OutputDir      string   // empty means in-place (filepath.Dir of each asset)
	ModOutputDir   string   // overrides data.ModOutputDir per-group when non-empty
	ModOutputDirs  []string // parallel to Paths; per-asset override when non-empty (per-mod output), takes priority over ModOutputDir
}

// SummaryData is passed from Compress → Summary.
type SummaryData struct {
	Succeeded       int
	Failed          int
	OutputSkipped   int    // files skipped because they already exist in mod output dir
	OutputDir       string // mod output dir path; non-empty when mod output mode was active
	OutputIsPattern bool   // true when OutputDir is a display pattern (e.g. "ATAK - *"), not a literal deletable folder
	TotalBefore     int64
	TotalAfter      int64
	Errors          []string
	// FallbackCounts is reason (compress.Fallback* const) → count. Only nonzero
	// keys are populated. Empty map = clean run, no fallbacks fired.
	FallbackCounts map[string]int
	// Fallbacks is one string per file that succeeded via a fallback, formatted
	// for the drill-down list. Kept parallel-with-Errors so the summary screen
	// can reuse the same cursor/window scroll semantics.
	Fallbacks  []string
	RetryPaths []string
}
