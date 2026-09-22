package scan

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noisethanks/atak/internal/config"
)

// uncompressedDDS builds a 32-bit BGRA DDS of the given size, padded with pixel
// data so the file clears a realistic minFileSizeBytes.
func uncompressedDDS(width, height int) []byte {
	var buf bytes.Buffer
	w := func(v uint32) { _ = binary.Write(&buf, binary.LittleEndian, v) }

	buf.Write([]byte("DDS "))
	w(124)
	w(0x100f) // caps | height | width | pitch | pixelformat
	w(uint32(height))
	w(uint32(width))
	w(uint32(width * 4))
	w(1) // depth
	w(1) // mipMapCount
	for i := 0; i < 11; i++ {
		w(0)
	}
	w(32)
	w(0x41) // DDPF_RGB | DDPF_ALPHAPIXELS
	w(0)    // fourCC
	w(32)   // rgbBitCount
	w(0x00ff0000)
	w(0x0000ff00)
	w(0x000000ff)
	w(0xff000000)
	w(0x1000) // caps
	w(0)
	w(0)
	w(0)
	w(0)
	buf.Write(make([]byte, width*height*4))
	return buf.Bytes()
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// pngNamedDDS is the shape of the real problem: a PNG that a mod author renamed
// to .dds. The engine cannot load it either, so the user needs to know it is
// there. 2 KB clears the minimum file size, so the only thing that can drop it is
// the header parse.
func pngNamedDDS() []byte {
	data := make([]byte, 2048)
	copy(data, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})
	return data
}

func statsTestProfiles() []config.Profile {
	return []config.Profile{
		{Name: "Diffuse / Color", Format: "BC7_UNORM", Patterns: []string{"*_d.*"}},
	}
}

// TestWalkReportsUnreadableFiles covers files the scan sees, cannot read, and used
// to drop on the floor: the walker ran `continue` on a parse error, so the file
// appeared in no count and no list. The user had no way to learn it existed.
//
// The two outcomes are deliberately separate. Skipped means ATAK looked and
// decided correctly — already compressed, or below the size floor. Unreadable
// means ATAK could not tell, which is the user's problem to look at.
func TestWalkReportsUnreadableFiles(t *testing.T) {
	modsDir := t.TempDir()
	base := filepath.Join(modsDir, "ModA", "gamedata", "textures")
	writeFile(t, filepath.Join(base, "wall_d.dds"), uncompressedDDS(16, 16))
	writeFile(t, filepath.Join(base, "renamed_d.dds"), pngNamedDDS())
	writeFile(t, filepath.Join(base, "stub_d.dds"), make([]byte, 64))

	assets, statsCh, _ := Walk(modsDir, statsTestProfiles(), nil, nil, 1024)
	var found []Asset
	for a := range assets {
		found = append(found, a)
	}
	stats := <-statsCh

	if len(found) != 1 || filepath.Base(found[0].Path) != "wall_d.dds" {
		t.Fatalf("emitted %d assets, want just wall_d.dds: %+v", len(found), found)
	}
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (the 64-byte stub)", stats.Skipped)
	}
	if len(stats.Unreadable) != 1 {
		t.Fatalf("Unreadable = %+v, want the renamed PNG", stats.Unreadable)
	}
	u := stats.Unreadable[0]
	if filepath.Base(u.Path) != "renamed_d.dds" {
		t.Errorf("Unreadable path = %q, want renamed_d.dds", u.Path)
	}
	if u.ModName != "ModA" {
		t.Errorf("Unreadable mod = %q, want ModA", u.ModName)
	}
	if u.Reason == "" {
		t.Error("Unreadable reason is empty — the user needs to know why it could not be read")
	}
}

// TestWalkVirtualReportsUnreadableFiles is the same guarantee for mod output mode,
// which runs a different loop over a pre-merged virtual filesystem. A file that is
// listed but missing from disk is unreadable too: only one of the two walkers had
// that case at all, and it also dropped it silently.
func TestWalkVirtualReportsUnreadableFiles(t *testing.T) {
	modsDir := t.TempDir()
	modRoot := filepath.Join(modsDir, "ModA")
	good := filepath.Join("gamedata", "textures", "wall_d.dds")
	bad := filepath.Join("gamedata", "textures", "renamed_d.dds")
	gone := filepath.Join("gamedata", "textures", "missing_d.dds")
	writeFile(t, filepath.Join(modRoot, good), uncompressedDDS(16, 16))
	writeFile(t, filepath.Join(modRoot, bad), pngNamedDDS())

	virtual := map[string]string{
		good: filepath.Join(modRoot, good),
		bad:  filepath.Join(modRoot, bad),
		gone: filepath.Join(modRoot, gone),
	}

	assets, statsCh, _ := WalkVirtual(virtual, modsDir, statsTestProfiles(), nil, nil, 1024)
	var found []Asset
	for a := range assets {
		found = append(found, a)
	}
	stats := <-statsCh

	if len(found) != 1 {
		t.Fatalf("emitted %d assets, want 1: %+v", len(found), found)
	}
	if len(stats.Unreadable) != 2 {
		t.Fatalf("Unreadable = %+v, want the renamed PNG and the missing file", stats.Unreadable)
	}
	var names []string
	for _, u := range stats.Unreadable {
		names = append(names, filepath.Base(u.Path))
	}
	joined := strings.Join(names, " ")
	if !strings.Contains(joined, "renamed_d.dds") || !strings.Contains(joined, "missing_d.dds") {
		t.Errorf("Unreadable names = %v, want both renamed_d.dds and missing_d.dds", names)
	}
}
