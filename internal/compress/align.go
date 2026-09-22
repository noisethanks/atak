package compress

import (
	"math"
	"strings"

	"github.com/noisethanks/atak/internal/scan"
)

// blockDim is the edge length of a BCn compression block. BC1 through BC7 all
// encode 4x4 pixel groups, so a width or height that is not a multiple of 4 has
// no exact representation in any of them.
const blockDim = 4

// isBlockCompressed reports whether format names a BCn format. It matches on the
// "BC" prefix instead of enumerating values so the _SRGB and _SNORM variants are
// covered without listing every combination. No non-block DXGI format name starts
// with BC, so the prefix is safe. Formats that store one value per pixel
// (R8G8B8A8_UNORM and friends) have no block constraint and must never be resized
// by planResize.
func isBlockCompressed(format string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(format)), "BC")
}

// resizePlan is the exact output size a job must be resampled to. The zero value
// means the source dimensions are already correct, so no -w/-h is passed and the
// encoder keeps the source size.
type resizePlan struct {
	Width   int
	Height  int
	Aligned bool // block alignment, not maxTextureSize, is what moved a dimension
	Convert bool // the resample runs for the channel order, whatever the size says
}

func (p resizePlan) needed() bool { return p.Width > 0 && p.Height > 0 }

// reason names the fallback this plan should be reported under. Block alignment
// wins when several rules fired, because it is the one the user did not ask for
// and the one that explains a dimension they never configured. A channel-order
// conversion is invisible in the file's header, so it is reported next.
func (p resizePlan) reason() string {
	switch {
	case p.Aligned:
		return FallbackBlockAlign
	case p.Convert:
		return FallbackChannelOrder
	}
	return FallbackResize
}

// planResize computes the output dimensions for one job. It covers two reasons to
// resample, and a single file can need both.
//
// The first is the user's maxTextureSize budget: scale by the longer axis so
// neither axis exceeds the budget and the aspect ratio holds.
//
// The second is block alignment, and it is the reason this function exists.
// d3dx11_43.dll rounds block-compressed dimensions up to a multiple of 4 when it
// loads a texture. X-Ray loads every texture through D3DX11CreateTextureFromMemory,
// so rounding forces a resample, and resampling a block-compressed image forces a
// full decode, resize, and re-encode through Microsoft's 2010 reference encoder,
// single threaded, on first bind. For BC7 that costs seconds to minutes per file:
// a 43x43 icon measured 345 ms, 269x271 measured 10.8 s, and 1026x770 measured
// 287 s. Aligned files cost under 3 ms because no encoder runs at all.
//
// Rounding up here is therefore not a new behavior. It is the same resize the
// engine already performs on every single load, moved offline, paid once, with a
// better filter. The displayed result is what the game shows today. Leaving the
// file misaligned does not avoid the resize, it only moves it into the hot path.
//
// Uncompressed sources escape all of this, which is why the problem only appears
// after compression: a RAW32 UI texture at 269x271 has no block constraint and
// nothing rounds.
func planResize(asset scan.Asset, format string, maxTextureSize int) resizePlan {
	// Dimensions come from the DDS header via scan. A malformed or unparsed header
	// leaves them at 0; resizing on a guessed size would be worse than skipping.
	if asset.Width <= 0 || asset.Height <= 0 {
		return resizePlan{}
	}

	w, h := asset.Width, asset.Height
	if maxTextureSize > 0 && (w > maxTextureSize || h > maxTextureSize) {
		if w >= h {
			h = int(math.Round(float64(h) * float64(maxTextureSize) / float64(w)))
			w = maxTextureSize
		} else {
			w = int(math.Round(float64(w) * float64(maxTextureSize) / float64(h)))
			h = maxTextureSize
		}
		w = max(w, 1)
		h = max(h, 1)
	}

	aligned := false
	if isBlockCompressed(format) {
		aw, ah := alignDim(w, maxTextureSize), alignDim(h, maxTextureSize)
		if aw != w || ah != h {
			aligned = true
			w, h = aw, ah
		}
	}

	if w == asset.Width && h == asset.Height {
		return resizePlan{}
	}
	return resizePlan{Width: w, Height: h, Aligned: aligned}
}

// alignDim rounds v to a multiple of blockDim. It rounds up by default, the same
// direction d3dx11_43.dll rounds, so a pre-resized file shows the picture the
// engine would have produced by itself. It rounds down instead when rounding up
// would push the dimension past maxTextureSize, so honoring the alignment never
// silently breaks the VRAM budget the user asked for. One block is the floor: a
// block-compressed texture already occupies a full 4x4 block at any smaller size.
func alignDim(v, maxTextureSize int) int {
	if v%blockDim == 0 {
		return v
	}
	up := v + blockDim - v%blockDim
	if maxTextureSize > 0 && up > maxTextureSize {
		if down := v - v%blockDim; down >= blockDim {
			return down
		}
	}
	return up
}
