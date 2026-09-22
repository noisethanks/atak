package modlist

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBuildVirtualFSHighestPriorityWins is the end-to-end guard for load-order
// correctness: when two mods provide the same relative path, the winning source
// in the virtual filesystem must be the higher-priority mod — the one listed
// first in modlist.txt (highest priority). Before the parser fix, the lowest
// priority mod won, silently compressing the overridden texture.
func TestBuildVirtualFSHighestPriorityWins(t *testing.T) {
	modsDir := t.TempDir()

	rel := filepath.Join("gamedata", "textures", "wpn", "ak74_d.dds")
	writeModFile(t, modsDir, "HighMod", rel, "high")
	writeModFile(t, modsDir, "LowMod", rel, "low")

	// As returned by ParseModList: highest priority first.
	modList := []string{"HighMod", "LowMod"}

	virtual, err := BuildVirtualFS(modsDir, modList)
	if err != nil {
		t.Fatalf("BuildVirtualFS: %v", err)
	}

	winner := virtual[rel]
	want := filepath.Join(modsDir, "HighMod", rel)
	if winner != want {
		t.Fatalf("wrong conflict winner:\n got  %s\n want %s", winner, want)
	}

	data, err := os.ReadFile(winner)
	if err != nil {
		t.Fatalf("read winner: %v", err)
	}
	if string(data) != "high" {
		t.Fatalf("winning file content = %q, want %q (lowest-priority mod won the conflict)", data, "high")
	}
}

// TestBuildVirtualFSCaseInsensitiveConflict covers the same load-order rule for
// two mods that spell the same path differently. MO2 merges case-insensitively on
// Windows, so ui_icon_ump.dds and ui_icon_ump.DDS are one file to the game and
// load order picks the winner. Keyed by exact case, the map instead kept both,
// the scan compressed both, and the two jobs wrote the same output path — so the
// low-priority copy could land in the output and be the one the game shows.
func TestBuildVirtualFSCaseInsensitiveConflict(t *testing.T) {
	modsDir := t.TempDir()

	high := filepath.Join("gamedata", "textures", "ui", "ui_icon_ump.dds")
	low := filepath.Join("gamedata", "textures", "ui", "ui_icon_ump.DDS")
	writeModFile(t, modsDir, "HighMod", high, "high")
	writeModFile(t, modsDir, "LowMod", low, "low")

	virtual, err := BuildVirtualFS(modsDir, []string{"HighMod", "LowMod"})
	if err != nil {
		t.Fatalf("BuildVirtualFS: %v", err)
	}

	if len(virtual) != 1 {
		t.Fatalf("got %d entries, want 1 — a case-different spelling is the same file to the game: %v", len(virtual), virtual)
	}
	got, ok := virtual[high]
	if !ok {
		t.Fatalf("winner is not keyed by the winning mod's spelling %q: %v", high, virtual)
	}
	want := filepath.Join(modsDir, "HighMod", high)
	if got != want {
		t.Fatalf("wrong conflict winner:\n got  %s\n want %s", got, want)
	}
}

// TestBuildVirtualFSKeepsWinningSpelling pins the direction of the case fix: the
// surviving key is the winning mod's own spelling, not a lowercased one. The key
// becomes the output path in mod-output mode, so lowercasing it here would rename
// every mixed-case texture in the output folder.
func TestBuildVirtualFSKeepsWinningSpelling(t *testing.T) {
	modsDir := t.TempDir()

	high := filepath.Join("gamedata", "textures", "ui", "ICON.DDS")
	low := filepath.Join("gamedata", "textures", "ui", "icon.dds")
	writeModFile(t, modsDir, "HighMod", high, "high")
	writeModFile(t, modsDir, "LowMod", low, "low")

	virtual, err := BuildVirtualFS(modsDir, []string{"HighMod", "LowMod"})
	if err != nil {
		t.Fatalf("BuildVirtualFS: %v", err)
	}
	if _, ok := virtual[high]; !ok || len(virtual) != 1 {
		t.Fatalf("want the single key %q (the winner's spelling), got %v", high, virtual)
	}
}

// TestBuildVirtualFSSkipsVariantFoldersInAnySpelling covers the guard three lines
// above the conflict key, which folded no case at all. A mod that ships its
// variant tree as gamedata/Blue/Gamedata/... means the same thing as one that
// spells it lowercase, and the lowercase one was pruned while the other was not.
func TestBuildVirtualFSSkipsVariantFoldersInAnySpelling(t *testing.T) {
	modsDir := t.TempDir()
	lower := filepath.Join("gamedata", "Green", "gamedata", "textures", "a.dds")
	upper := filepath.Join("gamedata", "Blue", "Gamedata", "textures", "b.dds")
	kept := filepath.Join("gamedata", "textures", "c.dds")
	writeModFile(t, modsDir, "ModA", lower, "x")
	writeModFile(t, modsDir, "ModA", upper, "x")
	writeModFile(t, modsDir, "ModA", kept, "x")

	virtual, err := BuildVirtualFS(modsDir, []string{"ModA"})
	if err != nil {
		t.Fatalf("BuildVirtualFS: %v", err)
	}
	if _, ok := virtual[lower]; ok {
		t.Error("lowercase variant folder was not pruned")
	}
	if _, ok := virtual[upper]; ok {
		t.Errorf("mixed-case variant folder survived: %v", virtual)
	}
	if _, ok := virtual[kept]; !ok {
		t.Error("an ordinary gamedata path was pruned")
	}
}

func writeModFile(t *testing.T, modsDir, mod, rel, content string) {
	t.Helper()
	full := filepath.Join(modsDir, mod, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
