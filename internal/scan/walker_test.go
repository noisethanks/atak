package scan

import (
	"strings"
	"testing"

	"github.com/noisethanks/atak/internal/config"
)

// testProfiles mirrors the ordering used by the shipped config: Bump Maps sits well
// ahead of Scope Textures, so scope bump maps only reach the scope profile if a
// profile's exclude list lets matching continue rather than dropping the file.
func testProfiles() []config.Profile {
	return []config.Profile{
		{Name: "UI Readables", Format: "BC3_UNORM", Patterns: []string{"*/textures/ui/readables/*"}},
		{
			Name:     "Bump Maps",
			Format:   "BC7_UNORM",
			Exclude:  []string{"*scope*bump*", "*lens_bump*"},
			Patterns: []string{"*_bump.*", "*_bump#.*"},
		},
		{
			Name:     "Normal Maps",
			Format:   "BC5_UNORM",
			Patterns: []string{"*_normal.*", "*_nm.*", "*_nrm.*"},
		},
		{Name: "UI / Icons", Format: "BC3_UNORM", Patterns: []string{"*/textures/ui/*", "*_icons.*"}},
		{Name: "Diffuse / Color", Format: "BC7_UNORM", Patterns: []string{"*_d.*", "*_diff.*", "*_b.*"}},
		{Name: "Scope Textures", Format: "BC7_UNORM", Patterns: []string{
			"*/textures/wpn/scope_*", "*scope*diff*", "*scope*bump*", "*lens_bump*",
		}},
		{Name: "Weapon Textures", Format: "BC7_UNORM", Patterns: []string{"*/textures/wpn/*"}},
	}
}

func TestMatchProfile(t *testing.T) {
	tests := []struct {
		name       string
		rel        string
		wantName   string
		wantFormat string
	}{
		{
			// The regression this exists for: Bump Maps matches *_bump.* first but
			// declines via its exclude list. Before the fix this dropped the file
			// entirely; it must now fall through to Scope Textures, which keeps every
			// part of a scope in one profile.
			name:       "scope bump declines Bump Maps and reaches Scope Textures",
			rel:        "gamedata/textures/wpn/wpn_aug/aug_scope_bump.dds",
			wantName:   "Scope Textures",
			wantFormat: "BC7_UNORM",
		},
		{
			name:       "lens bump declines Bump Maps and reaches Scope Textures",
			rel:        "gamedata/textures/wpn/elcan_lens_bump.dds",
			wantName:   "Scope Textures",
			wantFormat: "BC7_UNORM",
		},
		{
			name:       "hash-suffixed scope bump also falls through",
			rel:        "gamedata/textures/wpn/g3sg1_scope_bump#.dds",
			wantName:   "Scope Textures",
			wantFormat: "BC7_UNORM",
		},
		{
			// An exclude on one profile must not leak into unrelated files.
			name:       "ordinary bump map still claimed by Bump Maps",
			rel:        "gamedata/textures/wpn/ak74_bump.dds",
			wantName:   "Bump Maps",
			wantFormat: "BC7_UNORM",
		},
		{
			// The split the Bump Maps profile exists for: BC5 survives only for a
			// name that really does mean a two-channel normal map.
			name:       "true normal map still claimed by Normal Maps",
			rel:        "gamedata/textures/wpn/ak74_nrm.dds",
			wantName:   "Normal Maps",
			wantFormat: "BC5_UNORM",
		},
		{
			name:       "first matching profile wins",
			rel:        "gamedata/textures/ui/readables/note.dds",
			wantName:   "UI Readables",
			wantFormat: "BC3_UNORM",
		},
		{
			// A trailing /* is recursive, so nesting depth is irrelevant.
			name:       "path pattern matches at arbitrary depth",
			rel:        "gamedata/textures/ui/alticons/deep/bar.dds",
			wantName:   "UI / Icons",
			wantFormat: "BC3_UNORM",
		},
		{
			name:       "unmatched file yields empty profile",
			rel:        "gamedata/textures/misc/whatever.xyz",
			wantName:   "",
			wantFormat: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotFormat := matchProfile(tt.rel, testProfiles())
			if gotName != tt.wantName || gotFormat != tt.wantFormat {
				t.Errorf("matchProfile(%q) = (%q, %q), want (%q, %q)",
					tt.rel, gotName, gotFormat, tt.wantName, tt.wantFormat)
			}
		})
	}
}

// TestProfileExcludeAcceptsPathPatterns checks that a profile's exclude accepts path
// patterns, exactly like its patterns and the global excludePatterns. It previously
// matched the basename only, so a path pattern here was silently inert.
func TestProfileExcludeAcceptsPathPatterns(t *testing.T) {
	profiles := []config.Profile{
		{
			Name:     "Diffuse / Color",
			Format:   "BC3_UNORM",
			Exclude:  []string{"*/textures/wpn/scope_utility/*"},
			Patterns: []string{"*_d.*"},
		},
		{Name: "Weapon Textures", Format: "BC7_UNORM", Patterns: []string{"*/textures/wpn/*"}},
	}

	// Inside the excluded subtree — Diffuse / Color declines, Weapon Textures claims it.
	rel := "gamedata/textures/wpn/scope_utility/dirt_d.dds"
	if name, format := matchProfile(rel, profiles); name != "Weapon Textures" || format != "BC7_UNORM" {
		t.Errorf("got (%q, %q), want Weapon Textures/BC7_UNORM", name, format)
	}
	// Nested deeper — the exclude is recursive, so this declines too.
	rel = "gamedata/textures/wpn/scope_utility/sub/dirt_d.dds"
	if name, _ := matchProfile(rel, profiles); name != "Weapon Textures" {
		t.Errorf("got %q, want Weapon Textures — path exclude must cover the subtree", name)
	}
	// Outside it — Diffuse / Color still claims the file.
	rel = "gamedata/textures/wpn/ak74_d.dds"
	if name, _ := matchProfile(rel, profiles); name != "Diffuse / Color" {
		t.Errorf("got %q, want Diffuse / Color — exclude must not leak", name)
	}
}

// TestModRelPathFeedsPathPatterns guards the seam between the two walkers. Walk's rel is
// modsDir-relative and so carries a leading mod-name segment; WalkVirtual's relPath does
// not. Patterns are written against the mod root, and since * no longer crosses a
// separator that extra segment makes every path pattern miss — silently, as Unmatched.
// The earlier tests all passed mod-root-relative paths, so they only ever covered
// WalkVirtual and this regression went unnoticed.
func TestModRelPathFeedsPathPatterns(t *testing.T) {
	const walkRel = "Some Mod/gamedata/textures/ui/icon.dds"

	if got := modRelPath(walkRel); got != "gamedata/textures/ui/icon.dds" {
		t.Fatalf("modRelPath(%q) = %q", walkRel, got)
	}
	// The raw modsDir-relative path must NOT match — this is the shape of the bug.
	if matchPathPattern("*/textures/ui/*", walkRel) {
		t.Error("modsDir-relative path should not match; the mod segment must be stripped first")
	}
	// Both walkers must route an equivalent file to the same profile.
	name, format := matchProfile(modRelPath(walkRel), testProfiles())
	if name != "UI / Icons" || format != "BC3_UNORM" {
		t.Errorf("Walk shape routed to (%q, %q), want UI / Icons/BC3_UNORM", name, format)
	}
	virtualName, _ := matchProfile("gamedata/textures/ui/icon.dds", testProfiles())
	if virtualName != name {
		t.Errorf("Walk routed to %q but WalkVirtual routed to %q", name, virtualName)
	}
	// A mod folder with no subdirectory still yields a usable basename.
	if got := modRelPath("loose.dds"); got != "loose.dds" {
		t.Errorf("modRelPath(%q) = %q", "loose.dds", got)
	}
}

// TestMatchProfileDeclineWithNoFallthrough checks that a file declining every profile it
// matches is reported as unmatched rather than silently vanishing from the scan.
func TestMatchProfileDeclineWithNoFallthrough(t *testing.T) {
	profiles := []config.Profile{{
		Name:     "Bump Maps",
		Format:   "BC7_UNORM",
		Exclude:  []string{"*scope*bump*"},
		Patterns: []string{"*_bump.*"},
	}}

	name, format := matchProfile("x/aug_scope_bump.dds", profiles)
	if name != "" || format != "" {
		t.Errorf("got (%q, %q), want empty — caller labels this Unmatched", name, format)
	}
}

func TestMatchAnyPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		rel     string
		want    bool
	}{
		{"basename pattern matches regardless of depth", "fx_*", "gamedata/textures/sky/fx_sun_rays.dds", true},
		{"basename pattern does not match unrelated file", "lut_*", "gamedata/textures/sky/fx_sun_rays.dds", false},
		{"path pattern matches direct child", "*/textures/ui/SquareDOV/*", "gamedata/textures/ui/SquareDOV/minimap.dds", true},
		{
			// The regression this exists for: a path exclude must cover its whole
			// subtree. Under plain filepath.Match a trailing /* reached only direct
			// children, so nested files silently escaped exclusion while the identical
			// inclusion pattern matched them.
			name:    "path pattern matches nested descendant",
			pattern: "*/textures/ui/SquareDOV/*",
			rel:     "gamedata/textures/ui/SquareDOV/sub/minimap.dds",
			want:    true,
		},
		{
			// The leading */ still requires a separator before the anchor, matching
			// the previous substring behaviour (which looked for "/textures/ui/").
			// Real scan paths are always mod-root-relative, e.g. gamedata/textures/...
			name:    "leading star still requires a separator before the anchor",
			pattern: "*/textures/ui/SquareDOV/*",
			rel:     "textures/ui/SquareDOV/minimap.dds",
			want:    false,
		},
		{
			name:    "path pattern does not match a different subtree",
			pattern: "*/textures/ui/SquareDOV/*",
			rel:     "gamedata/textures/ui/alticons/minimap.dds",
			want:    false,
		},
		{
			name:    "? matches a single character but not a separator",
			pattern: "*/textures/ui/ico?.dds",
			rel:     "gamedata/textures/ui/icon.dds",
			want:    true,
		},
		{
			name:    "? does not match a separator",
			pattern: "*/textures/ui?icon.dds",
			rel:     "gamedata/textures/ui/icon.dds",
			want:    false,
		},
		{
			// A pattern is a full match, not a substring search.
			name:    "pattern is anchored at both ends",
			pattern: "*/textures/ui",
			rel:     "gamedata/textures/ui/icon.dds",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := tt.rel[strings.LastIndex(tt.rel, "/")+1:]
			if got := matchAnyPattern(base, tt.rel, []string{tt.pattern}); got != tt.want {
				t.Errorf("matchAnyPattern(%q, %q) = %v, want %v", base, tt.rel, got, tt.want)
			}
		})
	}
}

// TestPathPatternCoversNestedFiles checks that a trailing /* covers the whole subtree,
// not just direct children — the property a path-based exclude depends on.
func TestPathPatternCoversNestedFiles(t *testing.T) {
	const pattern = "*/textures/ui/*"
	rels := []string{
		"gamedata/textures/ui/icon.dds",
		"gamedata/textures/ui/alticons/icon.dds",
		"gamedata/textures/ui/a/b/c/icon.dds",
	}
	for _, rel := range rels {
		if !matchPathPattern(pattern, rel) {
			t.Errorf("matchPathPattern(%q, %q) = false, want true", pattern, rel)
		}
	}
	for _, rel := range []string{
		"gamedata/textures/wpn/icon.dds",
		"gamedata/textures/uix/icon.dds",
	} {
		if matchPathPattern(pattern, rel) {
			t.Errorf("matchPathPattern(%q, %q) = true, want false", pattern, rel)
		}
	}
}

// TestPathPatternBareSlashEqualsStar checks that a bare trailing / is shorthand for /* —
// the same recursive subtree match. Without this a directory pattern written as dir/
// compiled to ^dir/$ and silently matched nothing.
func TestPathPatternBareSlashEqualsStar(t *testing.T) {
	rels := []string{
		"gamedata/textures/ui/icon.dds",       // direct child
		"gamedata/textures/ui/alticons/x.dds", // nested
		"gamedata/textures/ui/a/b/c/x.dds",    // deeper
		"gamedata/textures/wpn/icon.dds",      // outside the subtree
		"gamedata/textures/uix/icon.dds",      // adjacent, must not leak
	}
	for _, rel := range rels {
		star := matchPathPattern("*/textures/ui/*", rel)
		slash := matchPathPattern("*/textures/ui/", rel)
		if star != slash {
			t.Errorf("dir/ and dir/* disagree on %q: star=%v slash=%v", rel, star, slash)
		}
	}
	// And it genuinely reaches a nested file, not just agrees on nothing.
	if !matchPathPattern("*/textures/ui/", "gamedata/textures/ui/a/b/x.dds") {
		t.Error("bare trailing / must cover the whole subtree")
	}
}

// TestPathPatternStarDoesNotCrossSeparator checks that away from a trailing /*, a star
// stays inside one path segment — the recursive case is the deliberate exception.
func TestPathPatternStarDoesNotCrossSeparator(t *testing.T) {
	const pattern = "*/textures/wpn/scope_*"
	if !matchPathPattern(pattern, "gamedata/textures/wpn/scope_30mm.dds") {
		t.Error("a star should match within a segment")
	}
	if matchPathPattern(pattern, "gamedata/textures/wpn/scope_reticles/lens.dds") {
		t.Error("a star must not cross a separator")
	}
	// A leading star is one segment too, so it cannot absorb extra depth.
	if matchPathPattern("*/textures/ui/*", "mods/gamedata/textures/ui/icon.dds") {
		t.Error("a leading star must not span multiple segments")
	}
}

// TestPathPatternRegexpIsSafeAndCached checks that compilation is cached and that a
// metacharacter cannot blow up compilation or leak into the generated expression.
func TestPathPatternRegexpIsSafeAndCached(t *testing.T) {
	pattern := `*/textures/ui/a+b(c).dds`
	if matchPathPattern(pattern, "gamedata/textures/ui/a+b(c).dds") != true {
		t.Error("literal metacharacters should match themselves")
	}
	if matchPathPattern(pattern, "gamedata/textures/ui/aaab.dds") != false {
		t.Error("metacharacters must not be interpreted as regexp syntax")
	}
	first := pathPatternRegexp(pattern)
	if second := pathPatternRegexp(pattern); first != second {
		t.Error("expected the compiled pattern to be cached")
	}
}

// TestExcludesDir covers the directory-exclusion matcher: a name-only pattern matches a
// directory's name; a path pattern matches its mod-root-relative path, and the three
// directory-naming forms (bare, trailing /, trailing /*) are equivalent. Previously a
// path pattern here was silently inert because only the basename was ever matched.
func TestExcludesDir(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		dirName  string
		relSlash string
		want     bool
	}{
		{"name matches directory name anywhere", "downloads", "downloads", "gamedata/downloads", true},
		{"name match is case-insensitive", "downloads", "Downloads", "gamedata/Downloads", true},
		{"dotfile glob matches hidden dir", ".*", ".git", ".git", true},
		{"name does not match a different dir", "downloads", "textures", "gamedata/textures", false},
		{"path pattern matches nested dir", "*/textures/ui/SquareDOV", "SquareDOV", "gamedata/textures/ui/SquareDOV", true},
		{"trailing / names the same dir", "*/textures/ui/SquareDOV/", "SquareDOV", "gamedata/textures/ui/SquareDOV", true},
		{"trailing /* names the same dir", "*/textures/ui/SquareDOV/*", "SquareDOV", "gamedata/textures/ui/SquareDOV", true},
		{"path pattern does not match a sibling dir", "*/textures/ui/SquareDOV", "alticons", "gamedata/textures/ui/alticons", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := excludesDir(tt.dirName, tt.relSlash, []string{tt.pattern}); got != tt.want {
				t.Errorf("excludesDir(%q, %q, %q) = %v, want %v", tt.dirName, tt.relSlash, tt.pattern, got, tt.want)
			}
		})
	}
}

// TestUnderExcludedDir covers mod-output mode, which has no directory walk to prune: a
// path exclusion must skip every file beneath the named directory, in all three naming
// forms. Name-only patterns are mod-level (applied by ExcludesMod) and must NOT filter
// files here, or they would double as basename globs.
func TestUnderExcludedDir(t *testing.T) {
	const nested = "gamedata/textures/ui/SquareDOV/sub/minimap.dds"
	for _, pattern := range []string{
		"*/textures/ui/SquareDOV",
		"*/textures/ui/SquareDOV/",
		"*/textures/ui/SquareDOV/*",
	} {
		if !underExcludedDir(nested, []string{pattern}) {
			t.Errorf("pattern %q must exclude nested file %q", pattern, nested)
		}
	}
	if underExcludedDir("gamedata/textures/ui/alticons/minimap.dds", []string{"*/textures/ui/SquareDOV"}) {
		t.Error("a path exclusion must not leak to a sibling subtree")
	}
	// A name-only exclusion is a mod filter, not a file filter — it must not match files.
	if underExcludedDir("gamedata/textures/ui/downloads/x.dds", []string{"downloads"}) {
		t.Error("name-only exclusions must not be applied to files in mod-output mode")
	}
}

// TestExcludesMod checks that whole-mod filtering matches a mod name but that a nested
// path pattern (which targets a directory inside a mod) never filters a bare mod name —
// those are honored per file by underExcludedDir instead.
func TestExcludesMod(t *testing.T) {
	if !ExcludesMod("G.A.M.M.A. UI", []string{"G.A.M.M.A. UI"}) {
		t.Error("a name pattern must filter the matching mod")
	}
	if ExcludesMod("G.A.M.M.A. UI", []string{"*/textures/ui/SquareDOV"}) {
		t.Error("a nested path pattern must not filter a whole mod")
	}
}
