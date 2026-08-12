package screens

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/noisethanks/atak/internal/compress"
	"github.com/noisethanks/atak/internal/config"
	"github.com/noisethanks/atak/internal/scan"
)

// buildTestGroups returns two profile groups (Normal Maps, Diffuse), each
// containing one asset from "ModA" and one from "ModB" — enough to exercise
// every combination of per-category / per-mod grouping.
func buildTestGroups() []AssetGroup {
	return []AssetGroup{
		{
			ProfileName:  "Normal Maps",
			SuggestedFmt: "BC5_UNORM",
			Assets: []assetRef{
				{Path: "/mods/ModA/gamedata/textures/x_bump.dds", ModName: "ModA", VirtualRelPath: "gamedata/textures/x_bump.dds"},
				{Path: "/mods/ModB/gamedata/textures/y_bump.dds", ModName: "ModB", VirtualRelPath: "gamedata/textures/y_bump.dds"},
			},
		},
		{
			ProfileName:  "Diffuse",
			SuggestedFmt: "BC3_UNORM",
			Assets: []assetRef{
				{Path: "/mods/ModA/gamedata/textures/x_diff.dds", ModName: "ModA", VirtualRelPath: "gamedata/textures/x_diff.dds"},
				{Path: "/mods/ModB/gamedata/textures/y_diff.dds", ModName: "ModB", VirtualRelPath: "gamedata/textures/y_diff.dds"},
			},
		},
	}
}

// runBuildJobs drives ResultsModel.buildJobs synchronously (it returns a
// tea.Cmd — a func() tea.Msg — so calling that func executes it inline) and
// unwraps the resulting CompressJobData.
func runBuildJobs(t *testing.T, cfg *config.Config) CompressJobData {
	t.Helper()
	m := ResultsModel{groups: buildTestGroups(), cfg: cfg}
	cmd := m.buildJobs(0, "", "") // scope 0 = Run All
	msg := cmd()
	nav, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("expected NavigateMsg, got %T", msg)
	}
	data, ok := nav.Data.(CompressJobData)
	if !ok {
		t.Fatalf("expected CompressJobData, got %T", nav.Data)
	}
	return data
}

// jobDirsByPath maps each job's source path to the ModOutputDir compress.go's
// buildJobs actually resolved for it (group-level, per-asset, or fallback —
// whichever wins per the priority in buildJobs).
func jobDirsByPath(t *testing.T, data CompressJobData) map[string]string {
	t.Helper()
	jobs := buildJobs(data)
	out := make(map[string]string, len(jobs))
	for _, j := range jobs {
		out[j.Asset.Path] = j.ModOutputDir
	}
	return out
}

func baseCfg(modsDir string) *config.Config {
	return &config.Config{
		ModsDir:       modsDir,
		ModOutputMode: true,
		ModOutputName: "ATAK",
	}
}

func TestModOutputGrouping_Off(t *testing.T) {
	cfg := baseCfg("/mods")
	data := runBuildJobs(t, cfg)
	dirs := jobDirsByPath(t, data)

	want := filepath.Join("/mods", "ATAK")
	for path, dir := range dirs {
		if dir != want {
			t.Errorf("%s: got dir %q, want flat %q", path, dir, want)
		}
	}
}

func TestModOutputGrouping_PerCategoryOnly(t *testing.T) {
	cfg := baseCfg("/mods")
	cfg.PerCategoryModOutput = true
	data := runBuildJobs(t, cfg)
	dirs := jobDirsByPath(t, data)

	wantNormal := filepath.Join("/mods", "ATAK - Normal Maps")
	wantDiffuse := filepath.Join("/mods", "ATAK - Diffuse")

	check := []struct {
		path string
		want string
	}{
		{"/mods/ModA/gamedata/textures/x_bump.dds", wantNormal},
		{"/mods/ModB/gamedata/textures/y_bump.dds", wantNormal},
		{"/mods/ModA/gamedata/textures/x_diff.dds", wantDiffuse},
		{"/mods/ModB/gamedata/textures/y_diff.dds", wantDiffuse},
	}
	for _, c := range check {
		if got := dirs[c.path]; got != c.want {
			t.Errorf("%s: got dir %q, want %q", c.path, got, c.want)
		}
	}
	// ModA and ModB must land in the SAME folder per category — that's the point.
	if dirs["/mods/ModA/gamedata/textures/x_bump.dds"] != dirs["/mods/ModB/gamedata/textures/y_bump.dds"] {
		t.Error("per-category should merge different mods into one folder, they diverged")
	}
}

func TestModOutputGrouping_PerModOnly(t *testing.T) {
	cfg := baseCfg("/mods")
	cfg.PerModModOutput = true
	data := runBuildJobs(t, cfg)
	dirs := jobDirsByPath(t, data)

	wantA := filepath.Join("/mods", "ATAK - ModA")
	wantB := filepath.Join("/mods", "ATAK - ModB")

	check := []struct {
		path string
		want string
	}{
		{"/mods/ModA/gamedata/textures/x_bump.dds", wantA},
		{"/mods/ModA/gamedata/textures/x_diff.dds", wantA},
		{"/mods/ModB/gamedata/textures/y_bump.dds", wantB},
		{"/mods/ModB/gamedata/textures/y_diff.dds", wantB},
	}
	for _, c := range check {
		if got := dirs[c.path]; got != c.want {
			t.Errorf("%s: got dir %q, want %q", c.path, got, c.want)
		}
	}
	// Normal Maps and Diffuse from the SAME mod must land in the SAME folder.
	if dirs["/mods/ModA/gamedata/textures/x_bump.dds"] != dirs["/mods/ModA/gamedata/textures/x_diff.dds"] {
		t.Error("per-mod should merge different categories into one folder, they diverged")
	}
}

func TestModOutputGrouping_Both(t *testing.T) {
	cfg := baseCfg("/mods")
	cfg.PerCategoryModOutput = true
	cfg.PerModModOutput = true
	data := runBuildJobs(t, cfg)
	dirs := jobDirsByPath(t, data)

	want := map[string]string{
		"/mods/ModA/gamedata/textures/x_bump.dds": filepath.Join("/mods", "ATAK - ModA - Normal Maps"),
		"/mods/ModB/gamedata/textures/y_bump.dds": filepath.Join("/mods", "ATAK - ModB - Normal Maps"),
		"/mods/ModA/gamedata/textures/x_diff.dds": filepath.Join("/mods", "ATAK - ModA - Diffuse"),
		"/mods/ModB/gamedata/textures/y_diff.dds": filepath.Join("/mods", "ATAK - ModB - Diffuse"),
	}
	for path, wantDir := range want {
		if got := dirs[path]; got != wantDir {
			t.Errorf("%s: got dir %q, want %q", path, got, wantDir)
		}
	}
	// All four must be distinct folders — no accidental merging.
	seen := map[string]bool{}
	for _, dir := range dirs {
		if seen[dir] {
			t.Errorf("folder %q reused across mod+category combos, expected all distinct", dir)
		}
		seen[dir] = true
	}
}

// TestScanExclusionCoversAllGroupingShapes confirms the "ATAK - *" glob added
// in scan.go actually matches every folder-name shape the grouping modes can
// produce, so previously-compressed output is never rescanned as a source mod.
func TestScanExclusionCoversAllGroupingShapes(t *testing.T) {
	pattern := "ATAK - *"
	names := []string{
		"ATAK - Normal Maps",
		"ATAK - ModA",
		"ATAK - ModA - Normal Maps",
	}
	for _, name := range names {
		if !scan.ExcludesMod(name, []string{pattern}) {
			t.Errorf("pattern %q should exclude mod folder %q, did not", pattern, name)
		}
	}
	// Sanity: an unrelated mod name must NOT be excluded.
	if scan.ExcludesMod("SomeOtherMod", []string{pattern}) {
		t.Error("pattern should not match an unrelated mod name")
	}
}

// TestIncrementalSkip_PerCategoryIsolatesOutputPaths verifies the worker's
// path-based skip check (worker.go) treats each category's output folder as
// independent — a file already compressed under "ATAK - Normal Maps" must not
// cause a same-named-but-different-category file to be skipped, and vice
// versa. Uses a fake Backend so no real texconv/compressonator binary is
// needed; the fake fails the test if invoked on a job we expect to be skipped.
func TestIncrementalSkip_PerCategoryIsolatesOutputPaths(t *testing.T) {
	tmp := t.TempDir()
	modsDir := filepath.Join(tmp, "mods")

	normalDir := filepath.Join(modsDir, "ATAK - Normal Maps")
	diffuseDir := filepath.Join(modsDir, "ATAK - Diffuse")
	relPath := "gamedata/textures/x.dds"

	// Pre-create the output file only under Normal Maps, simulating a prior run.
	if err := os.MkdirAll(filepath.Dir(filepath.Join(normalDir, relPath)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(normalDir, relPath), []byte("fake compressed dds"), 0644); err != nil {
		t.Fatal(err)
	}

	invoked := map[string]bool{}
	fake := &fakeBackend{
		onCompress: func(job compress.Job) {
			invoked[job.ModOutputDir] = true
		},
	}

	jobs := []compress.Job{
		{Asset: scan.Asset{Path: "/mods/ModA/x.dds"}, RelPath: relPath, ModOutputDir: normalDir},
		{Asset: scan.Asset{Path: "/mods/ModA/x.dds"}, RelPath: relPath, ModOutputDir: diffuseDir},
	}

	ctx := context.Background()
	results := compress.RunPool(ctx, fake, nil, jobs, 1)
	var skipped, ran int
	for r := range results {
		if r.OutputSkipped {
			skipped++
		} else {
			ran++
		}
	}

	if skipped != 1 {
		t.Errorf("expected exactly 1 skipped job (Normal Maps, pre-existing output), got %d", skipped)
	}
	if ran != 1 {
		t.Errorf("expected exactly 1 job to actually run (Diffuse, no pre-existing output), got %d", ran)
	}
	if invoked[normalDir] {
		t.Error("Normal Maps job should have been skipped, but backend was invoked for it")
	}
	if !invoked[diffuseDir] {
		t.Error("Diffuse job should have run (no pre-existing output), but backend was never invoked")
	}
}

type fakeBackend struct {
	onCompress func(job compress.Job)
}

func (f *fakeBackend) Name() string { return "fake" }

func (f *fakeBackend) Compress(ctx context.Context, job compress.Job) compress.CompressionResult {
	if f.onCompress != nil {
		f.onCompress(job)
	}
	return compress.CompressionResult{Asset: job.Asset, Success: true, Backend: "fake"}
}
