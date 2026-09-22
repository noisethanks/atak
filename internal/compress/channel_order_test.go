package compress

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/noisethanks/atak/internal/scan"
)

// synth24BitDDS builds an uncompressed 24-bit DDS filled with one color. The
// masks are the ones every 24-bit DDS in the corpus uses — red 0x00ff0000, green
// 0x0000ff00, blue 0x000000ff — which puts the bytes on disk in B, G, R order.
// 4x4 is the smallest size that needs no block-alignment resize, so a job built
// from this fixture reaches the encoder by the plain path.
func synth24BitDDS(width, height int, r, g, b byte) []byte {
	const (
		ddsdCaps        = 0x1
		ddsdHeight      = 0x2
		ddsdWidth       = 0x4
		ddsdPitch       = 0x8
		ddsdPixelFormat = 0x1000
		ddpfRGB         = 0x40
		ddscapsTexture  = 0x1000
	)

	var buf bytes.Buffer
	w := func(v uint32) { _ = binary.Write(&buf, binary.LittleEndian, v) }

	buf.Write([]byte("DDS "))
	w(124)
	w(ddsdCaps | ddsdHeight | ddsdWidth | ddsdPitch | ddsdPixelFormat)
	w(uint32(height))
	w(uint32(width))
	w(uint32(width * 3)) // pitch
	w(1)                 // depth
	w(1)                 // mipMapCount
	for i := 0; i < 11; i++ {
		w(0) // reserved1
	}
	// DDS_PIXELFORMAT
	w(32)
	w(ddpfRGB)
	w(0)          // fourCC
	w(24)         // rgbBitCount
	w(0x00ff0000) // red mask
	w(0x0000ff00) // green mask
	w(0x000000ff) // blue mask
	w(0)          // alpha mask
	w(ddscapsTexture)
	w(0) // caps2
	w(0) // caps3
	w(0) // caps4
	w(0) // reserved2

	for i := 0; i < width*height; i++ {
		buf.Write([]byte{b, g, r})
	}
	return buf.Bytes()
}

// decodeTopLeftRGBA converts a compressed DDS back to uncompressed RGBA with
// texconv and returns the top-left pixel. Decoding is the only way to see what
// the encoder actually stored — the header alone reports the format, not the
// channel order.
func decodeTopLeftRGBA(t *testing.T, texconvBin, path string) (r, g, b, a byte) {
	t.Helper()
	outDir := t.TempDir()
	cmd := exec.Command(texconvBin,
		"-f", "R8G8B8A8_UNORM", "-m", "1", "-y", "-nologo", "-o", outDir, "--", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("texconv decode: %v\n%s", err, out)
	}
	base := filepath.Base(path)
	decoded := filepath.Join(outDir, base[:len(base)-len(filepath.Ext(base))]+".dds")
	data, err := os.ReadFile(decoded)
	if err != nil {
		t.Fatalf("read decoded: %v", err)
	}

	// Pixel data starts after the 128-byte header, plus 20 more when the file
	// carries a DX10 extended header.
	offset := 128
	if len(data) > 88 && string(data[84:88]) == "DX10" {
		offset = 148
	}
	if len(data) < offset+4 {
		t.Fatalf("decoded file too short: %d bytes", len(data))
	}
	px := data[offset : offset+4]
	return px[0], px[1], px[2], px[3]
}

// TestCompressonator24BitChannelOrder is the regression guard for a silent
// corruption: compressonator-bc7e reads a 24-bit DDS with the channels
// transposed, so every such source came out with red and blue swapped. Nothing
// failed and nothing was reported — the file compressed cleanly and looked wrong
// in game. In one GAMMA modlist it hit 109 outputs, food icons and sky textures
// among them.
//
// The fixture is pure red. A swap shows up as pure blue, which no amount of
// encoder error can explain.
func TestCompressonator24BitChannelOrder(t *testing.T) {
	compressBin := repoToolPath(t, "compressonator-bc7e")
	texconvBin := repoToolPath(t, "texconv")

	src := writeFixture(t, "red24.dds", synth24BitDDS(4, 4, 255, 0, 0))
	outDir := t.TempDir()
	job := Job{
		Asset: scan.Asset{
			Path:           src,
			CurrentFmt:     "R8G8B8_UNORM",
			Width:          4,
			Height:         4,
			SourceMipCount: 1,
		},
		Format:    "BC7_UNORM",
		OutputDir: outDir,
	}

	res := dispatch(context.Background(), NewCompressonatorBackend(compressBin), NewTexconvBackend(texconvBin), job)
	if !res.Success {
		t.Fatalf("compression failed: err=%v stderr=%s", res.Err, res.Stderr)
	}

	r, g, b, _ := decodeTopLeftRGBA(t, texconvBin, filepath.Join(outDir, "red24.dds"))
	if r < 250 || b > 5 {
		t.Errorf("24-bit source came out as R=%d G=%d B=%d, want red (R≈255, B≈0)", r, g, b)
	}
	if res.FallbackReason != FallbackChannelOrder {
		t.Errorf("FallbackReason = %q, want %q — the swap fix has to be attributable in the summary",
			res.FallbackReason, FallbackChannelOrder)
	}
}

// TestChannelFixFailsClosedWithoutDimensions covers the case where the cheap fix
// is unavailable. A resample needs both axes, and a DDS header can carry a zero
// there — ParseDDS reports it rather than failing — so a 24-bit source with no
// usable size cannot take the split. Handing it back to the primary would quietly
// restore the swap, so the whole job goes to texconv instead: slower, correct.
func TestChannelFixFailsClosedWithoutDimensions(t *testing.T) {
	compressBin := repoToolPath(t, "compressonator-bc7e")
	texconvBin := repoToolPath(t, "texconv")

	src := writeFixture(t, "nodims.dds", synth24BitDDS(4, 4, 255, 0, 0))
	outDir := t.TempDir()
	job := Job{
		Asset: scan.Asset{
			Path:       src,
			CurrentFmt: "R8G8B8_UNORM",
			// Width and Height deliberately left at zero.
			SourceMipCount: 1,
		},
		Format:    "BC7_UNORM",
		OutputDir: outDir,
	}

	res := dispatch(context.Background(), NewCompressonatorBackend(compressBin), NewTexconvBackend(texconvBin), job)
	if !res.Success {
		t.Fatalf("compression failed: err=%v stderr=%s", res.Err, res.Stderr)
	}
	r, g, b, _ := decodeTopLeftRGBA(t, texconvBin, filepath.Join(outDir, "nodims.dds"))
	if r < 250 || b > 5 {
		t.Errorf("unsized 24-bit source came out as R=%d G=%d B=%d, want red", r, g, b)
	}
	if res.Backend != "texconv" {
		t.Errorf("Backend = %q, want texconv — the whole job has to leave the transposing encoder", res.Backend)
	}
}

// TestSplitPathKeepsSourceExtensionCase covers a trap in the split path: texconv
// always writes a lowercase .dds, so the intermediate for a Foo.DDS source is
// foo.dds, and the encoder names its output after the file it read. The output
// has to carry the source's own spelling back.
//
// Two real consequences if it does not. In place, the compressed file lands
// beside an untouched Foo.DDS. In mod-output mode, the job records Foo.DDS while
// disk holds foo.dds, so worker.go's incremental skip misses it forever and the
// file is re-encoded on every run.
func TestSplitPathKeepsSourceExtensionCase(t *testing.T) {
	compressBin := repoToolPath(t, "compressonator-bc7e")
	texconvBin := repoToolPath(t, "texconv")

	// 24-bit, so the job takes the split path for the channel-order conversion.
	src := writeFixture(t, "Upper.DDS", synth24BitDDS(4, 4, 255, 0, 0))
	outDir := t.TempDir()
	job := Job{
		Asset: scan.Asset{
			Path:           src,
			CurrentFmt:     "R8G8B8_UNORM",
			Width:          4,
			Height:         4,
			SourceMipCount: 1,
		},
		Format:    "BC7_UNORM",
		OutputDir: outDir,
	}

	res := dispatch(context.Background(), NewCompressonatorBackend(compressBin), NewTexconvBackend(texconvBin), job)
	if !res.Success {
		t.Fatalf("compression failed: err=%v stderr=%s", res.Err, res.Stderr)
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "Upper.DDS" {
		t.Errorf("output directory holds %v, want exactly [Upper.DDS]", names)
	}
}

// TestEveryFallbackReasonHasALabel keeps the summary screen legible. FallbackLabel
// returns its argument unchanged for a reason it does not know, so a new constant
// added without a label shows the user a raw slug like "channel-order" instead of
// a sentence. That is exactly what happened when this constant was added.
func TestEveryFallbackReasonHasALabel(t *testing.T) {
	for _, reason := range []string{
		FallbackBC7ToBC3,
		FallbackResize,
		FallbackReaderGap,
		FallbackBlockAlign,
		FallbackChannelOrder,
	} {
		if FallbackLabel(reason) == reason {
			t.Errorf("FallbackLabel(%q) returned the slug unchanged — add a case", reason)
		}
	}
}

// TestTexconv24BitChannelOrderUnaffected pins the other side of the diagnosis:
// texconv reads the same fixture correctly. Without this, a future change could
// "fix" the swap by moving it into the resample step and no test would notice.
func TestTexconv24BitChannelOrderUnaffected(t *testing.T) {
	texconvBin := repoToolPath(t, "texconv")

	src := writeFixture(t, "red24.dds", synth24BitDDS(4, 4, 255, 0, 0))
	outDir := t.TempDir()
	job := Job{
		Asset: scan.Asset{
			Path:           src,
			CurrentFmt:     "R8G8B8_UNORM",
			Width:          4,
			Height:         4,
			SourceMipCount: 1,
		},
		Format:    "BC7_UNORM",
		OutputDir: outDir,
	}

	res := dispatch(context.Background(), NewTexconvBackend(texconvBin), nil, job)
	if !res.Success {
		t.Fatalf("compression failed: err=%v stderr=%s", res.Err, res.Stderr)
	}
	r, g, b, _ := decodeTopLeftRGBA(t, texconvBin, filepath.Join(outDir, "red24.dds"))
	if r < 250 || b > 5 {
		t.Errorf("texconv turned a red 24-bit source into R=%d G=%d B=%d", r, g, b)
	}
}
