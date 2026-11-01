package compress

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noisethanks/atak/internal/scan"
)

// recordingBackend writes "compressed" to <OutputDir>/<stem>.dds and records the
// paths it was handed.
type recordingBackend struct {
	gotSrc, gotOut string
}

func (b *recordingBackend) Name() string { return "recording" }
func (b *recordingBackend) Compress(ctx context.Context, job Job) CompressionResult {
	b.gotSrc, b.gotOut = job.Asset.Path, job.OutputDir
	base := filepath.Base(job.Asset.Path)
	out := filepath.Join(job.OutputDir, strings.TrimSuffix(base, filepath.Ext(base))+".dds")
	if err := os.WriteFile(out, []byte("compressed"), 0644); err != nil {
		return CompressionResult{Asset: job.Asset, Err: err}
	}
	return CompressionResult{Asset: job.Asset, Backend: b.Name(), Success: true}
}

func TestDispatchNonASCIIPaths(t *testing.T) {
	root := t.TempDir()
	uni := filepath.Join(root, "xx-CLIKRΔD ROMEO Menu")
	plain := filepath.Join(root, "plain")
	for _, d := range []string{uni, plain} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("in-place non-ASCII dir", func(t *testing.T) {
		src := filepath.Join(uni, "Tex.DDS")
		os.WriteFile(src, []byte("source"), 0644)
		b := &recordingBackend{}
		r := dispatch(context.Background(), b, nil, Job{Asset: scan.Asset{Path: src}, Format: "BC1_UNORM", OutputDir: uni})
		if !r.Success {
			t.Fatalf("failed: %v", r.Err)
		}
		if !isASCII(b.gotSrc) || !isASCII(b.gotOut) {
			t.Fatalf("backend saw non-ASCII paths: %q %q", b.gotSrc, b.gotOut)
		}
		if filepath.Ext(b.gotSrc) != ".DDS" {
			t.Errorf("extension case not preserved: %q", b.gotSrc)
		}
		if r.Asset.Path != src {
			t.Errorf("result asset path = %q, want original", r.Asset.Path)
		}
		if got, _ := os.ReadFile(src); string(got) != "compressed" {
			t.Errorf("destination not replaced, got %q", got)
		}
		assertNoStaging(t, b.gotSrc)
	})

	t.Run("non-ASCII source, ASCII mod output", func(t *testing.T) {
		src := filepath.Join(uni, "a.dds")
		os.WriteFile(src, []byte("source"), 0644)
		b := &recordingBackend{}
		r := dispatch(context.Background(), b, nil, Job{Asset: scan.Asset{Path: src}, Format: "BC1_UNORM", OutputDir: plain})
		if !r.Success {
			t.Fatalf("failed: %v", r.Err)
		}
		if got, _ := os.ReadFile(filepath.Join(plain, "a.dds")); string(got) != "compressed" {
			t.Errorf("output missing, got %q", got)
		}
		if got, _ := os.ReadFile(src); string(got) != "source" {
			t.Errorf("source modified: %q", got)
		}
	})

	t.Run("ASCII source, non-ASCII output dir", func(t *testing.T) {
		src := filepath.Join(plain, "b.dds")
		os.WriteFile(src, []byte("source"), 0644)
		out := filepath.Join(uni, "out")
		b := &recordingBackend{}
		r := dispatch(context.Background(), b, nil, Job{Asset: scan.Asset{Path: src}, Format: "BC1_UNORM", OutputDir: out})
		if !r.Success {
			t.Fatalf("failed: %v", r.Err)
		}
		if got, _ := os.ReadFile(filepath.Join(out, "b.dds")); string(got) != "compressed" {
			t.Errorf("output missing, got %q", got)
		}
	})

	t.Run("ASCII mod output stages on the destination filesystem", func(t *testing.T) {
		src := filepath.Join(plain, "c.dds")
		os.WriteFile(src, []byte("source"), 0644)
		modOut := filepath.Join(root, "modout")
		b := &recordingBackend{}
		r := dispatch(context.Background(), b, nil, Job{Asset: scan.Asset{Path: src}, Format: "BC1_UNORM", OutputDir: modOut})
		if !r.Success {
			t.Fatalf("failed: %v", r.Err)
		}
		if b.gotSrc != src {
			t.Errorf("ASCII source was copied: %q", b.gotSrc)
		}
		if !strings.HasPrefix(b.gotOut, filepath.Join(modOut, stagingDirName)+string(filepath.Separator)) {
			t.Errorf("output not staged under the mod folder: %q", b.gotOut)
		}
		if got, _ := os.ReadFile(filepath.Join(modOut, "c.dds")); string(got) != "compressed" {
			t.Errorf("output missing, got %q", got)
		}
		if _, err := os.Stat(filepath.Join(modOut, stagingDirName)); !os.IsNotExist(err) {
			t.Errorf("staging dir left behind: %v", err)
		}
	})
}

// assertNoStaging checks the temp dir holding the staged source was removed.
func assertNoStaging(t *testing.T, stagedSrc string) {
	t.Helper()
	if _, err := os.Stat(stagedSrc); !os.IsNotExist(err) {
		t.Errorf("staging temp file left behind: %s", stagedSrc)
	}
}

// Real texconv on a non-ASCII directory: the regression this staging exists for.
func TestTexconvNonASCIIPathReal(t *testing.T) {
	tex := repoToolPath(t, "texconv")
	dir := filepath.Join(t.TempDir(), "xx-CLIKRΔD ROMEO Menu - Victor")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "tex.dds")
	if err := os.WriteFile(src, synthSrgbDX10DDS(true), 0644); err != nil {
		t.Fatal(err)
	}
	r := dispatch(context.Background(), NewTexconvBackend(tex), nil, Job{
		Asset: scan.Asset{Path: src, Width: 4, Height: 4}, Format: "BC1_UNORM", OutputDir: dir,
	})
	if !r.Success {
		t.Fatalf("texconv failed on non-ASCII path: %v\n%s", r.Err, r.Stderr)
	}
}
