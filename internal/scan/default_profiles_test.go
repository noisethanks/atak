package scan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/noisethanks/atak/internal/config"
)

// defaultProfilesPath is the file config embeds and writes out on first run. The
// test reads it from disk rather than through config.LoadProfiles, which would
// return whatever the machine running the test has in its user config dir.
const defaultProfilesPath = "../config/configs/compression_profiles.json"

func loadDefaultProfiles(t *testing.T) ([]config.Profile, []string) {
	t.Helper()
	data, err := os.ReadFile(defaultProfilesPath)
	if err != nil {
		t.Fatalf("read shipped profiles: %v", err)
	}
	var pf struct {
		ExcludePatterns []string         `json:"excludePatterns"`
		Profiles        []config.Profile `json:"profiles"`
	}
	if err := json.Unmarshal(data, &pf); err != nil {
		t.Fatalf("parse shipped profiles: %v", err)
	}
	return pf.Profiles, pf.ExcludePatterns
}

// TestDefaultProfilesNeverSendBumpMapsToBC5 is the regression guard for the worst
// bug the shipped defaults have had: every X-Ray bump map was compressed to BC5.
//
// BC5 stores two channels. A sampled BC5 texture returns blue as 0 and alpha as 1,
// and an X-Ray bump uses all four: sload.h reads gloss from R and unpacks the
// normal from .wzy, so alpha and blue carry two of the three normal components.
// The paired _bump# file carries parallax height in alpha. BC5 therefore does not
// degrade these textures, it deletes most of what they hold.
//
// The names below are real files from a GAMMA modlist, one per naming convention
// that reached the BC5 profile.
func TestDefaultProfilesNeverSendBumpMapsToBC5(t *testing.T) {
	profiles, _ := loadDefaultProfiles(t)

	bumps := []string{
		"gamedata/textures/wpn/wpn_addon_acog_ironsight_bump.dds",
		"gamedata/textures/act/act_stalker_nato_bump#.dds",
		"gamedata/textures/wpn/HE510C_Nbump.dds",
		"gamedata/textures/rwap/TA31_normalbump.dds",
		// The # sibling of every bump convention has to be covered too. _bump had
		// one from the start; _normalbump did not, so 35 real files matched no
		// profile at all and would have been skipped rather than compressed.
		"gamedata/textures/p90gamma/p90_muzzle_normalbump#.dds",
		"gamedata/textures/wpn/wpn_aug/aug_scope_bump.dds",
		"gamedata/textures/wpn/elcan_lens_bump.dds",
	}
	for _, rel := range bumps {
		name, format := matchProfile(rel, profiles)
		if name == "" {
			t.Errorf("%s: no profile matched — a bump map must not fall through to Unmatched", rel)
			continue
		}
		if format == "BC5_UNORM" {
			t.Errorf("%s: profile %q gives BC5_UNORM; an X-Ray bump needs all four channels", rel, name)
		}
	}
}

// TestDefaultProfilesKeepBC5ForTrueNormalMaps is the other half of the fix. BC5
// is the right format for a genuine two-channel tangent-space normal map, and it
// costs the same 1 byte per texel as BC7, so the suffixes that really do mean
// "XY normal only" keep it.
func TestDefaultProfilesKeepBC5ForTrueNormalMaps(t *testing.T) {
	profiles, _ := loadDefaultProfiles(t)

	for _, rel := range []string{
		"gamedata/textures/wpn/R_Pantek_D/rpd_norm.dds",
		"gamedata/textures/act/act_hood_nm.dds",
		"gamedata/textures/wpn/ak74_nrm.dds",
	} {
		name, format := matchProfile(rel, profiles)
		if format != "BC5_UNORM" {
			t.Errorf("%s: profile %q gives %s, want BC5_UNORM", rel, name, format)
		}
	}
}

// TestDefaultProfilesRoutePerCategory pins the rest of the routing the bump split
// could have disturbed — a new profile ahead of the others changes what every
// later pattern sees.
func TestDefaultProfilesRoutePerCategory(t *testing.T) {
	profiles, excludePatterns := loadDefaultProfiles(t)

	tests := []struct {
		rel        string
		wantName   string
		wantFormat string
	}{
		{"gamedata/textures/wpn/scope_crosshair_hud.dds", "Sights / Reticles", "BC3_UNORM"},
		// A suffix profile outranks a folder profile: *_d.* is Diffuse / Color even
		// under textures/wpn, and only a weapon texture with no suffix of its own
		// reaches Weapon Textures.
		{"gamedata/textures/wpn/wpn_ak74_d.dds", "Diffuse / Color", "BC7_UNORM"},
		{"gamedata/textures/wpn/wpn_ak74.dds", "Weapon Textures", "BC7_UNORM"},
		{"gamedata/textures/ui/ui_icon_equipment.dds", "UI / Icons", "BC3_UNORM"},
		{"gamedata/textures/ui/readables/pda_note.dds", "UI Readables", "BC3_UNORM"},
		{"gamedata/textures/sky/cloudy3/00-00.dds", "Sky Textures", "BC3_UNORM"},
		{"gamedata/textures/act/act_hood.dds", "Character / Hands", "BC7_UNORM"},
	}
	for _, tc := range tests {
		name, format := matchProfile(tc.rel, profiles)
		if name != tc.wantName || format != tc.wantFormat {
			t.Errorf("%s: got %q/%s, want %q/%s", tc.rel, name, format, tc.wantName, tc.wantFormat)
		}
	}

	// A global exclude still wins over every profile, bump patterns included.
	for _, rel := range []string{
		"gamedata/textures/sky/sky_test_cube#0.dds",
		"gamedata/textures/detail/detail_grnd_asphalt_lm.dds",
	} {
		if !matchAnyPattern(filepath.Base(rel), rel, excludePatterns) {
			t.Errorf("%s: expected a global excludePatterns match", rel)
		}
	}
}

// TestShippedProfilesMatchRepoRoot keeps the two copies of the defaults in step.
// SPEC.md points users at the repo-root profiles.json, while the build embeds the
// one under internal/config/configs, so a fix applied to one and not the other
// ships a file that disagrees with its own documentation.
func TestShippedProfilesMatchRepoRoot(t *testing.T) {
	embedded, err := os.ReadFile(defaultProfilesPath)
	if err != nil {
		t.Fatalf("read embedded profiles: %v", err)
	}
	root, err := os.ReadFile("../../profiles.json")
	if err != nil {
		t.Fatalf("read repo-root profiles.json: %v", err)
	}
	if string(embedded) != string(root) {
		t.Error("profiles.json at the repo root differs from internal/config/configs/compression_profiles.json")
	}
}
