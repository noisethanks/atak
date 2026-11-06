package config

import (
	"path/filepath"
	"strings"
)

// CleanPath normalizes a user-entered path. Windows "Copy as path" and dragging
// a file into Windows Terminal both wrap a path containing spaces in literal
// double quotes, which would otherwise be stored as part of the path.
//
// It trims surrounding whitespace, strips one pair of matching surrounding
// quotes (" or '), then applies filepath.Clean. A quote anywhere else is left
// alone, since it can be a legitimate part of a name. An empty result stays
// empty rather than becoming ".", because empty means "not configured".
func CleanPath(p string) string {
	p = strings.TrimSpace(p)
	if n := len(p); n >= 2 && (p[0] == '"' || p[0] == '\'') && p[n-1] == p[0] {
		p = p[1 : n-1]
	}
	if p == "" {
		return ""
	}
	return filepath.Clean(p)
}
