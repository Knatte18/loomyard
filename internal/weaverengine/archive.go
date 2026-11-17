// archive.go moves Weaver's run record out of the way, so the next run starts fresh.
// archiveRunInPlace renames the record's entries under a UTC stamp inside the run directory.
// ArchiveRunRecord moves every entry of the run directory into a directory the caller names.

package weaverengine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// archiveTimestampFormat is the UTC compact timestamp format of an archived record's names.
const archiveTimestampFormat = "20060102T150405Z"

// recordEntryNames are the entries of the run directory a run's record holds.
var recordEntryNames = []string{stateFileName, reportsDirName, journalFileName, summaryparser.FileName, outcomeFileName}

// archiveRunInPlace renames the run record's entries inside the run directory under a UTC stamp, so no reader reads them.
// The entries are the state file, the reports directory, the journal, the summary and the outcome record.
// An absent entry is skipped, and a stamped name already taken gets a numeric suffix.
func archiveRunInPlace(geom Geometry, now func() time.Time) error {
	stamp := now().UTC().Format(archiveTimestampFormat)
	for _, name := range recordEntryNames {
		from := filepath.Join(geom.RunDir, name)
		if _, err := os.Lstat(from); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("weaver: stat %s: %w", from, err)
		}
		ext := filepath.Ext(name)
		stem := name[:len(name)-len(ext)]
		to, err := firstFreeArchivePath(func(suffix string) string {
			return filepath.Join(geom.RunDir, fmt.Sprintf("%s-%s%s%s", stem, stamp, suffix, ext))
		})
		if err != nil {
			return fmt.Errorf("weaver: find an archive name for %s: %w", from, err)
		}
		if err := os.Rename(from, to); err != nil {
			return fmt.Errorf("weaver: archive %s: %w", from, err)
		}
	}
	return nil
}

// firstFreeArchivePath returns the first free path of the sequence candidate(""), candidate("-1"), and on.
func firstFreeArchivePath(candidate func(suffix string) string) (string, error) {
	for n := 0; ; n++ {
		suffix := ""
		if n > 0 {
			suffix = fmt.Sprintf("-%d", n)
		}
		path := candidate(suffix)
		if _, err := os.Lstat(path); err != nil {
			if os.IsNotExist(err) {
				return path, nil
			}
			return "", err
		}
	}
}

// ArchiveRunRecord moves every entry of Weaver's run directory into dest, so the next run finds no state.
// The caller passes dest, and ArchiveRunRecord does not derive it.
// It refuses with ErrRunBusy while a run holds the run lock, and holds the state-mutation lease across the moves.
// An absent run directory moves nothing.
// An entry present at both source and destination is an error, and nothing is moved.
// Either copy may be the stale one.
func ArchiveRunRecord(geom Geometry, dest string) error {
	if err := os.MkdirAll(geom.ScratchDir, 0o755); err != nil {
		return fmt.Errorf("weaver: create scratch dir %s: %w", geom.ScratchDir, err)
	}
	runLock, locked, err := lock.TryAcquireWriteLock(filepath.Join(geom.ScratchDir, runLockName))
	if err != nil {
		return fmt.Errorf("weaver: acquire the run lock in %s: %w", geom.ScratchDir, err)
	}
	if !locked {
		return fmt.Errorf("%w: %q (%s held); way forward: wait for the run to stop, then retry", ErrRunBusy, geom.ScratchDir, runLockName)
	}
	defer func() { _ = runLock.Release() }()

	lease, err := runplan.AcquireStateMutation(geom.ScratchDir)
	if err != nil {
		return err
	}
	defer func() { _ = lease.Release() }()

	entries, err := os.ReadDir(geom.RunDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("weaver: read run dir %s: %w", geom.RunDir, err)
	}
	for _, entry := range entries {
		to := filepath.Join(dest, entry.Name())
		if _, err := os.Lstat(to); err == nil {
			from := filepath.Join(geom.RunDir, entry.Name())
			return fmt.Errorf("weaver: archive run record: %s exists at both %s and %s; way forward: remove whichever copy is stale, then retry", entry.Name(), from, to)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("weaver: stat archive target %s: %w", to, err)
		}
	}
	if len(entries) == 0 {
		return nil
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("weaver: create archive dir %s: %w", dest, err)
	}
	for _, entry := range entries {
		from := filepath.Join(geom.RunDir, entry.Name())
		if err := os.Rename(from, filepath.Join(dest, entry.Name())); err != nil {
			return fmt.Errorf("weaver: archive %s into %s: %w", from, dest, err)
		}
	}
	return nil
}
