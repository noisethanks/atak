package config

import (
	"encoding/json"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

//go:embed configs/compression_profiles.json
var defaultProfilesJSON []byte

// Profile defines a texture compression target matched by filename pattern.
type Profile struct {
	Name           string   `json:"name"`
	Format         string   `json:"format"`
	Patterns       []string `json:"patterns"`
	Exclude        []string `json:"exclude,omitempty"`
	GenerateMips   bool     `json:"generateMips,omitempty"`
	MaxTextureSize int      `json:"maxTextureSize,omitempty"`
}

type profileFile struct {
	ExcludePatterns  []string  `json:"excludePatterns,omitempty"`
	MinFileSizeBytes int       `json:"minFileSizeBytes,omitempty"`
	Profiles         []Profile `json:"profiles"`
}

// Compression backend identifiers, used for Config.CompressionBackend and by the
// compress package's backend registry. Kept as string constants (not iota) so the
// value is stable across builds and legible in the on-disk config.json.
const (
	BackendTexconv            = "texconv"
	BackendCompressonatorBc7e = "compressonator-bc7e"
)

// Config holds all user-persisted preferences.
//
// Field groups:
//   - Path fields (modsDir, backupDir, modlistPath): never block startup; validated
//     live at point of use and routed to the path-config screen when absent or invalid.
//   - All other fields: must be explicitly present in config.json with the correct JSON
//     type. A missing or wrong-typed field causes Load to return an error and the app
//     will not start. See validateConfig.
type Config struct {
	ModsDir   string `json:"modsDir"`
	BackupDir string `json:"backupDir"`

	WorkerCount int `json:"workerCount"`
	BackupLevel int `json:"backupLevel"`
	// CompressionBackend selects which embedded compressor runs. Valid values:
	// "texconv" (default, all platforms) and "compressonator-bc7e" (Linux/Windows
	// only). On darwin the resolved value is always coerced back to "texconv" on
	// load, so a config synced from another OS can't select an unavailable backend.
	// This coercion is expected to be removed once macOS gains compressonator-bc7e
	// support; validateConfig needs no changes at that point.
	CompressionBackend string `json:"compressionBackend"`
	// ScanExclusions are directory globs pruned during the scan. A plain name matches
	// a directory (or mod) anywhere; a path pattern like */textures/ui/SquareDOV
	// matches a nested directory, sharing the profiles.json pattern syntax. A matched
	// directory and its whole subtree are skipped. An empty array ([]) is valid.
	ScanExclusions        []string `json:"scanExclusions"`
	ModOutputMode         bool     `json:"modOutputMode"`
	ModOutputName         string   `json:"modOutputName"`
	ModlistPath           string   `json:"modlistPath"`
	// StripMipsWhenDisabled makes a profile's generateMips:false authoritative: source
	// mip chains are dropped instead of preserved. Off by default, where the source's
	// own mip count decides for generateMips:false profiles (see compress.ShouldGenerateMips).
	StripMipsWhenDisabled bool `json:"stripMipsWhenDisabled"`
}

func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "atak"), nil
}

// ConfigDir returns the atak config directory path.
func ConfigDir() (string, error) {
	return configDir()
}

// Load reads config.json from the user config dir. Returns defaults on first run
// (file absent). Returns an error if the file exists but contains invalid JSON,
// a missing required field, a wrong-typed field, or an invalid compressionBackend
// value — the app will not start in any of those cases.
func Load() (*Config, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultConfig(), nil
	}
	if err != nil {
		return nil, err
	}
	if err := validateConfig(data); err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	cfg.CompressionBackend = normalizeBackend(cfg.CompressionBackend)
	return &cfg, nil
}

// validateConfig checks that all must-be-explicit config.json fields are present and
// correctly typed. Path fields (modsDir, backupDir, modlistPath) are intentionally
// excluded — they are validated live at point of use.
//
// Every failure returns a consistent error naming the field and the problem; raw
// stdlib json error text never surfaces to the caller.
func validateConfig(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("config.json: invalid JSON: %w", err)
	}

	type fieldSpec struct {
		key      string
		jsonType string
	}
	for _, f := range []fieldSpec{
		{"workerCount", "number"},
		{"backupLevel", "number"},
		{"modOutputMode", "boolean"},
		{"modOutputName", "string"},
		{"stripMipsWhenDisabled", "boolean"},
	} {
		v, ok := raw[f.key]
		if !ok {
			return fmt.Errorf("config.json: field %q is missing", f.key)
		}
		if got := jsonKind(v); got != f.jsonType {
			return fmt.Errorf("config.json: field %q has wrong type (expected %s, got %s)", f.key, f.jsonType, got)
		}
	}

	// scanExclusions: must be present; an empty array is valid, but the key must exist
	// and the value must be an array of strings.
	excl, ok := raw["scanExclusions"]
	if !ok {
		return fmt.Errorf("config.json: field %q is missing", "scanExclusions")
	}
	if jsonKind(excl) != "array" {
		return fmt.Errorf("config.json: field %q has wrong type (expected array of strings, got %s)", "scanExclusions", jsonKind(excl))
	}
	var exclSlice []string
	if err := json.Unmarshal(excl, &exclSlice); err != nil {
		return fmt.Errorf("config.json: field %q has wrong type (expected array of strings, elements must be strings)", "scanExclusions")
	}

	// compressionBackend: must be present, must be a string, and must be exactly one
	// of the two valid backend identifiers. Applies uniformly on all platforms —
	// the darwin coercion in normalizeBackend is a separate post-load step.
	backendRaw, ok := raw["compressionBackend"]
	if !ok {
		return fmt.Errorf("config.json: field %q is missing", "compressionBackend")
	}
	if jsonKind(backendRaw) != "string" {
		return fmt.Errorf("config.json: field %q has wrong type (expected string, got %s)", "compressionBackend", jsonKind(backendRaw))
	}
	var backend string
	_ = json.Unmarshal(backendRaw, &backend)
	if backend != BackendTexconv && backend != BackendCompressonatorBc7e {
		return fmt.Errorf("config.json: field %q has invalid value %q (must be %q or %q)",
			"compressionBackend", backend, BackendTexconv, BackendCompressonatorBc7e)
	}

	return nil
}

// jsonKind returns the JSON type name for a RawMessage, inspecting the first byte.
func jsonKind(v json.RawMessage) string {
	if len(v) == 0 {
		return "null"
	}
	switch v[0] {
	case 't', 'f':
		return "boolean"
	case '"':
		return "string"
	case '[':
		return "array"
	case '{':
		return "object"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// normalizeBackend coerces a backend value to one available on the current platform.
// On darwin, always returns BackendTexconv regardless of the stored value, so a config
// synced from another OS can't select an unavailable backend. This darwin branch is
// expected to be removed once macOS gains compressonator-bc7e support; validateConfig
// needs no changes at that point.
func normalizeBackend(v string) string {
	if runtime.GOOS == "darwin" {
		return BackendTexconv
	}
	switch v {
	case BackendTexconv, BackendCompressonatorBc7e:
		return v
	}
	return BackendTexconv
}

// Save writes cfg to config.json in the user config dir.
func Save(cfg *Config) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0644)
}

// IsFirstRun returns true if no config.json exists yet.
func IsFirstRun() bool {
	dir, err := configDir()
	if err != nil {
		return true
	}
	_, err = os.Stat(filepath.Join(dir, "config.json"))
	return errors.Is(err, os.ErrNotExist)
}

// LoadProfiles returns compression profiles, global exclude patterns, minimum file
// size in bytes, and whether profiles.json was just created for the first time.
// profiles.json intentionally uses fallback-on-absence for all its fields — partial
// hand-authoring and sharing of the file is a supported workflow.
func LoadProfiles() ([]Profile, []string, int, bool, error) {
	dir, err := configDir()
	if err != nil {
		return nil, nil, 0, false, err
	}
	userPath := filepath.Join(dir, "profiles.json")
	data, err := os.ReadFile(userPath)
	created := false
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, nil, 0, false, err
		}
		if err := os.WriteFile(userPath, defaultProfilesJSON, 0644); err != nil {
			return nil, nil, 0, false, err
		}
		data = defaultProfilesJSON
		created = true
	} else if err != nil {
		return nil, nil, 0, false, err
	}
	var pf profileFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return nil, nil, 0, false, err
	}
	if pf.MinFileSizeBytes == 0 {
		pf.MinFileSizeBytes = 1024
	}
	return pf.Profiles, pf.ExcludePatterns, pf.MinFileSizeBytes, created, nil
}

func defaultConfig() *Config {
	return &Config{
		ModsDir:            detectModsDir(),
		WorkerCount:        1,
		BackupLevel:        6,
		ScanExclusions:     []string{".*", "downloads", "Downloads", "G.A.M.M.A. UI"},
		ModOutputMode:      true,
		ModOutputName:      "ATAK",
		ModlistPath:        "",
		CompressionBackend: BackendTexconv,
	}
}

func detectModsDir() string {
	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = []string{
			`C:\Games\Anomaly\mods`,
			`D:\Games\Anomaly\mods`,
			`D:\Anomaly\mods`,
			`C:\Anomaly\mods`,
			`C:\Games\GAMMA\mods`,
			`D:\Games\GAMMA\mods`,
			`D:\GAMMA\mods`,
			`C:\GAMMA\mods`,
		}
		if env := os.Getenv("MO2_GAME_PATH"); env != "" {
			candidates = append([]string{filepath.Join(env, "mods")}, candidates...)
		}
	} else {
		home, _ := os.UserHomeDir()
		candidates = []string{
			filepath.Join(home, "Games", "Anomaly", "mods"),
			filepath.Join(home, "Anomaly", "mods"),
			filepath.Join(home, "Games", "GAMMA", "mods"),
			filepath.Join(home, "GAMMA", "mods"),
		}
		if env := os.Getenv("MO2_GAME_PATH"); env != "" {
			candidates = append([]string{filepath.Join(env, "mods")}, candidates...)
		}
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
