package scan

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// writeDX10DDS builds a minimal DDS whose DX10 extended header advertises the
// given DXGI format code, and returns its path. Only the headers are written —
// ParseDDS never reads past byte 148, so pixel data would only pad the fixture.
func writeDX10DDS(t *testing.T, dxgiFormat uint32) string {
	t.Helper()
	var b bytes.Buffer
	w := func(v uint32) { _ = binary.Write(&b, binary.LittleEndian, v) }

	b.Write([]byte("DDS "))
	w(124)    // dwSize
	w(0x1007) // caps | height | width | pixelformat
	w(4)      // height
	w(4)      // width
	w(0)      // pitch
	w(1)      // depth
	w(1)      // mipMapCount
	for i := 0; i < 11; i++ {
		w(0) // reserved1
	}
	w(32)         // pf size
	w(ddpfFourCC) // pf flags
	b.Write([]byte("DX10"))
	for i := 0; i < 5; i++ {
		w(0) // bit counts and masks unused under a DX10 FourCC
	}
	w(0x1000) // caps
	w(0)      // caps2
	w(0)      // caps3
	w(0)      // caps4
	w(0)      // reserved2

	// DDS_HEADER_DXT10
	w(dxgiFormat)
	w(3) // resource dimension: texture2D
	w(0) // miscFlag
	w(1) // arraySize
	w(0) // miscFlags2

	path := filepath.Join(t.TempDir(), "fixture.dds")
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestDXGIFormatCodes pins every BCn entry to the code DXGI actually assigns it.
// The table used to be shifted by one from code 73 upward, which mislabeled four
// formats and — the part that changes behavior — claimed 85 was BC5_SNORM. 85 is
// B5G6R5_UNORM, an uncompressed format, so a file using it was reported as
// already compressed and skipped instead of being compressed.
func TestDXGIFormatCodes(t *testing.T) {
	tests := []struct {
		code       uint32
		wantFormat string
		wantComp   bool
	}{
		{70, "BC1_UNORM", true},      // BC1_TYPELESS
		{71, "BC1_UNORM", true},      // BC1_UNORM
		{72, "BC1_UNORM_SRGB", true}, // BC1_UNORM_SRGB
		{73, "BC2_UNORM", true},      // BC2_TYPELESS — was missing entirely
		{74, "BC2_UNORM", true},      // BC2_UNORM
		{75, "BC2_UNORM_SRGB", true}, // BC2_UNORM_SRGB
		{76, "BC3_UNORM", true},      // BC3_TYPELESS — was reported as BC2
		{77, "BC3_UNORM", true},      // BC3_UNORM
		{78, "BC3_UNORM_SRGB", true}, // BC3_UNORM_SRGB
		{79, "BC4_UNORM", true},      // BC4_TYPELESS — was reported as BC3
		{80, "BC4_UNORM", true},      // BC4_UNORM
		{81, "BC4_SNORM", true},      // BC4_SNORM — was reported as BC4_UNORM
		{82, "BC5_UNORM", true},      // BC5_TYPELESS — was reported as BC4_SNORM
		{83, "BC5_UNORM", true},      // BC5_UNORM
		{84, "BC5_SNORM", true},      // BC5_SNORM — was reported as BC5_UNORM
		{94, "BC6H_UF16", true},      // BC6H_TYPELESS
		{95, "BC6H_UF16", true},      // BC6H_UF16 — was reported as SF16
		{96, "BC6H_SF16", true},      // BC6H_SF16
		{97, "BC7_UNORM", true},      // BC7_TYPELESS
		{98, "BC7_UNORM", true},      // BC7_UNORM
		{99, "BC7_UNORM_SRGB", true}, // BC7_UNORM_SRGB

		// Uncompressed codes next to the BCn block. A wrong "compressed" verdict
		// here means the file is never compressed and never reported.
		{85, "DXGI_85", false}, // B5G6R5_UNORM
		{87, "DXGI_87", false}, // B8G8R8A8_UNORM
		{28, "DXGI_28", false}, // R8G8B8A8_UNORM
	}

	for _, tc := range tests {
		info, err := ParseDDS(writeDX10DDS(t, tc.code))
		if err != nil {
			t.Fatalf("code %d: ParseDDS: %v", tc.code, err)
		}
		if info.Format != tc.wantFormat {
			t.Errorf("code %d: Format = %q, want %q", tc.code, info.Format, tc.wantFormat)
		}
		if info.Compressed != tc.wantComp {
			t.Errorf("code %d: Compressed = %v, want %v", tc.code, info.Compressed, tc.wantComp)
		}
	}
}

// TestBytesPerTexelCoversParsedFormats guards the pairing between the two tables:
// every name ParseDDS can report has to be priced, or VRAM estimates silently
// fall back to 4 bytes per texel and overstate a BC1 texture eightfold.
func TestBytesPerTexelCoversParsedFormats(t *testing.T) {
	want := map[string]float64{
		"BC1_UNORM": 0.5, "BC1_UNORM_SRGB": 0.5,
		"BC2_UNORM": 1, "BC2_UNORM_SRGB": 1,
		"BC3_UNORM": 1, "BC3_UNORM_SRGB": 1,
		"BC4_UNORM": 0.5, "BC4_SNORM": 0.5,
		"BC5_UNORM": 1, "BC5_SNORM": 1,
		"BC6H_UF16": 1, "BC6H_SF16": 1,
		"BC7_UNORM": 1, "BC7_UNORM_SRGB": 1,
	}
	for format, size := range want {
		if got := bytesPerTexelFloat(format); got != size {
			t.Errorf("bytesPerTexelFloat(%q) = %v, want %v", format, got, size)
		}
	}
}
