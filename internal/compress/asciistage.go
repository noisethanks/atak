package compress

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// isASCII reports whether s contains only 7-bit bytes.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// needsASCIIStaging reports whether a job's source or output directory has a
// non-ASCII byte in its path. The Linux texconv port converts argv without
// setlocale, so such paths arrive mangled and fail ("This format requires WIC").
// Compressonator is staged too for consistency, though its known failures on
// these files are DDS-reader gaps, not path related.
func needsASCIIStaging(job Job) bool {
	return !isASCII(job.Asset.Path) || !isASCII(job.OutputDir)
}

// withASCIIPaths runs run on a job whose source and output paths are plain
// ASCII, then moves the result to the real destination from Go, which handles
// UTF-8 paths correctly. Jobs whose paths are already ASCII go straight to run.
//
// Source and output are staged in separate subdirectories so a ".dds" source
// can't be overwritten by its own output. The source temp file keeps the
// original extension, case included, so the backends' extension handling is
// unchanged. The temp dir is removed on every exit path; the real destination
// is only touched after the backend reports success, so a failed or cancelled
// run leaves the source file as it was.
func withASCIIPaths(job Job, run func(Job) CompressionResult) CompressionResult {
	if !needsASCIIStaging(job) {
		return run(job)
	}

	tmpDir, err := os.MkdirTemp("", "atak-ascii-")
	if err != nil {
		return CompressionResult{Asset: job.Asset, Err: err}
	}
	defer os.RemoveAll(tmpDir)

	inDir := filepath.Join(tmpDir, "in")
	outDir := filepath.Join(tmpDir, "out")
	for _, d := range []string{inDir, outDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return CompressionResult{Asset: job.Asset, Err: err}
		}
	}
	tmpSrc := filepath.Join(inDir, "src"+filepath.Ext(job.Asset.Path))
	if err := copyFile(job.Asset.Path, tmpSrc); err != nil {
		return CompressionResult{Asset: job.Asset, Err: err}
	}

	inPlace := filepath.Dir(job.Asset.Path) == job.OutputDir
	inner := job
	inner.Asset.Path = tmpSrc
	inner.OutputDir = outDir
	r := run(inner)
	r.Asset = job.Asset
	if !r.Success {
		return r
	}

	entries, err := os.ReadDir(outDir)
	if err != nil || len(entries) != 1 {
		r.Success = false
		r.Err = fmt.Errorf("ascii staging: expected one output file, found %d (%v)", len(entries), err)
		return r
	}
	produced := filepath.Join(outDir, entries[0].Name())
	dest := job.Asset.Path
	if !inPlace {
		base := filepath.Base(job.Asset.Path)
		dest = filepath.Join(job.OutputDir, strings.TrimSuffix(base, filepath.Ext(base))+filepath.Ext(produced))
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err == nil {
		err = moveFile(produced, dest)
	}
	if err != nil {
		r.Success = false
		r.Err = err
	}
	return r
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// moveFile renames, falling back to copy+remove when src and dst are on
// different filesystems (the temp dir often is).
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}
