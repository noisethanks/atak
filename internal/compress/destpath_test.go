package compress

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/noisethanks/atak/internal/scan"
)

// In Mod Output Mode the destination is never the source, whatever OutputDir
// holds. OutputDir == Dir(source) used to make destPath return the source, so
// every file looked already compressed.
func TestDestPathModOutputNeverEqualsSource(t *testing.T) {
	src := filepath.Join("mods", "ModA", "gamedata", "t", "a.dds")
	for _, outDir := range []string{"", filepath.Dir(src), "elsewhere"} {
		job := Job{
			Asset:        scan.Asset{Path: src},
			OutputDir:    outDir,
			ModOutputDir: filepath.Join("mods", "ATAK"),
			RelPath:      "gamedata/t/a.dds",
		}
		got, err := destPath(job)
		if err != nil {
			t.Fatalf("OutputDir=%q: %v", outDir, err)
		}
		if got == src {
			t.Errorf("OutputDir=%q: destPath == source %q", outDir, src)
		}
		if want := filepath.Join("mods", "ATAK", "gamedata", "t", "a.dds"); got != want {
			t.Errorf("OutputDir=%q: destPath = %q, want %q", outDir, got, want)
		}
	}
}

// A Mod Output Mode job without RelPath is an error, not a guess.
func TestDestPathModOutputRequiresRelPath(t *testing.T) {
	src := filepath.Join("mods", "ModA", "a.dds")
	job := Job{Asset: scan.Asset{Path: src}, OutputDir: filepath.Dir(src), ModOutputDir: "out"}
	if p, err := destPath(job); err == nil {
		t.Fatalf("destPath = %q, want error", p)
	}
	for r := range RunPool(context.Background(), nil, nil, []Job{job}, 1) {
		if r.Success || r.Err == nil || r.OutputSkipped {
			t.Fatalf("result = %+v, want a failure with an error", r)
		}
	}
}
