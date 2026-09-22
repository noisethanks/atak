package compress

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/noisethanks/atak/internal/tools"
)

// CompressonatorBackend implements Backend using the embedded compressonator-bc7e
// CLI (an AMD Compressonator fork with the BC7 CPU codec swapped for bc7e.ispc,
// GPU codec paths removed). Two properties matter for callers:
//
//   - Always CPU. The fork has no working GPU path — even so, every invocation
//     explicitly passes `-EncodeWith CPU` rather than relying on a default, so
//     a future upstream change to the default can't quietly re-enable a broken
//     GPU path. This is not user-configurable.
//   - No exact-size resize. The CLI exposes mip controls but no `-w`/`-h`
//     equivalent, so files that need MaxTextureSize downscaling are routed to
//     texconv by dispatch() rather than silently keeping their original size.
type CompressonatorBackend struct {
	Path string
}

func NewCompressonatorBackend(path string) *CompressonatorBackend {
	return &CompressonatorBackend{Path: path}
}

func (b *CompressonatorBackend) Name() string { return "compressonator-bc7e" }

func (b *CompressonatorBackend) Compress(ctx context.Context, job Job) CompressionResult {
	asset := job.Asset
	before, _ := fileSize(asset.Path)

	if err := os.MkdirAll(job.OutputDir, 0755); err != nil {
		return CompressionResult{Asset: asset, Backend: b.Name(), Success: false, Err: err}
	}

	// Compressonator infers container from the output filename extension. Naming
	// the output ".dds" up-front avoids the extension-case rename dance texconv
	// needs on case-sensitive filesystems.
	outPath := filepath.Join(job.OutputDir, filepath.Base(asset.Path))
	ext := filepath.Ext(outPath)
	if !strings.EqualFold(ext, ".dds") {
		outPath = strings.TrimSuffix(outPath, ext) + ".dds"
	}
	// Remove any pre-existing output — Compressonator has no overwrite flag and
	// the CLI's behavior on an existing destination isn't documented; deleting
	// first makes it deterministic.
	_ = os.Remove(outPath)

	format, err := compressonatorFormat(job.Format)
	if err != nil {
		return CompressionResult{Asset: asset, Backend: b.Name(), Success: false, Err: err, Before: before}
	}

	args := compressonatorArgs(format, job.GenerateMips, asset.Path, outPath)

	cmd := exec.Command(b.Path, args...)
	tools.SetProcAttr(cmd)
	var stderrBuf, stdoutBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	cmd.Stdout = &stdoutBuf

	jobHandle, _ := tools.NewJob()
	if startErr := cmd.Start(); startErr != nil {
		tools.CloseJob(jobHandle)
		return CompressionResult{Asset: asset, Backend: b.Name(), Success: false, Err: fmt.Errorf("compressonator: %w", startErr), Before: before}
	}
	tools.AssignJob(jobHandle, cmd)
	defer tools.CloseJob(jobHandle)
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

	// Compressonator writes progress+diagnostics to stdout, not stderr — carry
	// both so failure UI shows the actual error message rather than empty.
	diag := strings.TrimSpace(stderrBuf.String())
	if s := strings.TrimSpace(stdoutBuf.String()); s != "" {
		if diag != "" {
			diag = s + "\n" + diag
		} else {
			diag = s
		}
	}

	if ctx.Err() != nil {
		os.Remove(outPath)
		return CompressionResult{Asset: asset, Backend: b.Name(), Success: false, Err: ctx.Err()}
	}
	if waitErr != nil {
		// Prepend the exact argv so failure records are self-contained — no need
		// to re-run atak under a debugger to see what the child received.
		argvLine := "argv: " + b.Path + " " + strings.Join(args, " ")
		if diag != "" {
			diag = argvLine + "\n" + diag
		} else {
			diag = argvLine
		}
		return CompressionResult{
			Asset:        asset,
			Backend:      b.Name(),
			ActualFormat: job.Format,
			Success:      false,
			Err:          fmt.Errorf("compressonator: %w", waitErr),
			Stderr:       diag,
			Before:       before,
		}
	}

	after, _ := fileSize(outPath)
	return CompressionResult{
		Asset:        asset,
		Backend:      b.Name(),
		ActualFormat: job.Format,
		Success:      true,
		Stderr:       diag,
		Before:       before,
		After:        after,
	}
}

// compressonatorArgs builds the CLI command line. Kept separate from Compress so
// the flag set can be asserted without exec. `-EncodeWith CPU` is hardcoded per
// the fork's GPU-path removal (see type doc). `-Quality 1.0` matches the fork
// README's recommended invocation and is the setting the bc7e integration was
// verified against.
func compressonatorArgs(format string, generateMips bool, inputPath, outputPath string) []string {
	args := []string{
		"-fd", format,
		"-EncodeWith", "CPU",
		"-Quality", "1.0",
		"-noprogress",
	}
	if generateMips {
		// -mipsize 1 asks Compressonator to build the chain down to a 1-pixel
		// minimum dimension, rebuilding every level from level 0. texconv's Run
		// reaches the same behavior by flattening the source first, so both
		// backends produce a chain of their own making.
		args = append(args, "-mipsize", "1")
	} else {
		args = append(args, "-nomipmap")
	}
	return append(args, inputPath, outputPath)
}

// compressonatorFormat maps atak's texconv-style format strings onto the format
// tokens compressonatorcli's -fd flag expects. Only the five formats atak's
// profiles actually use are enumerated — an unrecognized value is a bug in
// upstream code that must fail loudly rather than silently pick a default.
func compressonatorFormat(f string) (string, error) {
	switch f {
	case "BC1_UNORM":
		return "BC1", nil
	case "BC3_UNORM":
		return "BC3", nil
	case "BC4_UNORM":
		return "BC4", nil
	case "BC5_UNORM":
		return "BC5", nil
	case "BC7_UNORM":
		return "BC7", nil
	}
	return "", fmt.Errorf("compressonator: unsupported format %q", f)
}
