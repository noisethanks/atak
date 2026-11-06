//go:build windows

package tools

import (
	"strings"
	"syscall"
)

// CommandLine renders the command line a child process receives. On Windows a
// process gets one string, not an argv array: os/exec builds it by quoting each
// argument with syscall.EscapeArg, and the child re-parses it itself. Logging
// that exact string is what shows whether a path with spaces arrived intact or
// was split by the child's own parser.
func CommandLine(path string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, syscall.EscapeArg(path))
	for _, a := range args {
		parts = append(parts, syscall.EscapeArg(a))
	}
	return strings.Join(parts, " ")
}
