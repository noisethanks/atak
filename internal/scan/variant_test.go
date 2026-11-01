package scan

import (
	"path/filepath"
	"testing"
)

func TestIsVariantPath(t *testing.T) {
	cases := []struct {
		rel  string
		want bool
	}{
		{"gamedata/textures/wpn/x.dds", false},
		{"gamedata/textures/GameData_old/x.dds", false},
		{"gamedata/Blue/Gamedata/textures/x.dds", true},
		{"gamedata/blue/gamedata/textures/x.dds", true},
		{"Gamedata/textures/gamedata/x.dds", true},
	}
	for _, c := range cases {
		if got := IsVariantPath(c.rel); got != c.want {
			t.Errorf("IsVariantPath(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
}

// variantCases are (mod, mod-root-relative path, skipped) triples shared by both walkers.
var variantCases = []struct {
	mod, rel string
	skipped  bool
}{
	{"Gamedata Fixes", "gamedata/textures/wpn/x.dds", false},
	{"ModA", "gamedata/textures/GameData_old/x.dds", false},
	{"ModA", "gamedata/Blue/Gamedata/textures/x.dds", true},
	{"ModA", "gamedata/blue/gamedata/textures/x.dds", true},
}

func TestWalkVariantFolderGuardMatchesSegments(t *testing.T) {
	for _, c := range variantCases {
		modsDir := t.TempDir()
		writeFile(t, filepath.Join(modsDir, c.mod, filepath.FromSlash(c.rel)), uncompressedDDS(16, 16))

		assets, statsCh, _ := Walk(modsDir, statsTestProfiles(), nil, nil, 0)
		n := 0
		for range assets {
			n++
		}
		stats := <-statsCh
		if gotSkipped := n == 0 && stats.Skipped == 1; gotSkipped != c.skipped {
			t.Errorf("Walk %q/%q: skipped = %v, want %v (assets=%d, Skipped=%d)", c.mod, c.rel, gotSkipped, c.skipped, n, stats.Skipped)
		}
	}
}

func TestWalkVirtualVariantFolderGuardMatchesSegments(t *testing.T) {
	for _, c := range variantCases {
		modsDir := t.TempDir()
		abs := filepath.Join(modsDir, c.mod, filepath.FromSlash(c.rel))
		writeFile(t, abs, uncompressedDDS(16, 16))

		assets, statsCh, _ := WalkVirtual(map[string]string{filepath.FromSlash(c.rel): abs}, modsDir, statsTestProfiles(), nil, nil, 0)
		n := 0
		for range assets {
			n++
		}
		stats := <-statsCh
		if gotSkipped := n == 0 && stats.Skipped == 1; gotSkipped != c.skipped {
			t.Errorf("WalkVirtual %q/%q: skipped = %v, want %v (assets=%d, Skipped=%d)", c.mod, c.rel, gotSkipped, c.skipped, n, stats.Skipped)
		}
	}
}

// An excluded file is Excluded even when its header cannot be parsed, in both walkers.
func TestExcludedBeforeUnreadable(t *testing.T) {
	excludes := []string{"*_skip.dds"}
	rel := filepath.Join("gamedata", "textures", "bad_skip.dds")

	check := func(name string, assets <-chan Asset, statsCh <-chan Stats) {
		t.Helper()
		var found []Asset
		for a := range assets {
			found = append(found, a)
		}
		stats := <-statsCh
		if len(found) != 1 || !found[0].Excluded || found[0].ProfileMatch != "Excluded" {
			t.Errorf("%s: assets = %+v, want one Excluded", name, found)
		}
		if len(stats.Unreadable) != 0 {
			t.Errorf("%s: Unreadable = %+v, want none", name, stats.Unreadable)
		}
	}

	modsDir := t.TempDir()
	abs := filepath.Join(modsDir, "ModA", rel)
	writeFile(t, abs, pngNamedDDS())

	a, s, _ := Walk(modsDir, statsTestProfiles(), excludes, nil, 1024)
	check("Walk", a, s)
	a, s, _ = WalkVirtual(map[string]string{rel: abs}, modsDir, statsTestProfiles(), excludes, nil, 1024)
	check("WalkVirtual", a, s)
}
