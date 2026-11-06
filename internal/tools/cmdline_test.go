package tools

import "testing"

// A failure record must show where a spaced path's argument starts and ends,
// or a split path is indistinguishable from an intact one.
func TestCommandLineQuotesSpacedArgs(t *testing.T) {
	got := CommandLine("/bin/tool", []string{"-o", "/My Mods/out dir", "--", "/a/b.dds"})
	want := `/bin/tool -o "/My Mods/out dir" -- /a/b.dds`
	if got != want {
		t.Errorf("CommandLine = %s, want %s", got, want)
	}
}
