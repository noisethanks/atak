package scan

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

// DDS magic and header layout constants.
const (
	ddsMagic       = 0x20534444 // "DDS "
	ddsHeaderSize  = 124
	ddpfFourCC     = 0x4
	ddpfRGB        = 0x40
	ddpfAlphaPixel = 0x1
)

// Known FourCC codes mapped to format strings.
var fourCCFormats = map[uint32]string{
	0x31545844: "BC1_UNORM",  // DXT1
	0x33545844: "BC2_UNORM",  // DXT3
	0x35545844: "BC3_UNORM",  // DXT5
	0x31495441: "BC4_UNORM",  // ATI1 / BC4
	0x55344342: "BC4_UNORM",  // BC4U — texconv BC4
	0x53344342: "BC4_SNORM",  // BC4S — texconv BC4 signed
	0x32495441: "BC5_UNORM",  // ATI2 / BC5
	0x55354342: "BC5_UNORM",  // BC5U — texconv BC5
	0x53354342: "BC5_SNORM",  // BC5S — texconv BC5 signed
	0x30315844: "",           // DX10 — resolved via extended header
}

// DXGI format codes that map to BCn compressed formats. The codes come from the
// DXGI_FORMAT enum, where each family runs TYPELESS, UNORM, then a third variant:
// _UNORM_SRGB for BC1 to BC3 and BC7, _SNORM for BC4 and BC5. A TYPELESS code
// reports the UNORM name, because the file holds the same block data either way
// and nothing downstream separates them.
//
// The BCn codes are contiguous only up to 84. 85 to 93 are uncompressed formats
// (B5G6R5_UNORM at 85, B8G8R8A8_UNORM at 87) and must stay out of this table: a
// code listed here is reported as already compressed, so the file is never
// compressed and never surfaced to the user.
var dxgiCompressed = map[uint32]string{
	70: "BC1_UNORM", // BC1_TYPELESS
	71: "BC1_UNORM",
	72: "BC1_UNORM_SRGB",
	73: "BC2_UNORM", // BC2_TYPELESS
	74: "BC2_UNORM",
	75: "BC2_UNORM_SRGB",
	76: "BC3_UNORM", // BC3_TYPELESS
	77: "BC3_UNORM",
	78: "BC3_UNORM_SRGB",
	79: "BC4_UNORM", // BC4_TYPELESS
	80: "BC4_UNORM",
	81: "BC4_SNORM",
	82: "BC5_UNORM", // BC5_TYPELESS
	83: "BC5_UNORM",
	84: "BC5_SNORM",
	94: "BC6H_UF16", // BC6H_TYPELESS
	95: "BC6H_UF16",
	96: "BC6H_SF16",
	97: "BC7_UNORM", // BC7_TYPELESS
	98: "BC7_UNORM",
	99: "BC7_UNORM_SRGB",
}

// bytesPerTexelFloat returns the effective bytes per texel for VRAM estimation.
// BCn formats are block-compressed, so the value is fractional for the 4-bit ones.
// An _SRGB suffix is dropped first: gamma is a sampling rule, not a size, so the
// sRGB variant of a format costs exactly what the plain one costs.
func bytesPerTexelFloat(format string) float64 {
	switch strings.TrimSuffix(strings.ToUpper(format), "_SRGB") {
	case "BC1_UNORM", "BC4_UNORM", "BC4_SNORM":
		return 0.5 // 4 bits per pixel
	case "BC2_UNORM", "BC3_UNORM", "BC5_UNORM", "BC5_SNORM", "BC6H_UF16", "BC6H_SF16", "BC7_UNORM":
		return 1.0 // 8 bits per pixel
	case "R8G8B8A8_UNORM", "B8G8R8A8_UNORM":
		return 4.0
	case "R8G8B8_UNORM", "B8G8R8_UNORM":
		return 3.0
	case "R5G6B5_UNORM", "B5G6R5_UNORM", "B5G5R5A1_UNORM":
		return 2.0
	case "R16G16B16A16_FLOAT":
		return 8.0
	case "R32G32B32A32_FLOAT":
		return 16.0
	default:
		return 4.0 // conservative default
	}
}

// DDSInfo holds parsed metadata from a DDS file header.
type DDSInfo struct {
	Width       int
	Height      int
	Format      string // e.g. "BC7_UNORM", "DXT5", "R8G8B8A8_UNORM"
	Compressed  bool
	HasAlpha    bool
	MipMapCount int
}

// ParseDDS reads the first 148 bytes of path (enough for base + DX10 header) and
// returns texture metadata.
func ParseDDS(path string) (DDSInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return DDSInfo{}, err
	}
	defer f.Close()

	buf := make([]byte, 148) // 4 magic + 124 header + 20 DX10
	n, err := io.ReadFull(f, buf)
	if err != nil && n < 128 {
		return DDSInfo{}, fmt.Errorf("file too small for DDS header")
	}
	buf = buf[:n]

	if binary.LittleEndian.Uint32(buf[0:4]) != ddsMagic {
		return DDSInfo{}, fmt.Errorf("not a DDS file")
	}

	// DDS_HEADER starts at byte 4.
	height := int(binary.LittleEndian.Uint32(buf[12:16]))
	width := int(binary.LittleEndian.Uint32(buf[16:20]))
	mipCount := int(binary.LittleEndian.Uint32(buf[28:32]))
	if mipCount == 0 {
		mipCount = 1
	}

	// DDS_PIXELFORMAT is at byte 76 (offset 4 + 72).
	pfFlags := binary.LittleEndian.Uint32(buf[80:84])
	fourCC := binary.LittleEndian.Uint32(buf[84:88])
	rgbBitCount := binary.LittleEndian.Uint32(buf[88:92])
	alphaMask := binary.LittleEndian.Uint32(buf[104:108])

	var info DDSInfo
	info.Width = width
	info.Height = height
	info.MipMapCount = mipCount

	if pfFlags&ddpfFourCC != 0 {
		if fourCC == 0x30315844 { // "DX10"
			info, err = parseDX10Header(buf, info)
			if err != nil {
				return DDSInfo{}, err
			}
		} else {
			name, ok := fourCCFormats[fourCC]
			if ok {
				info.Format = name
				info.Compressed = true
			} else {
				info.Format = fourCCToString(fourCC)
				info.Compressed = false
			}
		}
	} else if pfFlags&ddpfRGB != 0 {
		info.Compressed = false
		switch rgbBitCount {
		case 32:
			if alphaMask != 0 {
				info.Format = "R8G8B8A8_UNORM"
				info.HasAlpha = true
			} else {
				info.Format = "R8G8B8X8_UNORM"
			}
		case 24:
			info.Format = "R8G8B8_UNORM"
		case 16:
			info.Format = "R5G6B5_UNORM"
		default:
			info.Format = fmt.Sprintf("RGB%d", rgbBitCount)
		}
	} else {
		info.Format = "UNKNOWN"
		info.Compressed = false
	}

	if pfFlags&ddpfAlphaPixel != 0 || alphaMask != 0 {
		info.HasAlpha = true
	}

	return info, nil
}

func parseDX10Header(buf []byte, base DDSInfo) (DDSInfo, error) {
	if len(buf) < 148 {
		return base, fmt.Errorf("file too small for DX10 header")
	}
	// DX10 header starts at byte 128 (4 magic + 124 base header).
	dxgiFormat := binary.LittleEndian.Uint32(buf[128:132])
	if name, ok := dxgiCompressed[dxgiFormat]; ok {
		base.Format = name
		base.Compressed = true
	} else {
		base.Format = fmt.Sprintf("DXGI_%d", dxgiFormat)
		base.Compressed = false
	}
	// Check for alpha in common DXGI formats.
	switch dxgiFormat {
	case 28, 29, 30, 31, 87, 91: // RGBA variants
		base.HasAlpha = true
	}
	return base, nil
}

func fourCCToString(fourCC uint32) string {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, fourCC)
	return string(b)
}

// VRAMBytes estimates VRAM usage for width×height×mips in the given format.
func VRAMBytes(width, height int, format string, mipCount int) int64 {
	bpt := bytesPerTexelFloat(format)
	// Mip chain multiplier ≈ 1.33 (geometric series sum for all mip levels).
	var total float64
	w, h := float64(width), float64(height)
	for i := 0; i < mipCount; i++ {
		total += w * h * bpt
		w = max(1, w/2)
		h = max(1, h/2)
	}
	if mipCount <= 1 {
		total *= 1.33
	}
	return int64(total)
}
