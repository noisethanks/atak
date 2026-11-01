package compress

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/noisethanks/atak/internal/scan"
)

// Job represents a single compression task.
type Job struct {
	Asset          scan.Asset
	Format         string
	GenerateMips   bool
	MaxTextureSize int
	OutputDir      string // filepath.Dir(asset.Path) for in-place
	// Mod output mode: when ModOutputDir is non-empty, output goes to
	// filepath.Join(ModOutputDir, RelPath) instead of OutputDir.
	ModOutputDir string // e.g. /mods/ATAK
	RelPath      string // e.g. gamedata/textures/wpn/ak74.dds
}

// RunPool executes jobs concurrently using workerCount goroutines. Every job is
// dispatched to primary; fallback (may be nil) covers the one case primary
// can't handle — the compressonator maxTextureSize resize gap — and is picked
// per-job by dispatch() rather than per-run so a mixed workload doesn't force
// the whole run onto texconv. Results are sent to the returned channel, which
// is closed when all jobs complete.
func RunPool(ctx context.Context, primary, fallback Backend, jobs []Job, workerCount int) <-chan CompressionResult {
	results := make(chan CompressionResult, len(jobs))
	work := make(chan Job, len(jobs))

	for _, j := range jobs {
		work <- j
	}
	close(work)

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range work {
				select {
				case <-ctx.Done():
					return
				default:
				}
				if job.ModOutputDir != "" {
					outPath, err := destPath(job)
					if err != nil {
						results <- CompressionResult{Asset: job.Asset, Success: false, Err: err}
						continue
					}
					if _, err := os.Stat(outPath); err == nil {
						// Already compressed on a previous run — skip incrementally.
						results <- CompressionResult{Asset: job.Asset, Success: true, OutputSkipped: true}
						continue
					}
					if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
						results <- CompressionResult{Asset: job.Asset, Success: false, Err: err}
						continue
					}
					job.OutputDir = filepath.Dir(outPath)
				}
				results <- dispatch(ctx, primary, fallback, job)
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}
