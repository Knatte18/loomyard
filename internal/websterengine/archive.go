// archive.go implements webster's own archive-never-refuse primitives: a
// webster-local copy of builderengine/runlevel.go's FirstFreeArchivePath and
// ArchiveStateFile, plus a webster-owned ArchiveReportsDir, since builder's
// own exported versions have an in-tree builder caller (frozen, per the
// Shared Decision builder-is-frozen-copy-not-move) and cannot be moved.
// firstFreeArchivePath is the shared same-second collision rule every
// archive helper in this package reuses (including outcome.go's
// archiveStaleOutcome); archiveStateFile and archiveReportsDir are the
// --fresh crash/resume escape's two halves, wired into state and runlevel
// in batch 7.

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// archiveTimestampFormat is the UTC compact timestamp format every webster
// archive helper in this package shares (this file's own helpers, plus
// outcome.go's archiveStaleOutcome), so every archived webster artifact
// sorts and reads identically regardless of which one archived it.
// Webster-owned rather than reusing builder's own unexported
// archiveTimestampFormat const, per the builder-is-frozen-copy-not-move
// decision.
const archiveTimestampFormat = "20060102T150405Z"

// firstFreeArchivePath returns the first path in the sequence
// candidate(""), candidate("-1"), candidate("-2"), ... that does not
// currently exist on disk — the same-second collision rule every archive
// helper in this package shares, so no two archives landing in the same
// clock-second ever clobber each other.
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

// archiveStateFile renames websterDir's state.json (stateFileName, defined
// in state.go), if present, to state-<UTC-compact-timestamp>.json in
// place — the --fresh fingerprint-mismatch escape's first half. Absent
// file: ("", nil), a no-op. now is a seam so tests can pin the timestamp
// deterministically instead of racing the real clock; production callers
// pass time.Now.
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

// archiveReportsDir renames reportsDir wholesale, if present, to
// <reportsDir>-<UTC-compact-timestamp> — the --fresh escape's second half —
// then recreates an empty reportsDir so the re-initialized run has
// somewhere to write the first batch's report into. Absent dir: a no-op
// (still recreates an empty one, since a fresh run needs it regardless of
// whether a prior one ever existed).
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
