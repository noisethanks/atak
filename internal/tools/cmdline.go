//go:build !windows

package tools

import (
	"strconv"
	"strings"
)

// CommandLine renders a child's argv for failure records. Outside Windows the
// child gets the array as is, so each argument is shown verbatim, Go-quoted when
// it is empty or has a space or quote, so argument boundaries stay visible.
func CommandLine(path string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	for _, a := range append([]string{path}, args...) {
		if a == "" || strings.ContainsAny(a, " \t\"'") {
			a = strconv.Quote(a)
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}
