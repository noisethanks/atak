package archive

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/noisethanks/atak/internal/tools"
)

// BackupInfo describes a discovered backup archive.
type BackupInfo struct {
	Path    string
	Name    string
	Size    int64
	ModTime time.Time
}

// ProgressMsg carries progress updates from a long-running 7zz operation.
type ProgressMsg struct {
	Percent     int
	CurrentFile string
}

// ListBackups returns all .7z files in backupDir.
func ListBackups(backupDir string) ([]BackupInfo, error) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []BackupInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".7z") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, BackupInfo{
			Path:    filepath.Join(backupDir, e.Name()),
			Name:    e.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	return out, nil
}

// Backup creates a solid LZMA2 archive of modsDir at outputPath.
// Progress is sent on the returned channel (closed on completion).
// The error channel receives a single value (nil or error) when done.
func Backup(ctx context.Context, sevenZipPath, modsDir, outputPath string, backupLevel int) (<-chan ProgressMsg, <-chan error) {
	progress := make(chan ProgressMsg, 32)
	done := make(chan error, 1)

	go func() {
		defer close(progress)
		defer close(done)

		args := []string{
			"a", "-t7z",
			"-m0=lzma2", fmt.Sprintf("-mx=%d", backupLevel), "-mfb=16", "-md=32k", "-ms=on",
			"-bsp1",
			outputPath,
			modsDir,
			"-xr!downloads",
			"-xr!Downloads",
		}
		cmd := exec.Command(sevenZipPath, args...)
		tools.SetProcAttr(cmd)
		stderr, err := cmd.StderrPipe()
		if err != nil {
			done <- err
			return
		}
		job, _ := tools.NewJob()
		if err := cmd.Start(); err != nil {
			tools.CloseJob(job)
			done <- err
			return
		}
		tools.AssignJob(job, cmd)
		defer tools.CloseJob(job)
		_ = tools.WriteLock(cmd.Process.Pid)

		processExited := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				tools.KillProcess(cmd)
			case <-processExited:
			}
		}()

		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if pct, file, ok := parseProgress(line); ok {
				progress <- ProgressMsg{Percent: pct, CurrentFile: file}
			}
		}

		err = cmd.Wait()
		close(processExited)
		_ = tools.ClearLock()

		if ctx.Err() != nil {
			os.Remove(outputPath)
			done <- ctx.Err()
			return
		}
		done <- err
	}()

	return progress, done
}

// Restore extracts a single mod from archive to modDir.
// Progress is sent on the returned channel; error on done channel when complete.
func Restore(ctx context.Context, sevenZipPath, archivePath, modName, modDir string) (<-chan ProgressMsg, <-chan error) {
	progress := make(chan ProgressMsg, 32)
	done := make(chan error, 1)

	go func() {
		defer close(progress)
		defer close(done)

		pattern := "mods/" + modName + "/*"
		args := []string{
			"x", archivePath,
			"-o" + filepath.Dir(modDir),
			pattern,
			"-y",
			"-bsp1",
		}
		cmd := exec.CommandContext(ctx, sevenZipPath, args...)
		stderr, err := cmd.StderrPipe()
		if err != nil {
			done <- err
			return
		}
		job, _ := tools.NewJob()
		if err := cmd.Start(); err != nil {
			tools.CloseJob(job)
			done <- err
			return
		}
		tools.AssignJob(job, cmd)
		defer tools.CloseJob(job)

		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if pct, file, ok := parseProgress(line); ok {
				progress <- ProgressMsg{Percent: pct, CurrentFile: file}
			}
		}

		done <- cmd.Wait()
	}()

	return progress, done
}

// RestoreAll extracts the entire archive to modsParentDir (the directory containing the mods folder).
func RestoreAll(ctx context.Context, sevenZipPath, archivePath, modsParentDir string) (<-chan ProgressMsg, <-chan error) {
	progress := make(chan ProgressMsg, 32)
	done := make(chan error, 1)

	go func() {
		defer close(progress)
		defer close(done)

		args := []string{
			"x", archivePath,
			"-o" + modsParentDir,
			"-r",
			"-y",
			"-bsp1",
		}
		cmd := exec.Command(sevenZipPath, args...)
		tools.SetProcAttr(cmd)
		stderr, err := cmd.StderrPipe()
		if err != nil {
			done <- err
			return
		}
		job, _ := tools.NewJob()
		if err := cmd.Start(); err != nil {
			tools.CloseJob(job)
			done <- err
			return
		}
		tools.AssignJob(job, cmd)
		defer tools.CloseJob(job)
		_ = tools.WriteLock(cmd.Process.Pid)

		processExited := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				tools.KillProcess(cmd)
			case <-processExited:
			}
		}()

		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if pct, file, ok := parseProgress(line); ok {
				progress <- ProgressMsg{Percent: pct, CurrentFile: file}
			}
		}

		err = cmd.Wait()
		close(processExited)
		_ = tools.ClearLock()
		done <- err
	}()

	return progress, done
}

// ListMods parses `7zz l -slt` output and returns unique mod names from the archive.
// Uses technical listing format (one property per line) to handle spaces in paths
// and mods without explicit directory entries.
func ListMods(sevenZipPath, archivePath string) ([]string, error) {
	cmd := exec.Command(sevenZipPath, "l", "-slt", archivePath)
	tools.SetProcAttr(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("7zz list: %w", err)
	}

	const prefix = "Path = "
	seen := make(map[string]bool)
	var mods []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		path := line[len(prefix):]
		if !strings.HasPrefix(strings.ToLower(path), "mods/") {
			continue
		}
		parts := strings.SplitN(path, "/", 3)
		if len(parts) < 2 || parts[1] == "" {
			continue
		}
		name := parts[1]
		if !seen[name] {
			seen[name] = true
			mods = append(mods, name)
		}
	}
	return mods, nil
}

// Verify runs `7zz t` on archivePath and returns any error.
func Verify(sevenZipPath, archivePath string) error {
	cmd := exec.Command(sevenZipPath, "t", archivePath)
	tools.SetProcAttr(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("verify failed: %w\n%s", err, string(out))
	}
	return nil
}

// Delete removes the archive file after confirmation (caller must confirm).
func Delete(archivePath string) error {
	return os.Remove(archivePath)
}

// parseProgress parses a 7zz -bsp1 progress line.
// Format: "  X% N - filename" where X is percent and N is files.
func parseProgress(line string) (pct int, file string, ok bool) {
	line = strings.TrimSpace(line)
	if len(line) == 0 {
		return 0, "", false
	}
	// 7zz progress format: "  N% M - path/to/file"
	pctIdx := strings.Index(line, "%")
	if pctIdx <= 0 {
		return 0, "", false
	}
	pctStr := strings.TrimSpace(line[:pctIdx])
	pct64, err := strconv.ParseInt(pctStr, 10, 32)
	if err != nil {
		return 0, "", false
	}

	rest := strings.TrimSpace(line[pctIdx+1:])
	// Skip "N - " prefix if present.
	if dashIdx := strings.Index(rest, " - "); dashIdx >= 0 {
		file = strings.TrimSpace(rest[dashIdx+3:])
	}

	return int(pct64), file, true
}
