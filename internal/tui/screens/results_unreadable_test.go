package screens

import (
	"strings"
	"testing"

	"github.com/noisethanks/atak/internal/config"
	"github.com/noisethanks/atak/internal/scan"
)

// TestResultsShowsUnreadableFiles covers the last leg of the scan's honesty fix.
// The walkers now report files they could not read, and that only helps if the
// count and a few names reach the screen — the whole bug was that these files
// existed and the user never heard about them.
func TestResultsShowsUnreadableFiles(t *testing.T) {
	data := ScanResultData{
		Assets: []scan.Asset{{
			Path:         "/mods/ModA/gamedata/textures/wall_d.dds",
			ModName:      "ModA",
			ProfileMatch: "Unmatched",
			CurrentFmt:   "R8G8B8A8_UNORM",
		}},
		Stats: scan.Stats{
			Skipped: 12,
			Unreadable: []scan.UnreadableFile{
				{Path: "/mods/ModA/gamedata/textures/item/can_inside.dds", ModName: "ModA", Reason: "not a DDS file"},
			},
		},
	}

	view := NewResults(data, &config.Config{}).View()
	if !strings.Contains(view, "Unreadable (1 files)") {
		t.Errorf("results view has no unreadable count:\n%s", view)
	}
	if !strings.Contains(view, "can_inside.dds") {
		t.Errorf("results view does not name the unreadable file:\n%s", view)
	}
	if !strings.Contains(view, "not a DDS file") {
		t.Errorf("results view does not give the reason:\n%s", view)
	}
}

// TestResultsHidesUnreadableSectionWhenEmpty keeps the common case quiet: a clean
// modlist should not carry a line reporting zero problems.
func TestResultsHidesUnreadableSectionWhenEmpty(t *testing.T) {
	data := ScanResultData{
		Assets: []scan.Asset{{Path: "/mods/ModA/gamedata/textures/wall_d.dds", ProfileMatch: "Unmatched"}},
		Stats:  scan.Stats{Skipped: 3},
	}
	if view := NewResults(data, &config.Config{}).View(); strings.Contains(view, "Unreadable") {
		t.Errorf("results view reports an empty unreadable section:\n%s", view)
	}
}

// TestScanReachesResultsWithNoCompressibleAssets covers the abort that threw the
// new information away. A scan that emits no assets is not the same as a scan
// that found nothing: every texture may already be compressed, or every one may
// have failed to parse. Both used to end at "No .dds files found".
func TestScanReachesResultsWithNoCompressibleAssets(t *testing.T) {
	cases := []struct {
		name    string
		stats   scan.Stats
		wantErr bool
	}{
		{"nothing at all", scan.Stats{}, true},
		{"everything already compressed", scan.Stats{Skipped: 4000}, false},
		{"everything unreadable", scan.Stats{Unreadable: []scan.UnreadableFile{{Path: "/mods/ModA/x.dds", Reason: "not a DDS file"}}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewScan(&config.Config{ModsDir: "/mods"}, nil)
			m, cmd := m.Update(scanCompleteMsg{stats: tc.stats})
			if tc.wantErr {
				if m.err == "" {
					t.Error("want the empty-directory message")
				}
				return
			}
			if m.err != "" {
				t.Errorf("got error %q, want navigation to results", m.err)
			}
			if cmd == nil {
				t.Fatal("no command returned — the scan never navigates to results")
			}
			nav, ok := cmd().(NavigateMsg)
			if !ok || nav.To != NavResults {
				t.Fatalf("got %#v, want a NavigateMsg to NavResults", cmd())
			}
			data := nav.Data.(ScanResultData)
			if data.Stats.Skipped != tc.stats.Skipped || len(data.Stats.Unreadable) != len(tc.stats.Unreadable) {
				t.Errorf("stats did not survive the hop: %+v", data.Stats)
			}
		})
	}
}

// TestResultsCapsUnreadableList checks the cap: past three names the screen gives
// a count instead, so a badly packaged mod cannot push the key hints out of view.
func TestResultsCapsUnreadableList(t *testing.T) {
	var files []scan.UnreadableFile
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		files = append(files, scan.UnreadableFile{Path: "/mods/ModA/" + n + ".dds", Reason: "not a DDS file"})
	}
	data := ScanResultData{
		Assets: []scan.Asset{{Path: "/mods/ModA/gamedata/textures/wall_d.dds", ProfileMatch: "Unmatched"}},
		Stats:  scan.Stats{Unreadable: files},
	}

	view := NewResults(data, &config.Config{}).View()
	if !strings.Contains(view, "Unreadable (5 files)") {
		t.Errorf("wrong unreadable count:\n%s", view)
	}
	if !strings.Contains(view, "…and 2 more") {
		t.Errorf("list is not capped at %d names:\n%s", unreadableListLimit, view)
	}
	if strings.Contains(view, "e.dds") {
		t.Errorf("view named a file past the cap:\n%s", view)
	}
}
