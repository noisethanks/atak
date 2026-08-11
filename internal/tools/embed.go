package tools

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

//go:embed bin/THIRD_PARTY_LICENSES.txt
var LicenseText []byte

// EmbeddedTools holds paths to extracted binaries for the current session.
// CompressonatorPath is empty on platforms where the compressonator-bc7e backend
// is unavailable — callers must treat "" as "unavailable" rather than
// special-casing runtime.GOOS. Every supported platform ships a build today,
// so the empty case is a guard, not a routine state.
type EmbeddedTools struct {
	TexconvPath        string
	SevenZipPath       string
	CompressonatorPath string
	tmpDir             string
}

// Cleanup removes the temp directory containing extracted binaries.
func (t *EmbeddedTools) Cleanup() {
	if t.tmpDir != "" {
		os.RemoveAll(t.tmpDir)
	}
}

func writeBin(dir, name string, data []byte) (string, error) {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0755); err != nil {
		return "", fmt.Errorf("write %s: %w", name, err)
	}
	if runtime.GOOS == "linux" {
		if err := os.Chmod(path, 0755); err != nil {
			return "", fmt.Errorf("chmod %s: %w", name, err)
		}
	}
	return path, nil
}

// Extract writes embedded binaries to a temp dir and returns the tool paths.
// The compressonator binary is only written when the embedded data is non-empty,
// which keeps a platform without a build from writing a zero-byte executable.
func Extract() (*EmbeddedTools, error) {
	dir, err := os.MkdirTemp("", "atak-*")
	if err != nil {
		return nil, fmt.Errorf("mkdirtemp: %w", err)
	}

	tcPath, err := writeBin(dir, texconvName, texconvBin)
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}

	szPath, err := writeBin(dir, sevenZipName, sevenZipBin)
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}

	var cmpPath string
	if len(compressonatorBin) > 0 && compressonatorName != "" {
		cmpPath, err = writeBin(dir, compressonatorName, compressonatorBin)
		if err != nil {
			os.RemoveAll(dir)
			return nil, err
		}
	}

	return &EmbeddedTools{
		TexconvPath:        tcPath,
		SevenZipPath:       szPath,
		CompressonatorPath: cmpPath,
		tmpDir:             dir,
	}, nil
}
