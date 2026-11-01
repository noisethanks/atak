package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/noisethanks/atak/internal/config"
)

// Asset is a single texture file discovered during a scan.
type Asset struct {
	Path           string
	ModName        string
	CurrentFmt     string
	Compressed     bool
	Excluded       bool
	Width          int
	Height         int
	HasAlpha       bool
	ProfileMatch   string
	SuggestedFmt   string
	SourceMipCount int    // mip levels in the source DDS (1 = no chain); drives per-file mip policy
	VirtualRelPath string // set by WalkVirtual: path relative to mod root (e.g. gamedata/textures/wpn/ak74.dds)
}

// UnreadableFile is a .dds the scan found and could not read. It is not an Asset,
// because nothing is known about it beyond its name — not its format, not its
// size on screen, not which profile would claim it.
type UnreadableFile struct {
	Path    string
	ModName string
	Reason  string
}

// Stats reports every file a scan saw and did not emit as an Asset. The two
// fields answer different questions and must not be added together.
//
// Skipped is a correct decision: the file is already compressed, is below the
// minimum size, or sits in a variant folder ATAK does not merge.
//
// Unreadable is a decision ATAK could not make. Both walkers used to drop these
// files with a bare continue, so a mod shipping a PNG named .dds — the game
// cannot load one either — left no trace in the UI at all.
type Stats struct {
	Skipped    int
	Unreadable []UnreadableFile
}

// Walk traverses modsDir, emitting Asset values for every uncompressed .dds file found.
// excludePatterns are globs from profiles.json — patterns without '/' match the basename,
// patterns with '/' match the full path relative to the mod root. Matched files are emitted
// with Excluded:true. exclusions are directory globs from config.json — a plain name
// matches a directory anywhere, a path pattern matches its mod-root-relative path; a
// matched directory and its whole subtree are skipped entirely.
// Returns three channels: assets, Stats (one value sent on completion), and errors.
func Walk(modsDir string, profiles []config.Profile, excludePatterns []string, exclusions []string, minFileSizeBytes int) (<-chan Asset, <-chan Stats, <-chan error) {
	assets := make(chan Asset, 256)
	statsCh := make(chan Stats, 1)
	errs := make(chan error, 1)

	go func() {
		defer close(errs)
		var stats Stats

		err := filepath.WalkDir(modsDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip unreadable entries, keep walking
			}
			if d.IsDir() {
				rel, _ := filepath.Rel(modsDir, path)
				if excludesDir(d.Name(), modRelPath(rel), exclusions) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.EqualFold(filepath.Ext(path), ".dds") {
				return nil
			}
			if fi, fiErr := d.Info(); fiErr == nil && fi.Size() < int64(minFileSizeBytes) {
				stats.Skipped++
				return nil
			}

			rel, _ := filepath.Rel(modsDir, path)

			info, err := ParseDDS(path)
			if err != nil {
				stats.Unreadable = append(stats.Unreadable, UnreadableFile{
					Path:    path,
					ModName: modNameFromRel(rel),
					Reason:  err.Error(),
				})
				return nil
			}
			if info.Compressed {
				stats.Skipped++
				return nil
			}

			if strings.Count(strings.ToLower(filepath.ToSlash(rel)), "gamedata") > 1 {
				stats.Skipped++
				return nil // variant folder with double gamedata path, in any spelling
			}
			modName := modNameFromRel(rel)
			base := filepath.Base(path)
			modRelSlash := modRelPath(rel)

			// Global exclude check — runs before profile matching.
			if matchAnyPattern(base, modRelSlash, excludePatterns) {
				assets <- Asset{
					Path:           path,
					ModName:        modName,
					CurrentFmt:     info.Format,
					Width:          info.Width,
					Height:         info.Height,
					HasAlpha:       info.HasAlpha,
					ProfileMatch:   "Excluded",
					Excluded:       true,
					SourceMipCount: info.MipMapCount,
				}
				return nil
			}

			// Profile matching with per-profile exclusion.
			profileName, suggestedFmt := matchProfile(modRelSlash, profiles)
			if profileName == "" {
				profileName = "Unmatched"
			}

			assets <- Asset{
				Path:           path,
				ModName:        modName,
				CurrentFmt:     info.Format,
				Compressed:     info.Compressed,
				Width:          info.Width,
				Height:         info.Height,
				HasAlpha:       info.HasAlpha,
				ProfileMatch:   profileName,
				SuggestedFmt:   suggestedFmt,
				SourceMipCount: info.MipMapCount,
			}
			return nil
		})
		statsCh <- stats
		close(assets)
		close(statsCh)
		if err != nil {
			errs <- err
		}
	}()

	return assets, statsCh, errs
}

// WalkVirtual scans a pre-built virtual filesystem map (relPath→absPath) instead of
// walking a directory. Same classification logic as Walk. excludePatterns follow the
// same path-aware rules as Walk — relPath (already mod-root-relative) is used directly.
// exclusions are the same directory globs Walk prunes with: name-only patterns must
// already have removed whole mods from the modList passed to BuildVirtualFS (see
// ExcludesMod), while path patterns are applied here per file, since a flat virtual FS
// has no directory walk to prune.
func WalkVirtual(virtualFS map[string]string, modsDir string, profiles []config.Profile, excludePatterns []string, exclusions []string, minFileSizeBytes int) (<-chan Asset, <-chan Stats, <-chan error) {
	assets := make(chan Asset, 256)
	statsCh := make(chan Stats, 1)
	errs := make(chan error, 1)

	go func() {
		defer close(errs)
		var stats Stats

		// unreadable records a file the scan could not classify. Reporting it is the
		// whole point: the alternative, a bare continue, leaves the file out of every
		// count and the user never learns it is there.
		unreadable := func(path, mod, reason string) {
			stats.Unreadable = append(stats.Unreadable, UnreadableFile{
				Path:    path,
				ModName: mod,
				Reason:  reason,
			})
		}

		for relPath, absPath := range virtualFS {
			relSlash := filepath.ToSlash(relPath)
			if underExcludedDir(relSlash, exclusions) {
				continue // inside a directory the scan excludes — pruned in Walk, skipped here
			}
			if strings.Count(strings.ToLower(relSlash), "gamedata") > 1 {
				stats.Skipped++
				continue // variant folder with double gamedata path, in any spelling
			}
			if !strings.EqualFold(filepath.Ext(absPath), ".dds") {
				continue
			}

			relToMods, relErr := filepath.Rel(modsDir, absPath)
			if relErr != nil {
				unreadable(absPath, "", relErr.Error())
				continue
			}
			modName := modNameFromRel(relToMods)

			fi, err := os.Stat(absPath)
			if err != nil {
				unreadable(absPath, modName, err.Error())
				continue
			}
			if fi.Size() < int64(minFileSizeBytes) {
				stats.Skipped++
				continue
			}

			info, err := ParseDDS(absPath)
			if err != nil {
				unreadable(absPath, modName, err.Error())
				continue
			}
			if info.Compressed {
				stats.Skipped++
				continue
			}

			base := filepath.Base(absPath)

			// Global exclude check — runs before profile matching.
			if matchAnyPattern(base, relSlash, excludePatterns) {
				assets <- Asset{
					Path:           absPath,
					ModName:        modName,
					CurrentFmt:     info.Format,
					Width:          info.Width,
					Height:         info.Height,
					HasAlpha:       info.HasAlpha,
					ProfileMatch:   "Excluded",
					Excluded:       true,
					SourceMipCount: info.MipMapCount,
					VirtualRelPath: relPath,
				}
				continue
			}

			profileName, suggestedFmt := matchProfile(relSlash, profiles)
			if profileName == "" {
				profileName = "Unmatched"
			}

			assets <- Asset{
				Path:           absPath,
				ModName:        modName,
				CurrentFmt:     info.Format,
				Width:          info.Width,
				Height:         info.Height,
				HasAlpha:       info.HasAlpha,
				ProfileMatch:   profileName,
				SuggestedFmt:   suggestedFmt,
				SourceMipCount: info.MipMapCount,
				VirtualRelPath: relPath,
			}
		}

		statsCh <- stats
		close(assets)
		close(statsCh)
	}()

	return assets, statsCh, errs
}

// modNameFromRel extracts the top-level mod directory name from a relative path.
func modNameFromRel(rel string) string {
	parts := strings.SplitN(rel, string(filepath.Separator), 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return rel
}

// modRelPath converts a modsDir-relative path into the mod-root-relative slash path that
// every pattern is written against, by dropping the leading mod-name segment:
// "Some Mod/gamedata/textures/ui/icon.dds" -> "gamedata/textures/ui/icon.dds".
// WalkVirtual's relPath already has this shape; Walk's does not, and skipping this step
// leaves an extra segment that no path pattern can match.
func modRelPath(rel string) string {
	relSlash := filepath.ToSlash(rel)
	if idx := strings.Index(relSlash, "/"); idx >= 0 {
		return relSlash[idx+1:]
	}
	return relSlash
}

// matchProfile returns the profile name and suggested format for the given file.
// relSlash must be mod-root-relative and slash-separated (gamedata/textures/ui/icon.dds),
// since that is what path patterns like */textures/ui/* are written against; passing a
// modsDir-relative path instead leaves an extra leading segment that no path pattern
// will match.
//
// A profile's exclude list means that profile declines the file, not that the file is
// dropped: matching continues with later profiles, so e.g. scope diffuse textures decline
// Diffuse / Color and fall through to Scope Textures, which groups every part of a scope
// under one profile. To drop a file outright, use the global excludePatterns instead —
// those are also reported in the scan as Excluded, whereas a profile decline is silent.
func matchProfile(relSlash string, profiles []config.Profile) (name, format string) {
	base := filepath.Base(relSlash)
	for _, p := range profiles {
		if !matchAnyPattern(base, relSlash, p.Patterns) {
			continue
		}
		if matchAnyPattern(base, relSlash, p.Exclude) {
			continue // profile declines — keep looking
		}
		return p.Name, p.Format
	}
	return "", ""
}

// pathPatternCache memoizes compiled path patterns; the same handful of patterns is
// tested against every file in a scan.
var pathPatternCache sync.Map // normalized pattern -> *regexp.Regexp

// pathPatternRegexp compiles a path pattern into an anchored regexp.
//
// * and ? keep their usual glob meaning — neither crosses a separator, so * spans
// one path segment. The single exception is a directory suffix: a trailing /*, or a
// bare trailing / treated as shorthand for it, matches everything below that directory
// at any depth, so */textures/ui/* and */textures/ui/ both cover
// textures/ui/nested/file.dds. Without that, a directory pattern would only ever
// reach direct children, which is what made path-based excludePatterns silently fail
// to cover a subtree — and a bare-slash directory silently match nothing at all.
func pathPatternRegexp(pattern string) *regexp.Regexp {
	// Patterns are authored, not observed, so a backslash in one always means a
	// separator — unlike a real path, where on Unix it is a legal filename character.
	// filepath.ToSlash would only convert on Windows, leaving such a pattern inert here.
	key := strings.ToLower(strings.ReplaceAll(pattern, `\`, `/`))
	if v, ok := pathPatternCache.Load(key); ok {
		return v.(*regexp.Regexp)
	}

	dir, recursive := strings.CutSuffix(key, "/*")
	if !recursive {
		// A bare trailing / is shorthand for /* — the same recursive subtree match.
		dir, recursive = strings.CutSuffix(dir, "/")
	}

	// QuoteMeta escapes every metacharacter, including * and ?, so putting those two
	// back is all that separates a literal from a glob.
	expr := regexp.QuoteMeta(dir)
	expr = strings.ReplaceAll(expr, `\*`, `[^/]*`)
	expr = strings.ReplaceAll(expr, `\?`, `[^/]`)
	if recursive {
		expr += `/.*`
	}

	re := regexp.MustCompile(`^` + expr + `$`)
	pathPatternCache.Store(key, re)
	return re
}

// matchPathPattern matches a path pattern against a mod-root-relative slash path.
func matchPathPattern(pattern, relSlash string) bool {
	return pathPatternRegexp(pattern).MatchString(
		strings.ToLower(filepath.ToSlash(relSlash)),
	)
}

// matchesPattern matches one glob pattern against a file, and owns the case and
// separator normalization for both sides. Patterns containing a separator are matched
// against the mod-root-relative slash path; the rest match the basename.
//
// Every pattern list in profiles.json goes through here — a profile's patterns, a
// profile's exclude, and the global excludePatterns — so a pattern means the same
// thing wherever it is written. See SPEC.md for why they were unified.
func matchesPattern(pattern, basename, relSlash string) bool {
	if strings.ContainsAny(pattern, `/\`) {
		return matchPathPattern(pattern, relSlash)
	}
	matched, _ := filepath.Match(strings.ToLower(pattern), strings.ToLower(basename))
	return matched
}

// matchAnyPattern reports whether the file matches any pattern in the list.
func matchAnyPattern(basename, relSlash string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchesPattern(pattern, basename, relSlash) {
			return true
		}
	}
	return false
}

// Directory exclusions (config.json's scanExclusions) share the profiles.json pattern
// syntax: a pattern without a separator matches a directory or mod name, and a path
// pattern matches the directory's mod-root-relative path, so an exclusion can target a
// nested subtree (*/textures/ui/SquareDOV) and not just a top-level folder name. They
// differ from file excludePatterns only in what they act on — a whole directory subtree
// rather than a single file.

// dirExclusionBase strips a path exclusion's optional recursive suffix — a trailing /*
// or a bare trailing / — leaving the directory it designates. */textures/ui/SquareDOV,
// its /-suffixed form and its /*-suffixed form all reduce to the same directory.
func dirExclusionBase(pattern string) string {
	p := strings.ReplaceAll(pattern, `\`, `/`)
	if b, ok := strings.CutSuffix(p, "/*"); ok {
		return b
	}
	return strings.TrimSuffix(p, "/")
}

// excludesDir reports whether a directory (its name plus its mod-root-relative path) is
// selected for pruning by any exclusion pattern. A name-only pattern matches the
// directory name; a path pattern matches the directory itself, and the walk then prunes
// the whole subtree beneath it.
func excludesDir(name, relSlash string, patterns []string) bool {
	for _, p := range patterns {
		if strings.ContainsAny(p, `/\`) {
			if matchPathPattern(dirExclusionBase(p), relSlash) {
				return true
			}
		} else if m, _ := filepath.Match(strings.ToLower(p), strings.ToLower(name)); m {
			return true
		}
	}
	return false
}

// underExcludedDir reports whether a file's mod-root-relative path lies inside a
// directory named by a path exclusion. Name-only exclusions filter whole mods before the
// virtual FS is built (see ExcludesMod), so only path patterns are relevant here — this
// is how a nested-subtree exclusion is honored in mod-output mode, where there is no
// directory walk to prune.
func underExcludedDir(relSlash string, patterns []string) bool {
	for _, p := range patterns {
		if strings.ContainsAny(p, `/\`) && matchPathPattern(dirExclusionBase(p)+"/*", relSlash) {
			return true
		}
	}
	return false
}

// ExcludesMod reports whether a mod folder name is selected by any scan-exclusion
// pattern, so the caller can drop that mod before building the virtual FS. A mod name is
// a single path segment: name-only patterns match it, while path patterns (which target
// nested directories) never match a bare mod name and are applied per file instead.
func ExcludesMod(mod string, patterns []string) bool {
	return excludesDir(mod, mod, patterns)
}
