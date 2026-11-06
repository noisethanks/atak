package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanPath(t *testing.T) {
	spaced := filepath.Join("C:", "Games", "Anomaly GAMMA", "mods")
	cases := []struct{ name, in, want string }{
		{"empty stays empty", "", ""},
		{"whitespace only stays empty", "   ", ""},
		{"empty quotes stay empty", `""`, ""},
		{"plain path", spaced, spaced},
		{"double-quoted path with spaces", `"` + spaced + `"`, spaced},
		{"single-quoted path with spaces", `'` + spaced + `'`, spaced},
		{"trailing space", spaced + " ", spaced},
		{"whitespace around quotes", "  \"" + spaced + "\"\t", spaced},
		{"trailing separator cleaned", spaced + string(filepath.Separator), spaced},
		{"mismatched quotes left alone", `"` + spaced + `'`, filepath.Clean(`"` + spaced + `'`)},
		{"lone leading quote left alone", `"` + spaced, filepath.Clean(`"` + spaced)},
		{"quote mid-string left alone", filepath.Join("mods", `Bob's "Big" Mod`), filepath.Join("mods", `Bob's "Big" Mod`)},
		{"only one pair stripped", `""` + spaced + `""`, filepath.Clean(`"` + spaced + `"`)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CleanPath(c.in); got != c.want {
				t.Errorf("CleanPath(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// A config.json written by an earlier version with a quoted modlistPath, as
// produced by Windows "Copy as path", must load to a path that opens.
func TestLoadCleansQuotedPaths(t *testing.T) {
	dir := redirectConfigDir(t)
	root := t.TempDir()
	profile := filepath.Join(root, "MO2 Profiles", "Default Profile")
	modsDir := filepath.Join(root, "My Mods")
	for _, d := range []string{profile, modsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	modlistPath := filepath.Join(profile, "modlist.txt")
	if err := os.WriteFile(modlistPath, []byte("+Some Mod\n"), 0644); err != nil {
		t.Fatal(err)
	}

	raw := map[string]any{
		"modsDir":               `"` + modsDir + `" `,
		"backupDir":             `'` + filepath.Join(root, "Backups Dir") + `'`,
		"modlistPath":           `"` + modlistPath + `"`,
		"workerCount":           1,
		"backupLevel":           6,
		"modOutputMode":         true,
		"modOutputName":         "ATAK",
		"stripMipsWhenDisabled": false,
		"compressionBackend":    "texconv",
		"scanExclusions":        []string{},
	}
	data, _ := json.Marshal(raw)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ModlistPath != modlistPath {
		t.Errorf("ModlistPath = %q, want %q", cfg.ModlistPath, modlistPath)
	}
	if cfg.ModsDir != modsDir {
		t.Errorf("ModsDir = %q, want %q", cfg.ModsDir, modsDir)
	}
	if want := filepath.Join(root, "Backups Dir"); cfg.BackupDir != want {
		t.Errorf("BackupDir = %q, want %q", cfg.BackupDir, want)
	}
	// modlist imports config, so open the file directly rather than via ParseModList.
	if got, err := os.ReadFile(cfg.ModlistPath); err != nil || string(got) != "+Some Mod\n" {
		t.Errorf("modlist from loaded path: %q, %v", got, err)
	}
	if fi, err := os.Stat(cfg.ModsDir); err != nil || !fi.IsDir() {
		t.Errorf("loaded modsDir does not resolve: %v", err)
	}
}
