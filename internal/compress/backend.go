package compress

import (
	"context"
	"strings"
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
//     any file whose dimensions exceed MaxTextureSize is sent directly to
//     texconv rather than silently keeping its original size.
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
		r := fallback.Compress(ctx, job)
		if r.Success && primary.Name() == "compressonator-bc7e" {
			r.FallbackReason = FallbackResize
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

func needsResize(job Job) bool {
	if job.MaxTextureSize <= 0 {
		return false
	}
	a := job.Asset
	if a.Width <= 0 || a.Height <= 0 {
		return false
	}
	return a.Width > job.MaxTextureSize || a.Height > job.MaxTextureSize
}
