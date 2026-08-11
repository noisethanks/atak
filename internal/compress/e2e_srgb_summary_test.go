package compress

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noisethanks/atak/internal/scan"
)

// TestE2ERealFileFallbackAttribution drives the real-world absui_arrows.dds
// (DXGI_FORMAT_B8G8R8A8_UNORM_SRGB via DX10 header) through RunPool with the
// same primary+fallback wiring the compress screen uses. Asserts the pool
// yields exactly one successful result whose FallbackReason attributes the
// silent-until-now sRGB reader-gap fallback — the ground-truth this feature
// exists to expose. Skipped when the mod file isn't installed locally.
func TestE2ERealFileFallbackAttribution(t *testing.T) {
	src := "/home/abhi/gamma/gamma/mods/419- Artefacts Belt Scroller - Demonized/gamedata/textures/ui/absui_arrows.dds"
	if _, err := os.Stat(src); err != nil {
		t.Skipf("real modfile absent: %v", err)
	}
	compressBin := repoToolPath(t, "compressonator-bc7e")
	texconvBin := repoToolPath(t, "texconv")

	primary := NewCompressonatorBackend(compressBin)
	fallback := NewTexconvBackend(texconvBin)
	outDir := t.TempDir()
	job := Job{
		Asset:        scan.Asset{Path: src, Width: 256, Height: 256, SourceMipCount: 1},
		Format:       "BC7_UNORM",
		GenerateMips: false,
		OutputDir:    outDir,
	}

	results := RunPool(context.Background(), primary, fallback, []Job{job}, 1)
	var got []CompressionResult
	for r := range results {
		got = append(got, r)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	r := got[0]
	if !r.Success {
		t.Fatalf("expected success via reader-gap fallback, got failure: err=%v stderr=%s", r.Err, r.Stderr)
	}
	if r.Backend != "texconv" {
		t.Errorf("Backend attribution: got %q, want %q", r.Backend, "texconv")
	}
	if r.FallbackReason != FallbackReaderGap {
		t.Errorf("FallbackReason: got %q, want %q", r.FallbackReason, FallbackReaderGap)
	}
	// Simulate the summary-bridge accumulation the compress screen performs so
	// the assertion covers the label the user actually sees, not just the raw
	// const. Regression barrier for any future rename of the label wording.
	label := FallbackLabel(r.FallbackReason)
	if !strings.Contains(label, "DDS reader gap") {
		t.Errorf("FallbackLabel wording: got %q, want substring %q", label, "DDS reader gap")
	}
	t.Logf("SUMMARY LINE: ↷  1 %s\n  Backend fallbacks list entry: %s\n    reason: %s",
		label, filepath.Base(r.Asset.Path), label)
}
