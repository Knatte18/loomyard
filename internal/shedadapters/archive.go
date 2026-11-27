// archive.go holds the helpers that move a run's previous output out of the way before a run starts fresh.
// archiveStaleOutput and archiveStaleOutputs rename each present output file;
// archiveRunDir renames a run directory and recreates it empty.
// Both give a taken archive name a numeric suffix.

package shedadapters

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// archiveTimestampFormat is the UTC compact timestamp format that archive names carry.
const archiveTimestampFormat = "20060102T150405Z"

// archiveStaleOutputs renames every entry of files that exists on disk to a timestamped sibling
// beside it, leaving absent entries alone without error.
// now resolves the archive stamp; the caller is responsible for defaulting a nil now to time.Now.
func archiveStaleOutputs(files []string, now func() time.Time) error {
	for _, path := range files {
		if _, err := archiveStaleOutput(path, now); err != nil {
			return err
		}
	}
	return nil
}

// archiveStaleOutput renames path, when it exists on disk, to a timestamped sibling beside it.
// It returns the sibling's path.
// It returns "" and no error when path is absent.
// now resolves the archive stamp; the caller is responsible for defaulting a nil now to time.Now.
func archiveStaleOutput(path string, now func() time.Time) (string, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("shedadapters: stat output file %s: %w", path, err)
	}

	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	stamp := now().UTC().Format(archiveTimestampFormat)

	target, err := firstFreeArchivePath(func(suffix string) string {
		return filepath.Join(dir, fmt.Sprintf("%s-%s%s%s", base, stamp, suffix, ext))
	})
	if err != nil {
		return "", fmt.Errorf("shedadapters: find archive target for output file %s: %w", path, err)
	}

	if err := os.Rename(path, target); err != nil {
		return "", fmt.Errorf("shedadapters: archive stale output file %s: %w", path, err)
	}
	return target, nil
}

// archiveRunDir renames runDir to a timestamped sibling beside it and recreates runDir as an empty directory.
// It carries the settle record, its patch and the generation record into the recreated directory,
// so the Bouncer's own generation archive keeps them.
// now resolves the archive stamp; the caller is responsible for defaulting a nil now to time.Now.
// The recreate is required: a missing run directory makes the next round a hard error, not a fresh seed.
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
	if err := CarryRecords(target, runDir); err != nil {
		return fmt.Errorf("shedadapters: carry the records of run dir %s: %w", runDir, err)
	}
	return nil
}

// archivedRunDirPattern matches a sibling name: a segment, the UTC stamp, and an optional numeric collision suffix.
var archivedRunDirPattern = regexp.MustCompile(`^(.+)-\d{8}T\d{6}Z(?:-\d+)?$`)

// ArchivedRunDirSegment returns the segment a timestamped run-directory sibling belongs to, and true.
// A name that does not end in the archive stamp, with or without a `-<n>` collision suffix, returns "" and false.
func ArchivedRunDirSegment(dirName string) (string, bool) {
	match := archivedRunDirPattern.FindStringSubmatch(dirName)
	if match == nil {
		return "", false
	}
	return match[1], true
}

// SegmentRan reports whether the review segment whose run directory is reviewsDir/segment has run.
// A round review, a generation record or a settle record in that directory shows that it has.
// So does an archived sibling of the directory.
// An absent directory is no run, and any other read failure is an error.
func SegmentRan(reviewsDir, segment string) (bool, error) {
	entries, err := os.ReadDir(reviewsDir)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("shedadapters: read reviews dir %s: %w", reviewsDir, err)
	}
	for _, entry := range entries {
		if archived, ok := ArchivedRunDirSegment(entry.Name()); ok && archived == segment && entry.IsDir() {
			return true, nil
		}
	}

	runDir := filepath.Join(reviewsDir, segment)
	if _, err := os.Stat(runDir); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("shedadapters: stat run dir %s: %w", runDir, err)
	}
	for _, name := range []string{generationRecordFile, settleRecordFile} {
		path := filepath.Join(runDir, name)
		if _, err := os.Stat(path); err == nil {
			return true, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return false, fmt.Errorf("shedadapters: stat %s: %w", path, err)
		}
	}
	round, err := LatestRound(runDir)
	return round > 0, err
}

// firstFreeArchivePath returns the first path that candidate builds and that does not exist.
// It tries the suffixes "", "-1", "-2", and so on, in that order.
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
