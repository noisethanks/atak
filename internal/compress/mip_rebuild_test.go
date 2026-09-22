package compress

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/noisethanks/atak/internal/scan"
)

// mippedDDS builds an uncompressed 32-bit DDS carrying two mip levels, each a
// solid color of its own. Level 1 is the interesting one: a rebuilt chain
// derives it from level 0, so its color tells the two behaviors apart with no
// tolerance to argue about.
func mippedDDS(width, height int, top, second [3]byte) []byte {
	return rgbaDDS(width, height, [][3]byte{top, second})
}

// flatDDS builds the single-level case: a source with nothing to discard, which
// is the one Run must not pay a flatten pass for.
func flatDDS(width, height int, color [3]byte) []byte {
	return rgbaDDS(width, height, [][3]byte{color})
}

// rgbaDDS writes an uncompressed 32-bit DDS carrying one solid-color level per
// entry in levels, halving the dimensions each time.
func rgbaDDS(width, height int, levels [][3]byte) []byte {
	var buf bytes.Buffer
	w := func(v uint32) { _ = binary.Write(&buf, binary.LittleEndian, v) }

	const (
		ddsdCaps        = 0x1
		ddsdHeight      = 0x2
		ddsdWidth       = 0x4
		ddsdPitch       = 0x8
		ddsdPixelFormat = 0x1000
		ddsdMipMapCount = 0x20000
		ddpfRGB         = 0x40
		ddpfAlphaPixels = 0x1
		ddscapsTexture  = 0x1000
		ddscapsMipMap   = 0x400000
	)

	buf.Write([]byte("DDS "))
	w(124)
	w(ddsdCaps | ddsdHeight | ddsdWidth | ddsdPitch | ddsdPixelFormat | ddsdMipMapCount)
	w(uint32(height))
	w(uint32(width))
	w(uint32(width * 4))
	w(1)                   // depth
	w(uint32(len(levels))) // mipMapCount
	for i := 0; i < 11; i++ {
		w(0)
	}
	w(32)
	w(ddpfRGB | ddpfAlphaPixels)
	w(0)  // fourCC
	w(32) // rgbBitCount
	w(0x00ff0000)
	w(0x0000ff00)
	w(0x000000ff)
	w(0xff000000)
	w(ddscapsTexture | ddscapsMipMap)
	w(0)
	w(0)
	w(0)
	w(0)

	// Bytes land as B, G, R, A for the masks above.
	write := func(wpx, hpx int, c [3]byte) {
		for i := 0; i < wpx*hpx; i++ {
			buf.Write([]byte{c[2], c[1], c[0], 255})
		}
	}
	lw, lh := width, height
	for _, c := range levels {
		write(lw, lh, c)
		lw, lh = max(lw/2, 1), max(lh/2, 1)
	}
	return buf.Bytes()
}

// decodeLevels reports how many mip levels a DDS carries and the first pixel of
// each one. The level count comes from the file's own header, so the decode
// cannot influence it; the decode then asks texconv for exactly that many levels,
// which is the one request that makes texconv copy the levels through instead of
// generating any. Omitting -m entirely makes texconv build a full chain, which
// would have made every assertion here measure the decode rather than the output.
func decodeLevels(t *testing.T, texconvBin, path string) (int, [][4]byte) {
	t.Helper()
	info, err := scan.ParseDDS(path)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	outDir := t.TempDir()
	cmd := exec.Command(texconvBin, "-f", "R8G8B8A8_UNORM",
		"-m", strconv.Itoa(info.MipMapCount), "-y", "-nologo", "-o", outDir, "--", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("texconv decode: %v\n%s", err, out)
	}
	base := filepath.Base(path)
	data, err := os.ReadFile(filepath.Join(outDir, base[:len(base)-len(filepath.Ext(base))]+".dds"))
	if err != nil {
		t.Fatalf("read decoded: %v", err)
	}
	offset := 128
	if len(data) > 88 && string(data[84:88]) == "DX10" {
		offset = 148
	}

	var firsts [][4]byte
	w, h := info.Width, info.Height
	for i := 0; i < info.MipMapCount; i++ {
		if offset+4 > len(data) {
			break
		}
		firsts = append(firsts, [4]byte{data[offset], data[offset+1], data[offset+2], data[offset+3]})
		offset += w * h * 4
		w = max(w/2, 1)
		h = max(h/2, 1)
	}
	return info.MipMapCount, firsts
}

// TestRunRebuildsSourceMipChain is the behavior this was changed to have: a mip
// chain is the encoder's own, never the source's.
//
// texconv generates only the levels a source lacks, so asking it for a chain
// leaves an existing one exactly as shipped — a partial chain stays partial, and
// hand-drawn lower levels survive. compressonator-bc7e rebuilds every level from
// level 0. The two backends produced different output for the same input, so Run
// now flattens the source to its top level first and both agree.
//
// The fixture is red with a green level 1. Rebuilt means level 1 comes back red.
func TestRunRebuildsSourceMipChain(t *testing.T) {
	texconvBin := repoToolPath(t, "texconv")

	src := writeFixture(t, "mipped.dds", mippedDDS(8, 8, [3]byte{255, 0, 0}, [3]byte{0, 255, 0}))
	outDir := t.TempDir()
	asset := scan.Asset{
		Path:           src,
		CurrentFmt:     "R8G8B8A8_UNORM",
		Width:          8,
		Height:         8,
		SourceMipCount: 2,
	}

	res := Run(context.Background(), texconvBin, asset, "BC7_UNORM", true, 0, outDir)
	if !res.Success {
		t.Fatalf("compression failed: err=%v stderr=%s", res.Err, res.Stderr)
	}

	levels, firsts := decodeLevels(t, texconvBin, filepath.Join(outDir, "mipped.dds"))
	if levels != 4 {
		t.Errorf("output has %d mip levels, want 4 (8x8 down to 1x1) — a source chain was inherited", levels)
	}
	if len(firsts) < 2 {
		t.Fatalf("could not read level 1 from the output")
	}
	if firsts[0][0] < 250 {
		t.Errorf("level 0 = %v, want red", firsts[0])
	}
	if firsts[1][1] > 5 {
		t.Errorf("level 1 = %v, want red — the source's own green level survived the rebuild", firsts[1])
	}
}

// TestRunBuildsChainForSinglLevelSource pins the case that needs no second pass.
// A source with one level has nothing to discard, so texconv's own generation is
// already a rebuild and flattening it would cost an invocation for free.
func TestRunBuildsChainForSinglLevelSource(t *testing.T) {
	texconvBin := repoToolPath(t, "texconv")

	src := writeFixture(t, "flat.dds", flatDDS(8, 8, [3]byte{255, 0, 0}))
	outDir := t.TempDir()
	asset := scan.Asset{
		Path:           src,
		CurrentFmt:     "R8G8B8A8_UNORM",
		Width:          8,
		Height:         8,
		SourceMipCount: 1,
	}

	res := Run(context.Background(), texconvBin, asset, "BC7_UNORM", true, 0, outDir)
	if !res.Success {
		t.Fatalf("compression failed: err=%v stderr=%s", res.Err, res.Stderr)
	}
	levels, _ := decodeLevels(t, texconvBin, filepath.Join(outDir, "flat.dds"))
	if levels != 4 {
		t.Errorf("output has %d mip levels, want 4", levels)
	}
}

// TestRunKeepsSingleLevelWhenMipsAreOff checks the rebuild never fires for a
// profile that asks for no chain. Flattening there would be the same result by a
// slower road, and stripping a mipped source is already what generateMips:false
// plus stripMipsWhenDisabled means.
func TestRunKeepsSingleLevelWhenMipsAreOff(t *testing.T) {
	texconvBin := repoToolPath(t, "texconv")

	src := writeFixture(t, "mipped.dds", mippedDDS(8, 8, [3]byte{255, 0, 0}, [3]byte{0, 255, 0}))
	outDir := t.TempDir()
	asset := scan.Asset{
		Path:           src,
		CurrentFmt:     "R8G8B8A8_UNORM",
		Width:          8,
		Height:         8,
		SourceMipCount: 2,
	}

	res := Run(context.Background(), texconvBin, asset, "BC7_UNORM", false, 0, outDir)
	if !res.Success {
		t.Fatalf("compression failed: err=%v stderr=%s", res.Err, res.Stderr)
	}
	levels, _ := decodeLevels(t, texconvBin, filepath.Join(outDir, "mipped.dds"))
	if levels != 1 {
		t.Errorf("output has %d mip levels, want 1", levels)
	}
}

func TestNeedsMipRebuild(t *testing.T) {
	tests := []struct {
		generateMips bool
		sourceMips   int
		want         bool
	}{
		{true, 11, true}, // full chain: rebuild, or the source's own levels survive
		{true, 7, true},  // partial chain: rebuild completes it to 1x1
		{true, 2, true},
		{true, 1, false}, // nothing to discard; texconv generates from level 0 anyway
		{true, 0, false}, // unparsed count, treat as single level
		{false, 11, false},
		{false, 1, false},
	}
	for _, tt := range tests {
		if got := needsMipRebuild(tt.generateMips, tt.sourceMips); got != tt.want {
			t.Errorf("needsMipRebuild(%v, %d) = %v, want %v",
				tt.generateMips, tt.sourceMips, got, tt.want)
		}
	}
}
