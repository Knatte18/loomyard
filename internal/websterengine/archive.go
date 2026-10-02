// archive.go implements webster's own archive-never-refuse primitives: FirstFreeArchivePath and
// ArchiveStateFile, plus a webster-owned ArchiveReportsDir.
// These are deliberately module-local rather than shared, since archive layout is part of
// webster's own contract shape.
// firstFreeArchivePath is the shared same-second collision rule every archive helper in this
// package reuses (including outcome.go's archiveStaleOutcome);
// archiveStateFile and archiveReportsDir are the --fresh crash/resume escape's two halves, wired
// into state and runlevel in batch 7.

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
)

// archiveTimestampFormat is the UTC compact timestamp format webster archive helpers share.
const archiveTimestampFormat = "20060102T150405Z"

// firstFreeArchivePath returns the first free path in the sequence candidate(""), candidate("-1"), ...
func firstFreeArchivePath(candidate func(suffix string) string) (string, error) {
	for n := 0; ; n++ {
		suffix := ""
		if n > 0 {
			suffix = fmt.Sprintf("-%d", n)
		}
		path := candidate(suffix)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return path, nil
			}
			return "", err
		}
	}
}

// archiveStateFile renames websterDir's state.json with a UTC timestamp, if present.
func archiveStateFile(websterDir string, now func() time.Time) (string, error) {
	path := filepath.Join(websterDir, stateFileName)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("websterengine: stat state file %s: %w", path, err)
	}

	stamp := now().UTC().Format(archiveTimestampFormat)
	target, err := firstFreeArchivePath(func(suffix string) string {
		return filepath.Join(websterDir, fmt.Sprintf("state-%s%s.json", stamp, suffix))
	})
	if err != nil {
		return "", fmt.Errorf("websterengine: find archive target for state file %s: %w", path, err)
	}

	if err := os.Rename(path, target); err != nil {
		return "", fmt.Errorf("websterengine: archive stale state file %s: %w", path, err)
	}
	return target, nil
}

// archiveReportsDir renames reportsDir with a UTC timestamp and recreates an empty one.
func archiveReportsDir(reportsDir string, now func() time.Time) error {
	if _, err := os.Stat(reportsDir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("websterengine: stat reports dir %s: %w", reportsDir, err)
		}
	} else {
		stamp := now().UTC().Format(archiveTimestampFormat)
		parent := filepath.Dir(reportsDir)
		base := filepath.Base(reportsDir)
		target, err := firstFreeArchivePath(func(suffix string) string {
			return filepath.Join(parent, fmt.Sprintf("%s-%s%s", base, stamp, suffix))
		})
		if err != nil {
			return fmt.Errorf("websterengine: find archive target for reports dir %s: %w", reportsDir, err)
		}
		if err := os.Rename(reportsDir, target); err != nil {
			return fmt.Errorf("websterengine: archive stale reports dir %s: %w", reportsDir, err)
		}
	}

	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return fmt.Errorf("websterengine: recreate reports dir %s: %w", reportsDir, err)
	}
	return nil
}

// ArchiveRunRecord moves every entry of geom.WebsterDir into dest, so the next run finds no
// state and starts fresh over the live plan only.
// dest is told and never derived; this function knows nothing of what it is archiving for.
// It refuses with ErrRunBusy while a run holds the run lock, then holds the state-mutation lease
// across the moves.
// It is idempotent for crash resume: an absent WebsterDir, or an entry already moved, is skipped.
// An entry present at both source and destination is an error and nothing is moved, since
// either copy may be the stale one.
// The rendered fork prompts are cleared as the --fresh escape does, since they are re-renderable
// and name the retired run's batches.
func ArchiveRunRecord(geom Geometry, dest string) error {
	if err := os.MkdirAll(geom.ScratchDir, 0o755); err != nil {
		return fmt.Errorf("websterengine: create webster scratch dir %s: %w", geom.ScratchDir, err)
	}
	runLock, locked, err := lock.TryAcquireWriteLock(filepath.Join(geom.ScratchDir, runLockName))
	if err != nil {
		return fmt.Errorf("websterengine: acquire run lock in %s: %w", geom.ScratchDir, err)
	}
	if !locked {
		return fmt.Errorf("%w: %q (run.lock held); way forward: wait for the run to finish, then retry", ErrRunBusy, geom.ScratchDir)
	}
	defer runLock.Release()

	lease, err := AcquireStateMutation(geom.ScratchDir)
	if err != nil {
		return err
	}
	defer lease.Release()

	entries, err := os.ReadDir(geom.WebsterDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("websterengine: read webster dir %s: %w", geom.WebsterDir, err)
	}

	for _, e := range entries {
		to := filepath.Join(dest, e.Name())
		if _, err := os.Lstat(to); err == nil {
			from := filepath.Join(geom.WebsterDir, e.Name())
			return fmt.Errorf("websterengine: archive run record: %s exists at both %s and %s; way forward: remove whichever copy is stale, then re-step", e.Name(), from, to)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("websterengine: stat archive target %s: %w", to, err)
		}
	}

	if len(entries) > 0 {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return fmt.Errorf("websterengine: create archive dir %s: %w", dest, err)
		}
	}
	for _, e := range entries {
		from := filepath.Join(geom.WebsterDir, e.Name())
		if err := os.Rename(from, filepath.Join(dest, e.Name())); err != nil {
			return fmt.Errorf("websterengine: archive %s into %s: %w", from, dest, err)
		}
	}

	return clearRenderedPrompts(geom.PromptsDir)
}
