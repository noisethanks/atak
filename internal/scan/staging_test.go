package scan

import (
	"os"
	"path/filepath"
	"testing"
)

// A staging dir at scan time is stale from a crash. Both walkers must skip and
// delete it even when scanExclusions is empty.
func TestWalkersSkipAndRemoveStagingDir(t *testing.T) {
	setup := func(t *testing.T) (modsDir, stage, file string) {
		modsDir = t.TempDir()
		stage = filepath.Join(modsDir, "ATAK", "gamedata", "textures", "wpn", StagingDirName)
		job := filepath.Join(stage, "job-123", "out")
		if err := os.MkdirAll(job, 0755); err != nil {
			t.Fatal(err)
		}
		file = filepath.Join(job, "ak74_d.dds")
		if err := os.WriteFile(file, []byte("partial"), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	drain := func(t *testing.T, assets <-chan Asset, stats <-chan Stats) {
		for a := range assets {
			t.Errorf("staging file emitted as asset: %s", a.Path)
		}
		if s := <-stats; len(s.Unreadable) != 0 {
			t.Errorf("staging file reported unreadable: %+v", s.Unreadable)
		}
	}

	t.Run("Walk", func(t *testing.T) {
		modsDir, stage, _ := setup(t)
		assets, stats, _ := Walk(modsDir, testProfiles(), nil, []string{}, 0)
		drain(t, assets, stats)
		if _, err := os.Stat(stage); !os.IsNotExist(err) {
			t.Errorf("stale staging dir not removed: %v", err)
		}
	})

	t.Run("WalkVirtual", func(t *testing.T) {
		modsDir, stage, file := setup(t)
		vfs := map[string]string{"gamedata/textures/wpn/" + StagingDirName + "/job-123/out/ak74_d.dds": file}
		assets, stats, _ := WalkVirtual(vfs, modsDir, testProfiles(), nil, []string{}, 0)
		drain(t, assets, stats)
		if _, err := os.Stat(stage); !os.IsNotExist(err) {
			t.Errorf("stale staging dir not removed: %v", err)
		}
	})
}

// modsDir itself sits under a directory named .atak-staging. WalkVirtual must
// remove only the mod's own staging dir, never anything above the mod.
func TestWalkVirtualStagingRemovalStaysInsideMod(t *testing.T) {
	outer := filepath.Join(t.TempDir(), StagingDirName)
	modsDir := filepath.Join(outer, "mods")
	keep := filepath.Join(outer, "keep.txt")
	modFile := filepath.Join(modsDir, "ATAK", "gamedata", "textures", "ak74_d.dds")
	stage := filepath.Join(modsDir, "ATAK", "gamedata", "textures", StagingDirName)
	stageFile := filepath.Join(stage, "job-1", "out", "ak74_d.dds")
	for _, f := range []string{keep, modFile, stageFile} {
		if err := os.MkdirAll(filepath.Dir(f), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	vfs := map[string]string{
		"gamedata/textures/" + StagingDirName + "/job-1/out/ak74_d.dds": stageFile,
	}
	assets, stats, _ := WalkVirtual(vfs, modsDir, testProfiles(), nil, []string{}, 0)
	for range assets {
	}
	<-stats

	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Errorf("mod's staging dir not removed: %v", err)
	}
	for _, f := range []string{keep, modFile} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("outside file removed: %s", f)
		}
	}
}

func TestStagingRootMismatchReturnsFalse(t *testing.T) {
	if _, ok := stagingRoot("a/"+StagingDirName+"/j/f.dds", "/mods/M/other/f.dds"); ok {
		t.Error("mismatched abs path must not yield a directory")
	}
}
