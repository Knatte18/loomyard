// summary.go implements webster's own prose summary artifact contract:
// SummaryPath resolves the fixed summary.md path inside a webster dir;
// ParseSummary enforces discussion.md's summary-artifact decision's minimal
// fail-loud validation (presence, non-empty, a "# <title>" first non-blank
// line with a non-empty title) — the artifact is the future loom-finalize
// PR-text source, never itself schema-validated beyond that; and
// ArchiveStaleSummary applies the same archive-never-refuse timestamp-rename
// discipline as builderengine.ArchiveStaleOutcome, reusing
// builderengine.FirstFreeArchivePath rather than re-implementing the
// same-second collision loop, per the reuse-by-import-never-copy decision.

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/builderengine"
)

// SummaryFileName is summary.md's fixed filename inside a webster dir.
const SummaryFileName = "summary.md"

// summaryArchiveTimestampFormat is the UTC compact timestamp format
// ArchiveStaleSummary archives a stale summary.md under — webster's own copy
// of builderengine's own archiveTimestampFormat (runlevel.go), which is
// unexported and so not importable; kept identical so an archived webster
// artifact sorts and reads the same way as builder's own archived artifacts.
const summaryArchiveTimestampFormat = "20060102T150405Z"

// SummaryPath returns the path to summary.md inside websterDir.
func SummaryPath(websterDir string) string {
	return filepath.Join(websterDir, SummaryFileName)
}

// Summary is Master's own final-action prose artifact, parsed from
// summary.md: Title is the "# <title>" heading's trailing text, and Body is
// every line after that heading, verbatim, never schema-validated — the
// consumer is PR prose (a future loom-finalize), not a machine contract.
type Summary struct {
	Title string
	Body  string
}

// ParseSummary reads and validates the summary.md file at path per
// discussion.md's summary-artifact decision: fail-loud, minimal validation
// only. The file must exist and carry at least one non-blank line, and that
// first non-blank line must be a "# <title>" heading with a non-empty
// title; every line after that heading is Body, exactly as written, never
// itself schema-validated. Every violation is its own distinct wrapped
// error, the same fail-loud posture builderengine.ParseOutcome applies to
// outcome.yaml.
func ParseSummary(path string) (*Summary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("webster: read summary file %s: %w", path, err)
	}

	if strings.TrimSpace(string(data)) == "" {
		return nil, fmt.Errorf("webster: summary file %s is empty", path)
	}

	// The non-empty check above guarantees at least one line is non-blank,
	// so this loop always finds a headingIdx before running out of lines.
	lines := strings.Split(string(data), "\n")
	headingIdx := 0
	for headingIdx < len(lines) && strings.TrimSpace(lines[headingIdx]) == "" {
		headingIdx++
	}

	heading := strings.TrimSpace(lines[headingIdx])
	if !strings.HasPrefix(heading, "# ") {
		return nil, fmt.Errorf("webster: summary file %s: first non-blank line %q is not a %q heading", path, heading, "# <title>")
	}
	title := strings.TrimSpace(strings.TrimPrefix(heading, "# "))
	if title == "" {
		return nil, fmt.Errorf("webster: summary file %s: title heading has an empty title", path)
	}

	body := strings.Join(lines[headingIdx+1:], "\n")
	return &Summary{Title: title, Body: body}, nil
}

// ArchiveStaleSummary renames websterDir's summary.md, if present, to
// summary-<UTC compact timestamp>.md in place, mirroring
// builderengine.ArchiveStaleOutcome's archive-never-refuse posture: a prior
// run's own summary stays on disk, auditable, rather than being silently
// overwritten or deleted when `run` spawns a fresh Master. now is a seam so
// tests can pin the timestamp deterministically instead of racing the real
// clock; production callers pass time.Now. Absent file: returns ("", nil) —
// not an error — since a fresh run has never written one yet. Collision:
// reuses builderengine.FirstFreeArchivePath's "-1"/"-2" suffix loop, per the
// reuse-by-import-never-copy decision, so two archives landing in the same
// clock-second never clobber each other.
func ArchiveStaleSummary(websterDir string, now func() time.Time) (archivedTo string, err error) {
	path := SummaryPath(websterDir)

	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return "", nil
		}
		return "", fmt.Errorf("webster: stat summary file %s: %w", path, statErr)
	}

	stamp := now().UTC().Format(summaryArchiveTimestampFormat)
	target, err := builderengine.FirstFreeArchivePath(func(suffix string) string {
		return filepath.Join(websterDir, fmt.Sprintf("summary-%s%s.md", stamp, suffix))
	})
	if err != nil {
		return "", fmt.Errorf("webster: find archive target for summary file %s: %w", path, err)
	}

	if err := os.Rename(path, target); err != nil {
		return "", fmt.Errorf("webster: archive stale summary file %s: %w", path, err)
	}
	return target, nil
}
