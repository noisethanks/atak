package compress

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/noisethanks/atak/internal/scan"
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

// isInPlace reports whether a job writes its output beside the source, so the
// output name can collide with the source file itself.
func isInPlace(job Job) bool {
	return filepath.Dir(job.Asset.Path) == job.OutputDir
}

// stagingDirName is the hidden directory jobs stage output in, so the final move
// is a same-filesystem rename. The leading dot matches the ".*" entry in the
// default scanExclusions, so the scanner prunes it.
const stagingDirName = scan.StagingDirName

// makeStagingDir creates this job's private staging directory. It lives under
// <dest root>/.atak-staging, where dest root is the mod output folder in Mod
// Output Mode and the source's directory in place, so moving the result onto
// its destination is a rename and not a copy. A staging path with a non-ASCII
// byte would hand the backend a path it mangles, so such jobs stage under the
// OS temp dir instead and rely on moveFile's atomic copy.
//
// The shared .atak-staging parent is removed by cleanupStaging once empty, so
// concurrent jobs can race on it; creation retries when a sibling job removes
// the parent between MkdirAll and MkdirTemp.
func makeStagingDir(job Job) (jobDir, parent string, err error) {
	root := job.ModOutputDir
	if root == "" {
		root = job.OutputDir
	}
	if isInPlace(job) {
		root = filepath.Dir(job.Asset.Path)
	}
	parent = filepath.Join(root, stagingDirName)
	if !isASCII(parent) {
		jobDir, err = os.MkdirTemp("", "atak-stage-")
		return jobDir, "", err
	}
	for i := 0; i < 5; i++ {
		if err = os.MkdirAll(parent, 0755); err != nil {
			return "", "", err
		}
		if jobDir, err = os.MkdirTemp(parent, "job-"); err == nil {
			return jobDir, parent, nil
		}
	}
	return "", "", err
}

// cleanupStaging removes a job's staging directory and, once the last job in the
// directory is done, the shared .atak-staging parent. Remove fails harmlessly
// while another job still has a directory in it.
func cleanupStaging(jobDir, parent string) {
	os.RemoveAll(jobDir)
	if parent != "" {
		os.Remove(parent)
	}
}

// destPath is the final file a job produces, named from the job and never from
// whatever the backend wrote: the source's own path in place, RelPath in Mod
// Output Mode. Case is preserved exactly, so Foo.DDS stays Foo.DDS. It is also
// the path worker.go probes to skip a file already compressed.
func destPath(job Job) string {
	switch {
	case isInPlace(job):
		return job.Asset.Path
	case job.ModOutputDir != "" && job.RelPath != "":
		return filepath.Join(job.ModOutputDir, job.RelPath)
	}
	return filepath.Join(job.OutputDir, filepath.Base(job.Asset.Path))
}

// withStaging runs run on a job whose output goes to a private staging
// directory, then moves the result to the real destination from Go, which
// handles UTF-8 paths correctly. Every job is staged, in place or in Mod Output
// Mode.
//
// The invariant: a final destination is only ever written by a completed
// rename. A backend may delete or truncate its output path (compressonator
// removes it before encoding, texconv overwrites it, and in place that path IS
// the source), so none ever gets a final path. The source is read-only for the
// whole run; after the backend reports success, moveFile replaces the
// destination in one rename. A cancel, failure or crash leaves the source
// byte-identical and no partial file at the destination; only staging is
// discarded. In place the final file keeps the source's exact name, whatever
// case the backend gave its output.
//
// The source is copied to a plain-ASCII temp name only when its path is
// non-ASCII (the Linux texconv port converts argv without setlocale, so such
// paths arrive mangled and fail with "This format requires WIC"). The copy keeps
// the original extension, case included, so the backends' extension handling is
// unchanged. Source and output are staged in separate subdirectories so a ".dds"
// source can't be overwritten by its own output.
func withStaging(job Job, run func(Job) CompressionResult) CompressionResult {
	tmpDir, parent, err := makeStagingDir(job)
	if err != nil {
		return CompressionResult{Asset: job.Asset, Err: err}
	}
	defer cleanupStaging(tmpDir, parent)

	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return CompressionResult{Asset: job.Asset, Err: err}
	}
	inner := job
	inner.OutputDir = outDir
	if !isASCII(job.Asset.Path) {
		inDir := filepath.Join(tmpDir, "in")
		if err := os.MkdirAll(inDir, 0755); err != nil {
			return CompressionResult{Asset: job.Asset, Err: err}
		}
		tmpSrc := filepath.Join(inDir, "src"+filepath.Ext(job.Asset.Path))
		if err := copyFile(job.Asset.Path, tmpSrc); err != nil {
			return CompressionResult{Asset: job.Asset, Err: err}
		}
		inner.Asset.Path = tmpSrc
	}

	r := run(inner)
	r.Asset = job.Asset
	if !r.Success {
		return r
	}

	entries, err := os.ReadDir(outDir)
	if err != nil || len(entries) != 1 {
		r.Success = false
		r.Err = fmt.Errorf("staging: expected one output file, found %d (%v)", len(entries), err)
		return r
	}
	produced := filepath.Join(outDir, entries[0].Name())
	dest := destPath(job)
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

// moveFile replaces dst with src atomically. Rename is atomic on one
// filesystem. Across filesystems (the temp dir often is one) the data is copied
// to a sibling of dst first and renamed into place, so dst is never observable
// half-written.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	sibling := dst + ".atak-tmp"
	if err := copyFile(src, sibling); err != nil {
		os.Remove(sibling)
		return err
	}
	if err := os.Rename(sibling, dst); err != nil {
		os.Remove(sibling)
		return err
	}
	return os.Remove(src)
}
