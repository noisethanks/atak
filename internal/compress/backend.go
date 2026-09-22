package compress

import (
	"context"
	"os"
	"path/filepath"
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
//   - Pre-run gap: compressonator-bc7e has no `-w`/`-h` equivalent, so any file
//     that needs an exact output size has to involve texconv rather than
//     silently keeping its original size. Two rules put a file there — the
//     MaxTextureSize budget, and block alignment for a BCn target whose source
//     dimensions are not a multiple of 4 (see planResize). A third rule sends a
//     file down the same path for a different reason: a 24-bit source has to be
//     widened to RGBA first, because compressonator reads 24-bit with red and
//     blue transposed (see prepassPlan). resampleThenPrimary tries the cheap
//     split first, letting texconv write the intermediate and the configured
//     encoder do the encoding; only if that fails does texconv take the whole
//     job.
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
	plan := prepassPlan(primary, job)
	if fallback != nil && (plan.needed() || plan.Convert) {
		if r, ok := resampleThenPrimary(ctx, primary, fallback, job, plan); ok {
			return r
		}
		// Two ways to land here. The split was tried and did not work, or the file
		// needs a channel-order conversion and has no usable dimensions to resample
		// to. Either way the fallback takes the whole job, which is the safe answer
		// for a 24-bit source: texconv reads it correctly and needs no size.
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
	r := primary.Compress(ctx, inner)
	if !r.Success {
		return CompressionResult{}, false
	}
	restoreOutputName(job, inner)

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

// restoreOutputName puts the source's own extension case back on the file the
// split path just wrote.
//
// texconv always writes a lowercase .dds extension, so the intermediate for a
// Foo.DDS source is named foo.dds, and the encoder names its output after the
// file it read. The plain path never has this problem, because there the encoder
// reads Foo.DDS itself and keeps the spelling.
//
// The mismatch is not cosmetic. In place, the compressed foo.dds lands beside an
// untouched Foo.DDS and the source is never replaced. In mod-output mode, the job
// records the output as Foo.DDS while the file on disk is foo.dds, so the
// incremental skip in worker.go never finds it and the file is re-encoded on
// every run.
func restoreOutputName(job, inner Job) {
	want := filepath.Base(job.Asset.Path)
	got := filepath.Base(inner.Asset.Path)
	if want == got {
		return
	}
	from := filepath.Join(job.OutputDir, got)
	to := filepath.Join(job.OutputDir, want)
	if from == to {
		return
	}
	if _, err := os.Stat(from); err != nil {
		return // the encoder named it something else; leave it alone
	}
	_ = os.Rename(from, to)
}

// prepassPlan reports what texconv has to do to a source before the primary
// encoder reads it. A zero plan means the primary can take the file as it is.
//
// Two rules are about size and belong to planResize: the user's maxTextureSize
// budget, and block alignment for a BCn target whose source is not a multiple of
// 4. planResize owns both so dispatch and texconv's Run can never disagree about
// the target.
//
// The third rule is about channel order. compressonator-bc7e reads a 24-bit DDS
// with red and blue transposed, so a 24-bit source encoded by it comes out with
// the two swapped — no error, no warning, just a wrong picture. Routing the file
// through the same resample split fixes it, because the intermediate texconv
// writes is RGBA and compressonator reads that correctly. The conversion needs no
// new machinery: the plan keeps the source's own dimensions, so the resample is a
// format change and nothing else.
//
// A source with unknown dimensions gets Convert without a size. An exact resample
// needs both axes, and a DDS header can carry a zero there, so the split is not
// available for such a file. dispatch reads Convert separately for that reason:
// the whole job goes to texconv instead, which is slower and correct. Failing
// open here would hand the file straight back to the encoder that transposes it.
func prepassPlan(primary Backend, job Job) resizePlan {
	plan := planResize(job.Asset, job.Format, job.MaxTextureSize)
	if transposes24Bit(primary.Name(), job.Asset.CurrentFmt) {
		plan.Convert = true
		if !plan.needed() {
			plan.Width, plan.Height = job.Asset.Width, job.Asset.Height
		}
	}
	return plan
}

// transposes24Bit reports whether this backend would read this source format with
// its channels out of order. Scoped to the one measured pairing rather than
// "convert every odd format", so an unrelated future failure stays visible
// instead of being absorbed by a silent conversion. ParseDDS reports every 24-bit
// DDS as R8G8B8_UNORM whatever its masks say.
func transposes24Bit(backendName, sourceFormat string) bool {
	if backendName != "compressonator-bc7e" {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(sourceFormat)) {
	case "R8G8B8_UNORM", "B8G8R8_UNORM":
		return true
	}
	return false
}
