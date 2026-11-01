package compress

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/noisethanks/atak/internal/scan"
)

// fakeTexconv writes a shell script that stands in for texconv and mimics its
// worst behaviors for the source file: it writes straight to <out>/<input
// basename> (the path that IS the source in place) before it fails or hangs, and
// on success it writes a lowercase <stem>.dds like the real tool.
//
//	FAKE_FAIL=1       clobber <out>/<basename>, exit 1
//	FAKE_HANG_ON=<b>  for that input: clobber <out>/<basename>, touch FAKE_MARK, sleep
func fakeTexconv(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake texconv")
	}
	script := `#!/bin/sh
while [ $# -gt 0 ]; do case "$1" in -o) out="$2"; shift;; --) in="$2"; shift;; esac; shift; done
b=$(basename "$in"); stem=${b%.*}
if [ "$FAKE_FAIL" = 1 ]; then printf junk > "$out/$b"; echo boom >&2; exit 1; fi
if [ "$b" = "$FAKE_HANG_ON" ]; then printf partial > "$out/$b"; : > "$FAKE_MARK"; sleep 30; fi
printf new > "$out/$stem.dds"
`
	p := filepath.Join(t.TempDir(), "texconv")
	if err := os.WriteFile(p, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeSources(t *testing.T, dir string, names []string) map[string]string {
	t.Helper()
	want := map[string]string{}
	for _, n := range names {
		content := "src-" + n
		if err := os.WriteFile(filepath.Join(dir, n), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		want[n] = content
	}
	return want
}

// assertDir fails unless dir holds exactly the files in want, with that content.
func assertDir(t *testing.T, dir string, want map[string]string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	var names []string
	for n := range want {
		names = append(names, n)
	}
	sort.Strings(got)
	sort.Strings(names)
	if len(got) != len(names) {
		t.Fatalf("dir has %v, want %v", got, names)
	}
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatalf("%s missing: %v (dir has %v)", n, err, got)
		}
		if string(b) != want[n] {
			t.Errorf("%s = %q, want %q", n, b, want[n])
		}
	}
}

func inPlaceJob(dir, name string) Job {
	return Job{
		Asset:     scan.Asset{Path: filepath.Join(dir, name), Width: 4, Height: 4},
		Format:    "BC1_UNORM",
		OutputDir: dir,
	}
}

func TestInPlaceCancelKeepsUnprocessedSources(t *testing.T) {
	dir := t.TempDir()
	names := []string{"a.dds", "B.DDS", "c.dds", "D.DDS"}
	want := writeSources(t, dir, names)
	t.Setenv("FAKE_HANG_ON", "c.dds")
	mark := filepath.Join(t.TempDir(), "mark")
	t.Setenv("FAKE_MARK", mark)

	var jobs []Job
	for _, n := range names {
		jobs = append(jobs, inPlaceJob(dir, n))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			if _, err := os.Stat(mark); err == nil {
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	for range RunPool(ctx, NewTexconvBackend(fakeTexconv(t)), nil, jobs, 1) {
	}

	// Processed sources are fully replaced under their exact names; the job that
	// was running and the one never started are byte-identical to before.
	want["a.dds"], want["B.DDS"] = "new", "new"
	assertDir(t, dir, want)
}

func TestInPlaceBackendFailureLeavesSource(t *testing.T) {
	dir := t.TempDir()
	want := writeSources(t, dir, []string{"a.dds", "B.DDS"})
	t.Setenv("FAKE_FAIL", "1")
	b := NewTexconvBackend(fakeTexconv(t))
	for n := range want {
		if r := dispatch(context.Background(), b, nil, inPlaceJob(dir, n)); r.Success {
			t.Fatalf("%s: expected failure", n)
		}
	}
	assertDir(t, dir, want)
}

func TestInPlaceKeepsUppercaseExtension(t *testing.T) {
	dir := t.TempDir()
	writeSources(t, dir, []string{"Foo.DDS"})
	r := dispatch(context.Background(), NewTexconvBackend(fakeTexconv(t)), nil, inPlaceJob(dir, "Foo.DDS"))
	if !r.Success {
		t.Fatalf("failed: %v", r.Err)
	}
	assertDir(t, dir, map[string]string{"Foo.DDS": "new"})
}

func TestModOutputCancelLeavesNoDestinationFile(t *testing.T) {
	src := t.TempDir()
	want := writeSources(t, src, []string{"B.DDS"})
	t.Setenv("FAKE_HANG_ON", "B.DDS")
	mark := filepath.Join(t.TempDir(), "mark")
	t.Setenv("FAKE_MARK", mark)
	modOut := t.TempDir()

	job := inPlaceJob(src, "B.DDS")
	job.OutputDir = ""
	job.ModOutputDir = modOut
	job.RelPath = "tex/B.DDS"
	backend := NewTexconvBackend(fakeTexconv(t))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			if _, err := os.Stat(mark); err == nil {
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	for range RunPool(ctx, backend, nil, []Job{job}, 1) {
	}

	// Nothing under the output folder in any case variant, staging included.
	assertDir(t, src, want)
	var left []string
	filepath.WalkDir(modOut, func(p string, d os.DirEntry, err error) error {
		if err == nil && p != modOut && !d.IsDir() {
			left = append(left, p)
		}
		if err == nil && d.IsDir() && d.Name() == stagingDirName {
			left = append(left, p)
		}
		return nil
	})
	if len(left) != 0 {
		t.Fatalf("cancel left files in the output folder: %v", left)
	}

	// A rerun must compress the file, not skip it as already done.
	t.Setenv("FAKE_HANG_ON", "")
	var got []CompressionResult
	for r := range RunPool(context.Background(), backend, nil, []Job{job}, 1) {
		got = append(got, r)
	}
	if len(got) != 1 || !got[0].Success || got[0].OutputSkipped {
		t.Fatalf("rerun result = %+v, want a compressed success", got)
	}
	entries, _ := os.ReadDir(filepath.Join(modOut, "tex"))
	if len(entries) != 1 || entries[0].Name() != "B.DDS" {
		t.Fatalf("rerun output = %v, want one B.DDS", entries)
	}
	if _, err := os.Stat(filepath.Join(modOut, stagingDirName)); !os.IsNotExist(err) {
		t.Errorf("staging dir left behind after rerun: %v", err)
	}
}

// texconv writes a lowercase .dds, but the destination is named from RelPath, so
// the output is B.DDS and the incremental skip finds it on the next run.
func TestModOutputNamedFromRelPathAndSkippedOnRerun(t *testing.T) {
	src := t.TempDir()
	writeSources(t, src, []string{"B.DDS"})
	modOut := t.TempDir()
	job := inPlaceJob(src, "B.DDS")
	job.OutputDir = ""
	job.ModOutputDir = modOut
	job.RelPath = "tex/B.DDS"
	backend := NewTexconvBackend(fakeTexconv(t))

	for run := 1; run <= 2; run++ {
		var got []CompressionResult
		for r := range RunPool(context.Background(), backend, nil, []Job{job}, 1) {
			got = append(got, r)
		}
		if len(got) != 1 || !got[0].Success {
			t.Fatalf("run %d: %+v", run, got)
		}
		if skipped := got[0].OutputSkipped; skipped != (run == 2) {
			t.Fatalf("run %d: OutputSkipped = %v", run, skipped)
		}
		assertDir(t, filepath.Join(modOut, "tex"), map[string]string{"B.DDS": "new"})
	}
}
