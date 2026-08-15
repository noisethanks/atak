package screens

import "os"

// dirExists reports whether path names an existing, accessible directory.
// Returns false for empty string, nonexistent path, unreadable path, or a
// path that points to a file rather than a directory.
func dirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
