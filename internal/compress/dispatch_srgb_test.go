package compress

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/noisethanks/atak/internal/scan"
)

// repoToolPath resolves one of the embedded tool binaries straight from
// internal/tools/bin for the platform running the test. These tests shell out
// to a real binary, and the embedded copy only becomes a file at runtime via
// tools.Extract, so the source tree is the one location that always has it —
// and it keeps the test running on every platform that ships the tool instead
// of only on the machine that wrote the path.
func repoToolPath(t *testing.T, stem string) string {
	t.Helper()
	var suffix string
	switch runtime.GOOS {
	case "darwin":
		suffix = "-macos"
	case "linux":
		suffix = "-linux"
	case "windows":
		suffix = "-windows.exe"
	default:
		t.Skipf("no %s build for %s", stem, runtime.GOOS)
	}
	path, err := filepath.Abs(filepath.Join("..", "tools", "bin", stem+suffix))
	if err != nil {
		t.Skipf("resolve %s: %v", stem, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("binary missing: %v", err)
	}
	return path
}

// synthSrgbDX10DDS builds a minimal, self-contained DDS with a DX10 extended
// header advertising dxgiFormat = 91 (DXGI_FORMAT_B8G8R8A8_UNORM_SRGB) — the
// exact subvariant compressonator-bc7e's DDS reader rejects but DirectXTex/
// texconv accepts. Kept synthetic (no real texture data — 4x4 = 64 pixel bytes
// on top of ~148 bytes of headers) so the regression fixture stays checked in
// as code, not as a binary blob, and doesn't inflate the repo or the embedded
// binary. When withPixels is false the file is truncated after the DX10
// header, giving a fixture both backends reject (used for the double-failure
// path).
func synthSrgbDX10DDS(withPixels bool) []byte {
	const (
		width, height = 4, 4

		ddsdCaps        = 0x1
		ddsdHeight      = 0x2
		ddsdWidth       = 0x4
		ddsdPitch       = 0x8
		ddsdPixelFormat = 0x1000
		ddsdMipMapCount = 0x20000

		ddpfFourCC = 0x4

		ddscapsTexture = 0x1000

		dxgiB8G8R8A8UnormSrgb = 91
		ddsDimensionTexture2D = 3
	)

	var b bytes.Buffer
	b.Write([]byte("DDS "))
	w := func(v uint32) { _ = binary.Write(&b, binary.LittleEndian, v) }

	// DDS_HEADER (124 bytes)
	w(124) // dwSize
	w(ddsdCaps | ddsdHeight | ddsdWidth | ddsdPitch | ddsdPixelFormat | ddsdMipMapCount)
	w(height)
	w(width)
	w(width * 4) // pitch = width * bytes-per-pixel for uncompressed BGRA
	w(1)         // depth
	w(1)         // mipMapCount
	for i := 0; i < 11; i++ {
		w(0) // reserved1
	}
	// DDS_PIXELFORMAT
	w(32)          // pf size
	w(ddpfFourCC)  // pf flags
	b.Write([]byte("DX10"))
	for i := 0; i < 5; i++ {
		w(0) // RGBA masks unused when FourCC is DX10
	}
	// caps
	w(ddscapsTexture)
	w(0) // caps2
	w(0) // caps3
	w(0) // caps4
	w(0) // reserved2

	// DDS_HEADER_DXT10 (20 bytes)
	w(dxgiB8G8R8A8UnormSrgb)
	w(ddsDimensionTexture2D)
	w(0) // miscFlag
	w(1) // arraySize
	w(0) // miscFlags2

	if withPixels {
		b.Write(make([]byte, width*height*4)) // zeroed BGRA pixels
	}
	return b.Bytes()
}

func writeFixture(t *testing.T, name string, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestDispatchCompressonatorSrgbFallback exercises the narrow-match fallback
// in dispatch(): a DX10-header DDS tagged DXGI_FORMAT_B8G8R8A8_UNORM_SRGB
// (91) is rejected by compressonator-bc7e's DDS reader with "Could not load
// source file"; dispatch retries via texconv, which accepts it. The
// synthesized fixture stays in-tree so this regression can't recur silently.
func TestDispatchCompressonatorSrgbFallback(t *testing.T) {
	compressBin := repoToolPath(t, "compressonator-bc7e")
	texconvBin := repoToolPath(t, "texconv")

	src := writeFixture(t, "srgb_dx10.dds", synthSrgbDX10DDS(true))
	outDir := t.TempDir()
	job := Job{
		Asset:        scan.Asset{Path: src, Width: 4, Height: 4, SourceMipCount: 1},
		Format:       "BC7_UNORM",
		GenerateMips: false,
		OutputDir:    outDir,
	}

	r := dispatch(context.Background(), NewCompressonatorBackend(compressBin), NewTexconvBackend(texconvBin), job)

	if !r.Success {
		t.Fatalf("expected success via fallback, got failure: err=%v stderr=%s", r.Err, r.Stderr)
	}
	if r.Backend != "texconv" {
		t.Errorf("Backend on fallback-success: got %q, want %q", r.Backend, "texconv")
	}
	if r.FallbackReason != FallbackReaderGap {
		t.Errorf("FallbackReason on reader-gap fallback: got %q, want %q", r.FallbackReason, FallbackReaderGap)
	}
	if _, err := os.Stat(filepath.Join(outDir, "srgb_dx10.dds")); err != nil {
		t.Errorf("expected output file: %v", err)
	}
}

// TestDispatchDoubleFailureCombinedStderr covers the case where the fallback
// also rejects the file — expects a single normal per-file failure with
// combined stderr and a blanked Backend (neither backend produced output).
// Uses the same DX10 sRGB header but truncates before pixel data so texconv
// also fails.
func TestDispatchDoubleFailureCombinedStderr(t *testing.T) {
	compressBin := "/home/abhi/stalker-tex/internal/tools/bin/compressonator-bc7e-linux"
	texconvBin := "/home/abhi/stalker-tex/internal/tools/bin/texconv-linux"
	for _, p := range []string{compressBin, texconvBin} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("binary missing: %v", err)
		}
	}

	src := writeFixture(t, "srgb_dx10_truncated.dds", synthSrgbDX10DDS(false))
	job := Job{
		Asset:     scan.Asset{Path: src, Width: 4, Height: 4},
		Format:    "BC7_UNORM",
		OutputDir: t.TempDir(),
	}

	r := dispatch(context.Background(), NewCompressonatorBackend(compressBin), NewTexconvBackend(texconvBin), job)
	if r.Success {
		t.Fatal("expected failure on truncated DDS")
	}
	// Guard against a regression where the primary error string drifts and the
	// fallback path is silently skipped — that would leave stderr with no
	// texconv prefix and blank the double-failure semantics.
	if !strings.Contains(r.Stderr, "compressonator-bc7e:") || !strings.Contains(r.Stderr, "texconv:") {
		t.Errorf("expected combined stderr with both backend prefixes; got: %s", r.Stderr)
	}
	if r.Backend != "" {
		t.Errorf("Backend on double-failure: got %q, want %q (blank — neither backend produced output)", r.Backend, "")
	}
}
