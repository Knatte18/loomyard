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
