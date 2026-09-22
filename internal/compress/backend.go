package compress

import (
	"context"
	"os"
	"strings"

	"github.com/noisethanks/atak/internal/scan"
)

// Backend abstracts a single-file texture compressor. Two implementations exist
// today — texconv (all platforms, GPU-accelerated on Windows for BC7) and
// compressonator-bc7e (all platforms, CPU-only, all five BC formats). The
// interface exists so worker.go can select once per run and route every job
// through the same code path, and so the compressonator→texconv fallbacks in
// dispatch() can swap backends per-file without leaking backend specifics into
// the worker loop.
type Backend interface {
	Name() string
	Compress(ctx context.Context, job Job) CompressionResult
}

// compressonatorLoadErrPattern is the exact substring compressonator-bc7e
// prints when its DDS reader rejects a source file it can't parse — most
// commonly DX10-header DDS with an sRGB DXGI_FORMAT variant (e.g.
// B8G8R8A8_UNORM_SRGB = 91) that DirectXTex handles but Compressonator does
// not. Kept as a narrow substring rather than a blanket "any compressonator
// failure retries" so unrelated future failure classes (argv bugs, missing
// binary, permissions, format mismatch, quota) don't get silently absorbed
// into a texconv retry that hides the real bug.
const compressonatorLoadErrPattern = "Could not load source file"

// dispatch routes a job through primary and covers the two cases primary
// can't handle by transparently retrying on fallback:
//
//   - Pre-run resize gap: compressonator-bc7e has no `-w`/`-h` equivalent, so
//     any file that needs an exact output size has to involve texconv rather
//     than silently keeping its original size. Two rules put a file there —
//     the MaxTextureSize budget, and block alignment for a BCn target whose
//     source dimensions are not a multiple of 4 (see planResize).
//     resampleThenPrimary tries the cheap split first, letting texconv resize
//     into a temporary file and the configured encoder do the encoding; only
//     if that fails does texconv take the whole job.
//   - Post-run DDS-reader gap: compressonator's DDS loader rejects some
//     subvariants (see compressonatorLoadErrPattern). On that specific error
//     substring, retry via texconv. If texconv also fails, the file is
//     surfaced as a normal per-file failure — same counting, same UI —
//     with both backends' stderr concatenated into the failure record so the
//     debug trail carries full context. Backend is blanked in that case
//     because neither backend produced output.
//
// Both mirror the pre-existing texconv BC7→BC3 fallback philosophy —
// automatic, transparent, and recorded in CompressionResult so summary UI
// can attribute mismatches correctly.
func dispatch(ctx context.Context, primary, fallback Backend, job Job) CompressionResult {
	if fallback != nil && needsResize(job) {
		plan := planResize(job.Asset, job.Format, job.MaxTextureSize)
		if r, ok := resampleThenPrimary(ctx, primary, fallback, job, plan); ok {
			return r
		}
		r := fallback.Compress(ctx, job)
		// Never overwrite a reason the fallback already attributed: a BC7 encoder
		// rejection that changed the format matters more than why the file was
		// resized.
		if r.Success && r.FallbackReason == "" && primary.Name() == "compressonator-bc7e" {
			r.FallbackReason = plan.reason()
		}
		return r
	}
	r := primary.Compress(ctx, job)
	if r.Success || fallback == nil || ctx.Err() != nil {
		return r
	}
	if primary.Name() != "compressonator-bc7e" || !strings.Contains(r.Stderr, compressonatorLoadErrPattern) {
		return r
	}
	r2 := fallback.Compress(ctx, job)
	if r2.Success {
		r2.FallbackReason = FallbackReaderGap
		return r2
	}
	r2.Stderr = "compressonator-bc7e:\n" + r.Stderr + "\n---\ntexconv:\n" + r2.Stderr
	r2.Backend = ""
	return r2
}

// Resampler is a backend that can resize a texture to an exact size without also
// being the one to encode it. Only texconv implements it today. dispatch uses it
// to split a resize job in two so the configured encoder still does the encoding.
type Resampler interface {
	Resample(ctx context.Context, asset scan.Asset, width, height int, outDir string) (string, string, error)
}

// resampleThenPrimary handles a resize job as two cheap steps instead of one
// expensive one: the fallback resizes the source into a temporary uncompressed
// file, then the primary encodes that file as if it had been the source.
//
// The point is speed. Handing the whole job to texconv also works and is the path
// taken when this returns false, but texconv's BC7 codec is scalar and single
// threaded. On a 658x493 icon from the corpus it costs 27.9 s against 0.09 s for
// resample-then-compressonator. A modlist has hundreds of misaligned files, so
// the difference decides whether the alignment fix costs seconds or hours.
//
// It returns ok=false whenever the two-step cannot be used or did not work, and
// the caller then runs the single-step path. Nothing here is allowed to turn a
// file that would have compressed into a failure.
func resampleThenPrimary(ctx context.Context, primary, fallback Backend, job Job, plan resizePlan) (CompressionResult, bool) {
	rs, ok := fallback.(Resampler)
	if !ok || !plan.needed() {
		return CompressionResult{}, false
	}

	tmpDir, err := os.MkdirTemp("", "atak-resample-")
	if err != nil {
		return CompressionResult{}, false
	}
	defer os.RemoveAll(tmpDir)

	tmpPath, _, err := rs.Resample(ctx, job.Asset, plan.Width, plan.Height, tmpDir)
	if err != nil || ctx.Err() != nil {
		return CompressionResult{}, false
	}

	// The intermediate is already the target size, so planResize on the inner job
	// returns nothing and the primary cannot resize it a second time.
	inner := job
	inner.Asset.Path = tmpPath
	inner.Asset.Width = plan.Width
	inner.Asset.Height = plan.Height
	// The intermediate really is single-mip, whatever the source carried. Saying so
	// keeps a texconv primary from flattening a file that is already flat.
	inner.Asset.SourceMipCount = 1
	r := primary.Compress(ctx, inner)
	if !r.Success {
		return CompressionResult{}, false
	}

	// Report the original file, not the temporary one: the asset the user
	// recognizes, and a Before size that makes the saved-bytes total correct.
	r.Asset = job.Asset
	before, _ := fileSize(job.Asset.Path)
	r.Before = before
	if r.FallbackReason == "" {
		r.FallbackReason = plan.reason()
	}
	return r, true
}

// needsResize reports whether a job has to reach an encoder that can resize to an
// exact size. Two things put it there: the user's maxTextureSize budget, and block
// alignment for a BCn target whose source is not a multiple of 4. planResize owns
// both rules so dispatch and texconv's Run can never disagree about the target.
func needsResize(job Job) bool {
	return planResize(job.Asset, job.Format, job.MaxTextureSize).needed()
}
