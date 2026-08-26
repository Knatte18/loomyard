// archive.go implements archiveStaleOutputs, the archive-never-refuse helper SingleLLMProducer runs
// over an already-told, already-absolute list of output files before invoking its seam.
// firstFreeArchivePath is a package-local same-second collision helper, re-implemented here rather
// than shared from websterengine because that package's own copy is unexported and the
// no-new-engine-surface rule forbids exporting it.

package shedadapters

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// archiveTimestampFormat is the UTC compact timestamp format archiveStaleOutputs uses, mirroring
// websterengine's own ArchiveStaleSummary stamp.
const archiveTimestampFormat = "20060102T150405Z"

// archiveStaleOutputs renames every entry of files that exists on disk to a timestamped sibling
// beside it, leaving absent entries alone without error.
// now resolves the archive stamp; the caller is responsible for defaulting a nil now to time.Now.
func archiveStaleOutputs(files []string, now func() time.Time) error {
	for _, path := range files {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("shedadapters: stat output file %s: %w", path, err)
		}

		dir := filepath.Dir(path)
		ext := filepath.Ext(path)
		base := strings.TrimSuffix(filepath.Base(path), ext)
		stamp := now().UTC().Format(archiveTimestampFormat)

		target, err := firstFreeArchivePath(func(suffix string) string {
			return filepath.Join(dir, fmt.Sprintf("%s-%s%s%s", base, stamp, suffix, ext))
		})
		if err != nil {
			return fmt.Errorf("shedadapters: find archive target for output file %s: %w", path, err)
		}

		if err := os.Rename(path, target); err != nil {
			return fmt.Errorf("shedadapters: archive stale output file %s: %w", path, err)
		}
	}
	return nil
}

// archiveRunDir renames runDir to a timestamped sibling beside it and recreates runDir as an empty
// directory. now resolves the archive stamp, mirroring archiveStaleOutputs's own clock use; the
// caller is responsible for defaulting a nil now to time.Now.
// Unlike archiveStaleOutputs, a directory has no extension to preserve, so the candidate closure
// appends nothing after the collision suffix.
// The recreate is not optional: ResolveRound hard-errors when the run directory is absent, so a
// caller that skipped the recreate would turn its very next call into a hard error rather than a
// seed.
func archiveRunDir(runDir string, now func() time.Time) error {
	dir := filepath.Dir(runDir)
	base := filepath.Base(runDir)
	stamp := now().UTC().Format(archiveTimestampFormat)

	target, err := firstFreeArchivePath(func(suffix string) string {
		return filepath.Join(dir, fmt.Sprintf("%s-%s%s", base, stamp, suffix))
	})
	if err != nil {
		return fmt.Errorf("shedadapters: find archive target for run dir %s: %w", runDir, err)
	}

	if err := os.Rename(runDir, target); err != nil {
		return fmt.Errorf("shedadapters: archive run dir %s: %w", runDir, err)
	}

	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("shedadapters: recreate run dir %s: %w", runDir, err)
	}
	return nil
}

// firstFreeArchivePath returns the first path in the sequence candidate(""), candidate("-1"),
// candidate("-2"), ... that does not exist.
// Any os.Stat error other than not-exist is returned as-is.
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
