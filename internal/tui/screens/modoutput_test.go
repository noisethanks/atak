package screens

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/noisethanks/atak/internal/compress"
	"github.com/noisethanks/atak/internal/config"
	"github.com/noisethanks/atak/internal/scan"
)

// fakeTexconvScript stands in for texconv: writes <out>/<stem>.dds.
func fakeTexconvScript(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake texconv")
	}
	script := `#!/bin/sh
while [ $# -gt 0 ]; do case "$1" in -o) out="$2"; shift;; --) in="$2"; shift;; esac; shift; done
b=$(basename "$in"); stem=${b%.*}
printf new > "$out/$stem.dds"
`
	p := filepath.Join(t.TempDir(), "texconv")
	if err := os.WriteFile(p, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestModOutputModeAbsentOutputFolderCompressesEverything drives the real TUI
// path (scan assets -> NewResults -> buildJobs -> buildJobs -> RunPool ->
// compressToOpCh) in Mod Output Mode with an output folder that does not exist.
// Every file must be compressed; none may be "already in output folder".
func TestModOutputModeAbsentOutputFolderCompressesEverything(t *testing.T) {
	isolateConfig(t)

	modsDir := t.TempDir()
	rels := []string{
		"gamedata/textures/wpn/a_hud.dds",
		"gamedata/textures/wpn/b_hud.dds",
		"gamedata/textures/wpn/c_hud.dds",
	}
	var assets []scan.Asset
	for _, rel := range rels {
		p := filepath.Join(modsDir, "ModA", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("src"), 0644); err != nil {
			t.Fatal(err)
		}
		assets = append(assets, scan.Asset{
			Path: p, ModName: "ModA", ProfileMatch: "Weapon Textures",
			CurrentFmt: "R8G8B8A8_UNORM", Width: 64, Height: 64,
			SourceMipCount: 1, VirtualRelPath: rel,
		})
	}

	cfg := &config.Config{ModsDir: modsDir, ModOutputMode: true, ModOutputName: "ATAK"}
	outDir := filepath.Join(modsDir, "ATAK")
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Fatalf("output folder must be absent, stat err = %v", err)
	}

	nav := NewResults(ScanResultData{Assets: assets}, cfg).buildJobs(0, "", "")().(NavigateMsg)
	data := nav.Data.(CompressJobData)
	jobs := buildJobs(data)

	backend := compress.NewTexconvBackend(fakeTexconvScript(t))
	resultCh := compress.RunPool(context.Background(), backend, nil, jobs, 1)
	_, sumCh := compressToOpCh(resultCh, len(jobs), data.ModOutputDir)
	sum := <-sumCh

	if sum.OutputSkipped != 0 || sum.Succeeded != len(rels) || sum.Failed != 0 {
		t.Fatalf("succeeded=%d skipped(already in output)=%d failed=%d, want %d/0/0 (errors: %v)",
			sum.Succeeded, sum.OutputSkipped, sum.Failed, len(rels), sum.Errors)
	}
	for _, rel := range rels {
		if _, err := os.Stat(filepath.Join(outDir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("missing output %s: %v", rel, err)
		}
	}
}
