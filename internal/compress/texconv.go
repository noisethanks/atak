package compress

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/noisethanks/atak/internal/scan"
	"github.com/noisethanks/atak/internal/tools"
)

// FallbackReason identifies which fallback path produced a successful result.
// Empty string means no fallback fired (primary handled the file directly).
// Used by summary/error UI to surface silent-fallback attribution — a success
// via fallback is still a success, but the user should be able to see it.
const (
	FallbackBC7ToBC3      = "bc7-to-bc3"                  // texconv exit≠0 on BC7 → retried BC3
	FallbackResize        = "compressonator-resize"       // compressonator can't -w/-h → routed to texconv
	FallbackReaderGap     = "compressonator-reader-gap"   // compressonator DDS reader rejected → retried texconv
	FallbackBlockAlign    = "block-align"                 // source dims not a multiple of 4 → resized so the engine's loader never re-encodes
	FallbackChannelOrder  = "channel-order"               // 24-bit source → converted to RGBA first, because compressonator reads 24-bit with red and blue transposed
)

// FallbackLabel returns a human-readable label for a fallback reason.
func FallbackLabel(reason string) string {
	switch reason {
	case FallbackBC7ToBC3:
		return "BC7 → BC3 (texconv encoder rejected BC7)"
	case FallbackResize:
		return "resized to fit maxTextureSize"
	case FallbackReaderGap:
		return "compressonator-bc7e → texconv (DDS reader gap)"
	case FallbackBlockAlign:
		return "resized to a multiple of 4 (block alignment)"
	case FallbackChannelOrder:
		return "converted to RGBA (24-bit channel order)"
	}
	return reason
}

// CompressionResult holds the outcome of a single backend invocation.
type CompressionResult struct {
	Asset          scan.Asset
	Backend        string // backend that actually processed the file — "texconv" | "compressonator-bc7e" | "" on double-failure (neither produced output)
	FallbackReason string // one of Fallback* consts; empty when primary handled the file with no fallback
	ActualFormat   string // format actually used; may differ from requested if BC3_UNORM fallback was triggered
	Success        bool
	Skipped        bool // true when source file is already compressed (pre-job filter)
	OutputSkipped  bool // true when output file already exists in mod output dir (incremental skip)
	Err            error
	Stderr         string
	Before         int64
	After          int64
}

// TexconvBackend implements Backend by shelling out to the embedded texconv
// binary. Its arg builder, BC7→BC3 fallback, and post-run extension-case fix
// stay identical to the pre-backend-abstraction behavior — the abstraction
// only shifts where the entry point lives.
type TexconvBackend struct {
	Path string
}

func NewTexconvBackend(path string) *TexconvBackend { return &TexconvBackend{Path: path} }

func (b *TexconvBackend) Name() string { return "texconv" }

func (b *TexconvBackend) Compress(ctx context.Context, job Job) CompressionResult {
	r := Run(ctx, b.Path, job.Asset, job.Format, job.GenerateMips, job.MaxTextureSize, job.OutputDir)
	r.Backend = b.Name()
	return r
}

// resampleFormat is the intermediate the resize step writes. Uncompressed RGBA
// is a superset of every source format in this corpus (RAW32, RAW24, DXT1, DXT5,
// BC7), so the encoder that reads it back loses nothing the one-step path would
// have kept. None of the sources use an _SRGB format, so no gamma conversion is
// involved; a measured one-step/two-step comparison on a 658x493 icon showed a
// signed mean channel difference of -0.05 of 255, which is ordinary BC7 encoder
// variation rather than a color shift.
const resampleFormat = "R8G8B8A8_UNORM"

// Resample writes an uncompressed copy of asset at exactly width x height into
// outDir and returns the path it wrote.
//
// It exists so a backend that encodes far faster than texconv but cannot resize
// still gets to do the encoding. texconv handles only the resample, which is
// nearly free, and the fast encoder reads the result. Measured on a 658x493 UI
// icon from the corpus: letting texconv encode BC7 as well costs 27.9 s, while
// resample plus compressonator-bc7e costs 0.09 s, because DirectXTex's BC7 codec
// is scalar and single threaded while bc7e.ispc is SIMD and multithreaded. Across
// the 332 misaligned sources in one GAMMA modlist that is the difference between
// a few seconds and a few hours.
//
// The intermediate is always single-mip: the encoder rebuilds the chain from the
// resized top level, which is what it would have done from the source.
func (b *TexconvBackend) Resample(ctx context.Context, asset scan.Asset, width, height int, outDir string) (string, string, error) {
	if width <= 0 || height <= 0 {
		return "", "", fmt.Errorf("resample: bad target size %dx%d", width, height)
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", "", err
	}
	success, stderr, runErr, _ := runOnce(ctx, b.Path, asset.Path, resampleFormat, false, width, height, outDir)
	if !success {
		if runErr == nil {
			runErr = fmt.Errorf("resample: texconv produced no output")
		}
		return "", stderr, runErr
	}
	// texconv always lowercases the output extension, so a "foo.DDS" source
	// lands as "foo.dds" and looking for the original spelling would miss it.
	base := filepath.Base(asset.Path)
	out := filepath.Join(outDir, strings.TrimSuffix(base, filepath.Ext(base))+".dds")
	if _, err := os.Stat(out); err != nil {
		return "", stderr, err
	}
	return out, stderr, nil
}

// Run invokes texconv on a single asset and returns the result.
// If format is BC7_UNORM and texconv exits non-zero, automatically retries with BC3_UNORM.
// outputDir should be filepath.Dir(asset.Path) for in-place compression.
func Run(ctx context.Context, texconvPath string, asset scan.Asset, format string, generateMips bool, maxTextureSize int, outputDir string) CompressionResult {
	before, _ := fileSize(asset.Path)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return CompressionResult{Asset: asset, Success: false, Err: err}
	}

	// texconv -w/-h are exact, not maximums, so planResize computes both axes
	// explicitly: it holds the aspect ratio under a maxTextureSize budget and rounds
	// a block-compressed target to a multiple of 4. A zero plan passes no -w/-h and
	// leaves the source size alone. dispatch() uses the same function to decide
	// whether compressonator can take the job at all, so the two can't disagree.
	plan := planResize(asset, format, maxTextureSize)

	actualFormat := format
	success, stderr, runErr, after := runOnce(ctx, texconvPath, asset.Path, format, generateMips, plan.Width, plan.Height, outputDir)

	// BC7 fallback — only if ctx is still live (not a cancellation failure).
	if !success && format == "BC7_UNORM" && ctx.Err() == nil {
		actualFormat = "BC3_UNORM"
		// The plan is unchanged: BC3 uses the same 4x4 block as BC7, so the
		// aligned target is still the right one.
		success, stderr, runErr, after = runOnce(ctx, texconvPath, asset.Path, "BC3_UNORM", generateMips, plan.Width, plan.Height, outputDir)
	}

	if ctx.Err() != nil {
		outPath := filepath.Join(outputDir, filepath.Base(asset.Path))
		os.Remove(outPath)
		return CompressionResult{Asset: asset, Success: false, Err: ctx.Err()}
	}
	if !success {
		return CompressionResult{
			Asset:        asset,
			ActualFormat: actualFormat,
			Success:      false,
			Err:          runErr,
			Stderr:       stderr,
			Before:       before,
		}
	}
	// texconv always lowercases output extension; on Linux case-sensitive fs this
	// creates a new file, leaving the original untouched. Rename to match original.
	// Only applies in-place — in mod output mode outputDir differs from the source
	// directory, so renaming to asset.Path would overwrite the source file.
	if filepath.Dir(asset.Path) == outputDir {
		ext := filepath.Ext(asset.Path)
		texconvOut := filepath.Join(outputDir, strings.TrimSuffix(filepath.Base(asset.Path), ext)+".dds")
		if texconvOut != asset.Path {
			os.Rename(texconvOut, asset.Path)
		}
	}
	// A changed format is the more important thing to report, so it wins when both
	// happened. Attributing the alignment resize here rather than in dispatch()
	// means it is reported whichever backend is configured, since a texconv-primary
	// run never goes through dispatch's fallback path at all.
	var fallback string
	switch {
	case actualFormat != format:
		fallback = FallbackBC7ToBC3
	case plan.Aligned:
		fallback = FallbackBlockAlign
	}
	return CompressionResult{
		Asset:          asset,
		ActualFormat:   actualFormat,
		FallbackReason: fallback,
		Success:        true,
		Stderr:         stderr,
		Before:         before,
		After:          after,
	}
}

// ShouldGenerateMips resolves the per-file mip decision from a profile's policy, the
// source texture's own mip count, and the user's strip-when-disabled preference. A
// profile's generateMips:true always forces a full chain — world textures (diffuse,
// normal, weapon, terrain) are minified with distance and need mips even if a careless
// source shipped without them. generateMips:false is where stripWhenDisabled decides.
// Left off (the default) it preserves the source's own choice, keeping a chain when the
// source already had one and generating none when it did not — which is what lets a single
// profile cover a folder whose sources disagree, e.g. anamflares, where the moon flare
// ships 11 mips and needs them while flat lens sprites ship one and don't. Turned on, it
// makes generateMips:false authoritative and drops the source chain, restoring the strict
// "drawn at a fixed size, never minified" behavior for anyone who wants smaller output
// over source fidelity. It never overrides generateMips:true.
func ShouldGenerateMips(profileGenerateMips bool, sourceMipCount int, stripWhenDisabled bool) bool {
	if profileGenerateMips {
		return true
	}
	if stripWhenDisabled {
		return false
	}
	return sourceMipCount > 1
}

// texconvArgs builds the texconv command line. Kept separate from runOnce so the flags
// can be asserted without executing texconv — generateMips was previously accepted by
// Run and never reached this slice, which silently gave every profile a full mip chain.
func texconvArgs(format string, generateMips bool, targetW, targetH int, outputDir, inputPath string) []string {
	// -m 0 builds the full chain down to 1x1; -m 1 emits the top level only. The caller
	// resolves generateMips per file via ShouldGenerateMips, so a mipped source is never
	// flattened and a mipless world texture still gets a chain forced by its profile.
	mips := "0"
	if !generateMips {
		mips = "1"
	}
	args := []string{
		"-f", format,
		"-m", mips,
		"-if", "CUBIC", // cubic interpolation for mip generation
		"-gpu", "0",    // GPU accelerated compression, falls back to CPU if unavailable
		"-y",           // overwrite
		"-nologo",      // suppress header
		"-o", outputDir,
	}
	if targetW > 0 {
		args = append(args, "-w", strconv.Itoa(targetW))
	}
	if targetH > 0 {
		args = append(args, "-h", strconv.Itoa(targetH))
	}
	return append(args, "--", inputPath)
}

// runOnce executes a single texconv invocation and reports the outcome.
func runOnce(ctx context.Context, texconvPath, inputPath, format string, generateMips bool, targetW, targetH int, outputDir string) (success bool, stderr string, err error, after int64) {
	args := texconvArgs(format, generateMips, targetW, targetH, outputDir, inputPath)

	cmd := exec.Command(texconvPath, args...)
	tools.SetProcAttr(cmd)
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	job, _ := tools.NewJob()
	if startErr := cmd.Start(); startErr != nil {
		tools.CloseJob(job)
		return false, "", fmt.Errorf("texconv: %w", startErr), 0
	}
	tools.AssignJob(job, cmd)
	defer tools.CloseJob(job)
	_ = tools.WriteLock(cmd.Process.Pid)

	processExited := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			tools.KillProcess(cmd)
		case <-processExited:
		}
	}()

	waitErr := cmd.Wait()
	close(processExited)
	_ = tools.ClearLock()

	stderrStr := stderrBuf.String()
	if waitErr != nil {
		return false, stderrStr, fmt.Errorf("texconv: %w", waitErr), 0
	}

	outPath := filepath.Join(outputDir, filepath.Base(inputPath))
	sz, _ := fileSize(outPath)
	return true, stderrStr, nil, sz
}

func fileSize(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}
