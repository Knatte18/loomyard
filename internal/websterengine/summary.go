// summary.go implements webster's two write-side helpers over the final-summary artifact:
// ArchiveStaleSummary applies the same archive-never-refuse timestamp-rename discipline as
// outcome.go's own archiveStaleOutcome, reusing archive.go's firstFreeArchivePath rather than
// re-implementing the same-second collision loop; AppendIntegrationFailure extends an
// already-written summary artifact with the integration-suite bisect's own localized finding
// (integration.go's BisectAndEscalate), the summary-document half of that escalation path. The
// artifact's read contract -- its path and its parse -- lives in internal/summaryparser, the sole
// owner of that shape.

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// ArchiveStaleSummary renames websterDir's final-summary artifact to summary-<UTC compact
// timestamp>.md, preserving it rather than deleting.
// Absent file returns ("", nil).
// Collision within the same clock-second appends a numeric suffix.
func ArchiveStaleSummary(websterDir string, now func() time.Time) (archivedTo string, err error) {
	path := summaryparser.Path(websterDir)

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

// AppendIntegrationFailure appends a section naming the integration bisect's localized finding to
// the final-summary artifact.
// Master's final-action rule guarantees the artifact exists before this runs.
func AppendIntegrationFailure(websterDir, offendingCard, offendingSHA string) error {
	path := summaryparser.Path(websterDir)
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
