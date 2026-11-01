package compress

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/noisethanks/atak/internal/scan"
)

// TestIsBlockCompressed guards the one decision that keeps planResize from
// touching formats with no block constraint. An R8G8B8A8 UI texture at 269x271
// loads instantly today and must keep its exact dimensions.
func TestIsBlockCompressed(t *testing.T) {
	block := []string{"BC1_UNORM", "BC3_UNORM", "BC4_UNORM", "BC5_UNORM", "BC7_UNORM", "BC7_UNORM_SRGB", "bc5_snorm"}
	for _, f := range block {
		if !isBlockCompressed(f) {
			t.Errorf("isBlockCompressed(%q) = false, want true", f)
		}
	}
	plain := []string{"R8G8B8A8_UNORM", "B8G8R8A8_UNORM", "R16G16B16A16_FLOAT", "", "R8_UNORM"}
	for _, f := range plain {
		if isBlockCompressed(f) {
			t.Errorf("isBlockCompressed(%q) = true, want false", f)
		}
	}
}

// TestAlignDim covers the rounding direction on its own. Up is the default
// because d3dx11_43.dll rounds up, so the offline resize reproduces the picture
// the engine already draws. Down is the exception that keeps a maxTextureSize
// budget intact, and one block is the floor in both directions.
func TestAlignDim(t *testing.T) {
	tests := []struct {
		v, maxTextureSize, want int
		why                     string
	}{
		{256, 0, 256, "already a multiple of 4"},
		{4, 0, 4, "exactly one block"},
		{1, 0, 4, "sub-block rounds up to one block"},
		{2, 0, 4, "sub-block rounds up to one block"},
		{3, 0, 4, "sub-block rounds up to one block"},
		{43, 0, 44, "real icon: ui/alticons/bg1.dds"},
		{271, 0, 272, "real icon: ui/ui_hud_hit_mark.dds"},
		{2150, 0, 2152, "real icon: ui/ui_icon_bas.dds"},
		{5614, 0, 5616, "real page: ui/guide/article_gamma_repgun_image.dds"},
		{2046, 2046, 2044, "rounding up would exceed the budget, so round down"},
		{2047, 2048, 2048, "rounding up still fits the budget"},
		{3, 2, 4, "one block wins over a budget smaller than one block"},
	}
	for _, tt := range tests {
		if got := alignDim(tt.v, tt.maxTextureSize); got != tt.want {
			t.Errorf("alignDim(%d, %d) = %d, want %d (%s)", tt.v, tt.maxTextureSize, got, tt.want, tt.why)
		}
	}
}

// TestPlanResize is the regression corpus for the GAMMA freeze. Every "real"
// case below is a texture measured in the user's own install: compressed to BC7
// at a dimension that is not a multiple of 4, it cost the engine between 345 ms
// and several minutes on first bind. The aligned cases are the guard against the
// opposite regression — rerouting files that were always fine.
func TestPlanResize(t *testing.T) {
	tests := []struct {
		name           string
		w, h           int
		format         string
		maxTextureSize int
		wantW, wantH   int
		wantAligned    bool
	}{
		// Already correct: no resize, no reroute, no behavior change.
		{"aligned power of two", 1024, 1024, "BC7_UNORM", 0, 0, 0, false},
		{"aligned non-square", 4096, 2048, "BC7_UNORM", 0, 0, 0, false},
		{"aligned under the budget", 1024, 1024, "BC7_UNORM", 2048, 0, 0, false},

		// Misaligned, no budget: round up to the next block.
		{"real 43x43 icon", 43, 43, "BC7_UNORM", 0, 44, 44, true},
		{"real 269x271 hit mark", 269, 271, "BC7_UNORM", 0, 272, 272, true},
		{"real 1026x770 portraits", 1026, 770, "BC7_UNORM", 0, 1028, 772, true},
		{"real 1750x400 icon, one axis only", 1750, 400, "BC7_UNORM", 0, 1752, 400, true},
		{"real 8192x2150 icon sheet", 8192, 2150, "BC7_UNORM", 0, 8192, 2152, true},
		{"real 4094x4096 bump", 4094, 4096, "BC5_UNORM", 0, 4096, 4096, true},
		{"real 588x5614 guide page", 588, 5614, "BC3_UNORM", 0, 588, 5616, true},
		{"real 2048x2049 plate", 2048, 2049, "BC7_UNORM", 0, 2048, 2052, true},

		// Tiny sources still round up to one whole block.
		{"1x1", 1, 1, "BC1_UNORM", 0, 4, 4, true},
		{"2x3", 2, 3, "BC3_UNORM", 0, 4, 4, true},
		{"16x15", 16, 15, "BC7_UNORM", 0, 16, 16, true},

		// Formats with no block constraint keep their exact dimensions.
		{"uncompressed stays misaligned", 269, 271, "R8G8B8A8_UNORM", 0, 0, 0, false},
		{"uncompressed over budget still downscales", 4096, 4096, "R8G8B8A8_UNORM", 2048, 2048, 2048, false},

		// maxTextureSize alone, which is the pre-existing behavior.
		{"budget downscale lands aligned", 4096, 4096, "BC7_UNORM", 2048, 2048, 2048, false},
		{"budget downscale holds aspect ratio", 4096, 2048, "BC7_UNORM", 2048, 2048, 1024, false},

		// Both reasons at once. Downscale first, then align the result.
		{"budget downscale lands misaligned", 4096, 2050, "BC7_UNORM", 2048, 2048, 1028, true},
		{"misaligned source over budget", 2048, 2049, "BC7_UNORM", 2048, 2048, 2048, true},

		// Unusable header data: never resize on a guess.
		{"zero dimensions", 0, 0, "BC7_UNORM", 0, 0, 0, false},
		{"zero width", 0, 271, "BC7_UNORM", 0, 0, 0, false},
		{"negative height", 43, -1, "BC7_UNORM", 0, 0, 0, false},
	}

	for _, tt := range tests {
		asset := scan.Asset{Path: "/x.dds", Width: tt.w, Height: tt.h}
		got := planResize(asset, tt.format, tt.maxTextureSize)
		if got.Width != tt.wantW || got.Height != tt.wantH {
			t.Errorf("%s: planResize(%dx%d, %s, max=%d) = %dx%d, want %dx%d",
				tt.name, tt.w, tt.h, tt.format, tt.maxTextureSize, got.Width, got.Height, tt.wantW, tt.wantH)
		}
		if got.Aligned != tt.wantAligned {
			t.Errorf("%s: Aligned = %v, want %v", tt.name, got.Aligned, tt.wantAligned)
		}
	}
}

// TestPlanResizeOutputIsAlwaysAligned is the property the whole feature exists to
// hold: after planResize, no block-compressed job can produce a file the engine
// has to round. It sweeps every dimension pair in a small range plus the real
// outliers, with and without a budget.
func TestPlanResizeOutputIsAlwaysAligned(t *testing.T) {
	var dims []int
	for v := 1; v <= 40; v++ {
		dims = append(dims, v)
	}
	dims = append(dims, 43, 269, 271, 400, 588, 770, 1026, 1750, 2049, 2150, 4094, 5614, 8192)

	for _, maxTextureSize := range []int{0, 512, 2048, 4096} {
		for _, w := range dims {
			for _, h := range dims {
				asset := scan.Asset{Path: "/x.dds", Width: w, Height: h}
				p := planResize(asset, "BC7_UNORM", maxTextureSize)
				outW, outH := w, h
				if p.needed() {
					outW, outH = p.Width, p.Height
				}
				if outW%blockDim != 0 || outH%blockDim != 0 {
					t.Fatalf("max=%d: %dx%d produced %dx%d, which the loader would still round",
						maxTextureSize, w, h, outW, outH)
				}
			}
		}
	}
}

// TestNeedsResize checks the routing predicate dispatch() uses. compressonator-bc7e
// has no -w/-h, so every job with a resize plan has to reach texconv instead.
func TestPrepassPlanSizeRules(t *testing.T) {
	texconv := NewTexconvBackend("")
	tests := []struct {
		name string
		job  Job
		want bool
	}{
		{"aligned, no budget", Job{Asset: scan.Asset{Width: 1024, Height: 1024}, Format: "BC7_UNORM"}, false},
		{"misaligned", Job{Asset: scan.Asset{Width: 269, Height: 271}, Format: "BC7_UNORM"}, true},
		{"misaligned but uncompressed", Job{Asset: scan.Asset{Width: 269, Height: 271}, Format: "R8G8B8A8_UNORM"}, false},
		{"over budget", Job{Asset: scan.Asset{Width: 4096, Height: 4096}, Format: "BC7_UNORM", MaxTextureSize: 2048}, true},
		{"under budget and aligned", Job{Asset: scan.Asset{Width: 512, Height: 512}, Format: "BC7_UNORM", MaxTextureSize: 2048}, false},
		{"no dimensions", Job{Asset: scan.Asset{}, Format: "BC7_UNORM"}, false},
	}
	for _, tt := range tests {
		if got := prepassPlan(texconv, tt.job).needed(); got != tt.want {
			t.Errorf("%s: prepassPlan().needed() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestPrepassPlanChannelRule covers the rule that has nothing to do with size: a
// 24-bit source must reach compressonator as RGBA, because compressonator reads
// 24-bit with red and blue transposed. The plan keeps the source dimensions, so
// the resample changes the format and not the picture.
func TestPrepassPlanChannelRule(t *testing.T) {
	compressonator := NewCompressonatorBackend("")
	texconv := NewTexconvBackend("")

	job := Job{
		Asset:  scan.Asset{Width: 512, Height: 512, CurrentFmt: "R8G8B8_UNORM"},
		Format: "BC7_UNORM",
	}

	plan := prepassPlan(compressonator, job)
	if !plan.needed() || !plan.Convert {
		t.Fatalf("24-bit source on compressonator: got %+v, want a conversion plan", plan)
	}
	if plan.Width != 512 || plan.Height != 512 {
		t.Errorf("conversion resized the picture: got %dx%d, want 512x512", plan.Width, plan.Height)
	}
	if plan.reason() != FallbackChannelOrder {
		t.Errorf("reason = %q, want %q", plan.reason(), FallbackChannelOrder)
	}

	// texconv reads 24-bit correctly, so it never pays for the extra step.
	if p := prepassPlan(texconv, job); p.needed() || p.Convert {
		t.Errorf("texconv primary: got %+v, want no plan", p)
	}

	// A 32-bit source is left alone on both backends.
	rgba := job
	rgba.Asset.CurrentFmt = "R8G8B8A8_UNORM"
	if p := prepassPlan(compressonator, rgba); p.needed() || p.Convert {
		t.Errorf("32-bit source: got %+v, want no plan", p)
	}

	// Block alignment still wins the attribution when a 24-bit source is also
	// misaligned: a changed dimension is the surprising part.
	both := Job{
		Asset:  scan.Asset{Width: 269, Height: 271, CurrentFmt: "R8G8B8_UNORM"},
		Format: "BC7_UNORM",
	}
	p := prepassPlan(compressonator, both)
	if !p.Aligned || !p.Convert {
		t.Fatalf("misaligned 24-bit source: got %+v, want both rules set", p)
	}
	if p.Width != 272 || p.Height != 272 {
		t.Errorf("got %dx%d, want 272x272", p.Width, p.Height)
	}
	if p.reason() != FallbackBlockAlign {
		t.Errorf("reason = %q, want %q", p.reason(), FallbackBlockAlign)
	}
}

// stubBackend records what dispatch asked of it. compressErr makes Compress
// report a per-file failure the way a real backend would.
type stubBackend struct {
	name        string
	compressErr bool
	compressed  int
	gotPath     string
}

func (s *stubBackend) Name() string { return s.name }

func (s *stubBackend) Compress(ctx context.Context, job Job) CompressionResult {
	s.compressed++
	s.gotPath = job.Asset.Path
	if s.compressErr {
		return CompressionResult{Asset: job.Asset, Backend: s.name, Success: false, Err: errors.New("stub failure")}
	}
	return CompressionResult{Asset: job.Asset, Backend: s.name, Success: true, After: 64}
}

// stubResampler is a fallback backend that can also resize, like texconv.
type stubResampler struct {
	stubBackend
	resampleErr bool
	resampled   int
	gotW, gotH  int
}

func (s *stubResampler) Resample(ctx context.Context, asset scan.Asset, width, height int, outDir string) (string, string, error) {
	s.resampled++
	s.gotW, s.gotH = width, height
	if s.resampleErr {
		return "", "", errors.New("stub resample failure")
	}
	out := filepath.Join(outDir, filepath.Base(asset.Path))
	if err := os.WriteFile(out, []byte("not a real dds"), 0644); err != nil {
		return "", "", err
	}
	return out, "", nil
}

// TestDispatchResampleThenPrimary checks the fast path: texconv is asked only for
// the resize, at the aligned size, and the configured encoder gets the temporary
// file rather than the original.
func TestDispatchResampleThenPrimary(t *testing.T) {
	primary := &stubBackend{name: "compressonator-bc7e"}
	fallback := &stubResampler{stubBackend: stubBackend{name: "texconv"}}
	src := filepath.Join(t.TempDir(), "hit_mark.dds")
	if err := os.WriteFile(src, []byte("source"), 0644); err != nil {
		t.Fatal(err)
	}
	job := Job{Asset: scan.Asset{Path: src, Width: 269, Height: 271}, Format: "BC7_UNORM", OutputDir: t.TempDir()}

	r := dispatch(context.Background(), primary, fallback, job)

	if !r.Success {
		t.Fatalf("expected success, got err=%v", r.Err)
	}
	if fallback.resampled != 1 || fallback.gotW != 272 || fallback.gotH != 272 {
		t.Errorf("resample called %d time(s) at %dx%d, want 1 at 272x272",
			fallback.resampled, fallback.gotW, fallback.gotH)
	}
	if fallback.compressed != 0 {
		t.Errorf("fallback encoded %d file(s); the fast path must leave encoding to the primary", fallback.compressed)
	}
	if primary.compressed != 1 {
		t.Errorf("primary encoded %d file(s), want 1", primary.compressed)
	}
	if primary.gotPath == src {
		t.Error("primary was handed the original source; it must receive the resized intermediate")
	}
	if r.Backend != "compressonator-bc7e" {
		t.Errorf("Backend: got %q, want %q", r.Backend, "compressonator-bc7e")
	}
	if r.FallbackReason != FallbackBlockAlign {
		t.Errorf("FallbackReason: got %q, want %q", r.FallbackReason, FallbackBlockAlign)
	}
	// The reported asset and Before size must describe the original file, or the
	// results screen names a temp path and the saved-bytes total goes wrong.
	if r.Asset.Path != src {
		t.Errorf("reported asset: got %q, want the original %q", r.Asset.Path, src)
	}
	if r.Before != int64(len("source")) {
		t.Errorf("Before: got %d, want %d (size of the original file)", r.Before, len("source"))
	}
}

// TestDispatchResampleFailureFallsBackToWholeJob is the safety property of the
// fast path: it may be slower to let texconv do everything, but a resize step
// that fails must never turn a file that would have compressed into a failure.
func TestDispatchResampleFailureFallsBackToWholeJob(t *testing.T) {
	for _, tt := range []struct {
		name        string
		resampleErr bool
		primaryErr  bool
	}{
		{"resize step fails", true, false},
		{"encoder rejects the intermediate", false, true},
	} {
		primary := &stubBackend{name: "compressonator-bc7e", compressErr: tt.primaryErr}
		fallback := &stubResampler{stubBackend: stubBackend{name: "texconv"}, resampleErr: tt.resampleErr}
		src := filepath.Join(t.TempDir(), "hit_mark.dds")
		if err := os.WriteFile(src, []byte("source"), 0644); err != nil {
			t.Fatal(err)
		}
		job := Job{Asset: scan.Asset{Path: src, Width: 269, Height: 271}, Format: "BC7_UNORM", OutputDir: t.TempDir()}

		r := dispatch(context.Background(), primary, fallback, job)

		if !r.Success {
			t.Errorf("%s: expected the whole job to go to the fallback and succeed, got err=%v", tt.name, r.Err)
		}
		if fallback.compressed != 1 {
			t.Errorf("%s: fallback encoded %d file(s), want 1", tt.name, fallback.compressed)
		}
		if r.Backend != "texconv" {
			t.Errorf("%s: Backend: got %q, want %q", tt.name, r.Backend, "texconv")
		}
		if r.FallbackReason != FallbackBlockAlign {
			t.Errorf("%s: FallbackReason: got %q, want %q", tt.name, r.FallbackReason, FallbackBlockAlign)
		}
	}
}
