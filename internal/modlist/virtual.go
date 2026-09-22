package modlist

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// BuildVirtualFS assembles a virtual filesystem representing the merged MO2
// modlist. Returns map[relPath]absoluteSourcePath where relPath is relative to
// the mod's own root (e.g. "gamedata/textures/wpn/ak74.dds").
//
// modList must be ordered high→low priority (as returned by ParseModList).
// We iterate low→high so higher-priority mods overwrite lower-priority entries.
//
// Conflicts resolve case-insensitively, because MO2 merges that way: the game
// sees one gamedata tree, and ui_icon_ump.dds and ui_icon_ump.DDS are one file in
// it. Keyed by exact case, two spellings survived as two entries, load order
// stopped deciding the winner, and both copies became compression jobs that wrote
// the same output path — so the losing mod's texture could be the one left in the
// output folder. The key keeps the winner's own spelling rather than a lowercased
// one, since in mod-output mode the key is the path the output file is written to.
func BuildVirtualFS(modsDir string, modList []string) (map[string]string, error) {
	virtual := make(map[string]string)
	keyOf := make(map[string]string) // case-folded rel -> the rel currently in virtual

	for i := len(modList) - 1; i >= 0; i-- {
		modPath := filepath.Join(modsDir, modList[i])
		err := filepath.WalkDir(modPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip unreadable entries
			}
			if d.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(modPath, path)
			if relErr != nil {
				return nil
			}
			key := strings.ToLower(filepath.ToSlash(rel))
			if strings.Count(key, "gamedata") > 1 {
				// Variant folder (e.g. gamedata/Green/gamedata/...). Counted on the
				// folded key, because a mod spelling it Gamedata means the same thing.
				return nil
			}
			if prev, ok := keyOf[key]; ok && prev != rel {
				delete(virtual, prev) // this mod outranks the one that spelled it differently
			}
			keyOf[key] = rel
			virtual[rel] = path
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return virtual, nil
}
