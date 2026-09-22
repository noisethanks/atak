package screens

import (
	"path/filepath"
	"testing"

	"github.com/noisethanks/atak/internal/config"
	"github.com/noisethanks/atak/internal/scan"
)

// isolateConfig points the config package at a throwaway directory, so
// LoadProfiles writes and reads the shipped defaults instead of whatever the
// machine running the test happens to have in its user config dir.
func isolateConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
}

// TestJobCarriesSourceFormat covers the seam between the scan and the encoder.
// Every field dispatch() needs has to survive three hops: scan.Asset → assetRef →
// ConfiguredGroup → compress.Job. Width and Height were carried across; the
// source format was not, and dropping it made the 24-bit channel-order fix inert
// — dispatch saw an empty format, decided no conversion was needed, and handed
// every 24-bit source straight to the encoder that transposes them.
//
// The test deliberately goes through the real path rather than building a Job
// literal. A Job built by hand proves nothing about what the application does.
func TestJobCarriesSourceFormat(t *testing.T) {
	isolateConfig(t)

	const srcFmt = "R8G8B8_UNORM"
	asset := scan.Asset{
		Path:           "/mods/ModA/gamedata/textures/wpn/ak74_hud.dds",
		ModName:        "ModA",
		ProfileMatch:   "Weapon Textures",
		CurrentFmt:     srcFmt,
		Width:          512,
		Height:         512,
		SourceMipCount: 1,
		VirtualRelPath: "gamedata/textures/wpn/ak74_hud.dds",
	}

	results := NewResults(ScanResultData{Assets: []scan.Asset{asset}}, &config.Config{
		ModsDir:       "/mods",
		ModOutputMode: true,
		ModOutputName: "ATAK",
	})

	msg := results.buildJobs(0, "", "")()
	nav, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("buildJobs returned %T, want NavigateMsg", msg)
	}
	data, ok := nav.Data.(CompressJobData)
	if !ok {
		t.Fatalf("NavigateMsg.Data is %T, want CompressJobData", nav.Data)
	}

	jobs := buildJobs(data)
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 — the asset did not reach a compressible group", len(jobs))
	}
	job := jobs[0]
	if job.Asset.CurrentFmt != srcFmt {
		t.Errorf("job.Asset.CurrentFmt = %q, want %q", job.Asset.CurrentFmt, srcFmt)
	}
	if job.Asset.Width != 512 || job.Asset.Height != 512 {
		t.Errorf("job dimensions = %dx%d, want 512x512", job.Asset.Width, job.Asset.Height)
	}
	if job.Format != "BC7_UNORM" {
		t.Errorf("job.Format = %q, want BC7_UNORM — a bump map must not reach BC5", job.Format)
	}
}

// TestJobFormatsStayParallelToPaths guards the slice pairing. ConfiguredGroup
// carries five slices indexed by the same i, and a group built with one of them
// short would silently give some files another file's format.
func TestJobFormatsStayParallelToPaths(t *testing.T) {
	isolateConfig(t)

	var assets []scan.Asset
	formats := []string{"R8G8B8_UNORM", "R8G8B8A8_UNORM", "R8G8B8_UNORM"}
	for i, f := range formats {
		assets = append(assets, scan.Asset{
			Path:         "/mods/ModA/gamedata/textures/wpn/part" + string(rune('a'+i)) + "_hud.dds",
			ModName:      "ModA",
			ProfileMatch: "Weapon Textures",
			CurrentFmt:   f,
			Width:        64,
			Height:       64,
		})
	}

	results := NewResults(ScanResultData{Assets: assets}, &config.Config{ModsDir: "/mods"})
	nav := results.buildJobs(0, "", "")().(NavigateMsg)
	jobs := buildJobs(nav.Data.(CompressJobData))

	if len(jobs) != len(formats) {
		t.Fatalf("got %d jobs, want %d", len(jobs), len(formats))
	}
	for i, job := range jobs {
		if job.Asset.CurrentFmt != formats[i] {
			t.Errorf("job %d (%s): CurrentFmt = %q, want %q",
				i, filepath.Base(job.Asset.Path), job.Asset.CurrentFmt, formats[i])
		}
	}
}

// TestBuildJobsToleratesShortFormats keeps the defensive read honest: buildJobs
// indexes Formats with the same i as Paths, and a group assembled elsewhere with
// no formats at all must still produce jobs rather than panic.
func TestBuildJobsToleratesShortFormats(t *testing.T) {
	data := CompressJobData{Groups: []ConfiguredGroup{{
		ProfileName:  "Bump Maps",
		Format:       "BC7_UNORM",
		Paths:        []string{"/mods/ModA/a_bump.dds", "/mods/ModA/b_bump.dds"},
		Widths:       []int{64, 64},
		Heights:      []int{64, 64},
		GenerateMips: []bool{true, true},
		Formats:      []string{"R8G8B8_UNORM"}, // deliberately short
	}}}

	jobs := buildJobs(data)
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	if jobs[0].Asset.CurrentFmt != "R8G8B8_UNORM" {
		t.Errorf("job 0 format = %q", jobs[0].Asset.CurrentFmt)
	}
	if jobs[1].Asset.CurrentFmt != "" {
		t.Errorf("job 1 format = %q, want empty rather than a neighbour's value", jobs[1].Asset.CurrentFmt)
	}
}
