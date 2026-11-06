package screens

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/noisethanks/atak/internal/compress"
	"github.com/noisethanks/atak/internal/config"
	"github.com/noisethanks/atak/internal/modlist"
	"github.com/noisethanks/atak/internal/scan"
)

// toolPath resolves a real backend binary from internal/tools/bin for this
// platform, skipping when it is not shipped for it.
func toolPath(t *testing.T, stem string) string {
	t.Helper()
	var suffix string
	switch runtime.GOOS {
	case "darwin":
		suffix = "-macos-" + runtime.GOARCH
	case "linux":
		suffix = "-linux"
	case "windows":
		suffix = "-windows.exe"
	default:
		t.Skipf("no %s build for %s", stem, runtime.GOOS)
	}
	p, err := filepath.Abs(filepath.Join("..", "..", "tools", "bin", stem+suffix))
	if err != nil {
		t.Skip(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("binary missing: %v", err)
	}
	return p
}

// rgbaDDS builds an uncompressed 8x8 A8R8G8B8 DDS with a legacy header, which
// both texconv and compressonator-bc7e read.
func rgbaDDS() []byte {
	const w, h = 8, 8
	buf := make([]byte, 4+124+w*h*4)
	copy(buf, "DDS ")
	le := binary.LittleEndian
	hdr := buf[4:]
	le.PutUint32(hdr[0:], 124)
	le.PutUint32(hdr[4:], 0x1|0x2|0x4|0x1000|0x8) // CAPS|HEIGHT|WIDTH|PIXELFORMAT|PITCH
	le.PutUint32(hdr[8:], h)
	le.PutUint32(hdr[12:], w)
	le.PutUint32(hdr[16:], w*4)
	pf := hdr[72:]
	le.PutUint32(pf[0:], 32)
	le.PutUint32(pf[4:], 0x41) // RGB|ALPHAPIXELS
	le.PutUint32(pf[12:], 32)
	le.PutUint32(pf[16:], 0x00ff0000)
	le.PutUint32(pf[20:], 0x0000ff00)
	le.PutUint32(pf[24:], 0x000000ff)
	le.PutUint32(pf[28:], 0xff000000)
	le.PutUint32(hdr[104:], 0x1000) // DDSCAPS_TEXTURE
	px := buf[128:]
	for i := range px {
		px[i] = byte(i * 7)
	}
	return buf
}

// TestSpacedPathsEndToEnd puts a space in every path a run touches — the
// modlist path, modsDir, the mod folder, the mod output folder, and therefore
// the .atak-staging directory under it — and drives the real scan, the real job
// builder and the real backends in both modes.
func TestSpacedPathsEndToEnd(t *testing.T) {
	backends := map[string]func(t *testing.T) (compress.Backend, compress.Backend){
		"texconv": func(t *testing.T) (compress.Backend, compress.Backend) {
			return compress.NewTexconvBackend(toolPath(t, "texconv")), nil
		},
		"compressonator-bc7e": func(t *testing.T) (compress.Backend, compress.Backend) {
			return compress.NewCompressonatorBackend(toolPath(t, "compressonator-bc7e")),
				compress.NewTexconvBackend(toolPath(t, "texconv"))
		},
	}
	for name, mk := range backends {
		for _, modOutput := range []bool{false, true} {
			mode := "in place"
			if modOutput {
				mode = "mod output"
			}
			t.Run(name+"/"+mode, func(t *testing.T) {
				primary, fallback := mk(t)
				root := t.TempDir()
				profileDir := filepath.Join(root, "MO2 Profile Dir")
				modsDir := filepath.Join(root, "My Mods Dir")
				modName := "Some Mod Name"
				texDir := filepath.Join(modsDir, modName, "gamedata", "textures", "ui dir")
				for _, d := range []string{profileDir, texDir} {
					if err := os.MkdirAll(d, 0755); err != nil {
						t.Fatal(err)
					}
				}
				src := filepath.Join(texDir, "my icon.dds")
				orig := rgbaDDS()
				if err := os.WriteFile(src, orig, 0644); err != nil {
					t.Fatal(err)
				}
				modlistPath := filepath.Join(profileDir, "mod list.txt")
				if err := os.WriteFile(modlistPath, []byte("+"+modName+"\n"), 0644); err != nil {
					t.Fatal(err)
				}

				mods, err := modlist.ParseModList(modlistPath)
				if err != nil {
					t.Fatal(err)
				}
				vfs, err := modlist.BuildVirtualFS(modsDir, mods)
				if err != nil {
					t.Fatal(err)
				}
				profiles := []config.Profile{{Name: "UI", Format: "BC7_UNORM", Patterns: []string{"*.dds"}}}
				assetCh, _, _ := scan.WalkVirtual(vfs, modsDir, profiles, nil, nil, 0)
				var assets []scan.Asset
				for a := range assetCh {
					assets = append(assets, a)
				}
				if len(assets) != 1 || assets[0].ProfileMatch != "UI" {
					t.Fatalf("scan: got %+v", assets)
				}
				a := assets[0]

				data := CompressJobData{
					Groups: []ConfiguredGroup{{
						ProfileName:  "UI",
						Format:       a.SuggestedFmt,
						GenerateMips: []bool{true},
						Paths:        []string{a.Path},
						RelPaths:     []string{a.VirtualRelPath},
						Widths:       []int{a.Width},
						Heights:      []int{a.Height},
						Formats:      []string{a.CurrentFmt},
					}},
					WorkerCount: 1,
					ModsDir:     modsDir,
				}
				dest := src
				if modOutput {
					data.ModOutputDir = filepath.Join(modsDir, "ATAK Output Mod")
					dest = filepath.Join(data.ModOutputDir, filepath.FromSlash(a.VirtualRelPath))
				}

				jobs := buildJobs(data)
				var results []compress.CompressionResult
				for r := range compress.RunPool(context.Background(), primary, fallback, jobs, 1) {
					results = append(results, r)
				}
				if len(results) != 1 {
					t.Fatalf("got %d results", len(results))
				}
				r := results[0]
				if !r.Success || r.OutputSkipped {
					t.Fatalf("job failed: success=%v skipped=%v err=%v\n%s", r.Success, r.OutputSkipped, r.Err, r.Stderr)
				}
				info, err := scan.ParseDDS(dest)
				if err != nil {
					t.Fatalf("output %s: %v", dest, err)
				}
				if !info.Compressed {
					t.Errorf("output %s not compressed: %+v", dest, info)
				}
				if modOutput {
					if got, _ := os.ReadFile(src); string(got) != string(orig) {
						t.Error("source modified in mod output mode")
					}
				}
				filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
					if err == nil && strings.Contains(p, scan.StagingDirName) {
						t.Errorf("staging left behind: %s", p)
					}
					return nil
				})
			})
		}
	}
}
