// summary.go implements webster's own prose summary artifact contract:
// SummaryPath resolves the fixed summary.md path inside a webster dir;
// ParseSummary enforces discussion.md's summary-artifact decision's minimal
// fail-loud validation (presence, non-empty, a "# <title>" first non-blank
// line with a non-empty title) — the artifact is the future loom-finalize
// PR-text source, never itself schema-validated beyond that;
// ArchiveStaleSummary applies the same archive-never-refuse timestamp-rename
// discipline as outcome.go's own archiveStaleOutcome, reusing archive.go's
// firstFreeArchivePath rather than re-implementing the same-second
// collision loop; and AppendIntegrationFailure extends an already-written
// summary.md with the integration-suite bisect's own localized finding
// (integration.go's BisectAndEscalate), the summary-document half of that
// escalation path.

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SummaryFileName is summary.md's fixed filename inside a webster dir.
const SummaryFileName = "summary.md"

// SummaryPath returns the path to summary.md inside websterDir.
func SummaryPath(websterDir string) string {
	return filepath.Join(websterDir, SummaryFileName)
}

// Summary is Master's final-action prose artifact: Title from the "# <title>"
// heading, Body from the remaining lines verbatim.
type Summary struct {
	Title string
	Body  string
}

// ParseSummary reads and validates summary.md at path with minimal fail-loud
// validation: file must exist, have at least one non-blank line, and that
// first line must be a "# <title>" heading with non-empty title. Every
// violation is its own distinct wrapped error.
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

// ArchiveStaleSummary renames websterDir's summary.md to
// summary-<UTC compact timestamp>.md, preserving it rather than deleting.
// Absent file returns ("", nil). Collision within the same clock-second
// appends a numeric suffix.
func ArchiveStaleSummary(websterDir string, now func() time.Time) (archivedTo string, err error) {
	path := SummaryPath(websterDir)

	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return "", nil
		}
		return "", fmt.Errorf("webster: stat summary file %s: %w", path, statErr)
	}

	stamp := now().UTC().Format(archiveTimestampFormat)
	target, err := firstFreeArchivePath(func(suffix string) string {
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

// AppendIntegrationFailure appends a section naming the integration bisect's
// localized finding to summary.md. Master's final-action rule guarantees
// summary.md exists before this runs.
func AppendIntegrationFailure(websterDir, offendingCard, offendingSHA string) error {
	path := SummaryPath(websterDir)
	section := fmt.Sprintf("\n\n## Integration suite failed\n\nThe plan-level `## verify:` suite failed. SHA-bisect localized the failure to card `%s` (commit `%s`).\n", offendingCard, offendingSHA)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("webster: append integration failure to summary file %s: %w", path, err)
	}
	defer f.Close()

	if _, err := f.WriteString(section); err != nil {
		return fmt.Errorf("webster: append integration failure to summary file %s: %w", path, err)
	}
	return nil
}
