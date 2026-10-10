// summary.go implements webster's write-side helpers over the final-summary artifact (AppendIntegrationTriage, AppendAuditWarnings and AppendBackgroundShells append further sections beside the one below):
// ArchiveStaleSummary applies the same archive-never-refuse timestamp-rename discipline as
// outcome.go's own archiveStaleOutcome, reusing archive.go's firstFreeArchivePath rather than
// re-implementing the same-second collision loop.
// The artifact's read contract -- its path and its parse -- lives in internal/summaryparser, the sole
// owner of that shape.

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
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

// AppendIntegrationTriage appends a section listing the flaky identities the verify gate passed on rerun, by identity only.
// It is a no-op when flaky is empty.
func AppendIntegrationTriage(websterDir string, flaky []string) error {
	if len(flaky) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("\n\n## Integration suite triage\n\nThe plan-level `## verify:` suite failed, but webster's triage did not attribute the failure to this run.\n")
	writeTriageList(&b, "Flaky (passed on rerun)", flaky)
	return appendToSummary(websterDir, "integration triage", b.String())
}

// AppendAuditWarnings appends an "Audit warnings" section listing findings recorded as warnings, one bullet each in the order given.
// It is a no-op when warnings is empty.
func AppendAuditWarnings(websterDir string, warnings []string) error {
	if len(warnings) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("\n\n## Audit warnings\n\nThese findings (fork-audit policy findings, and drift about a later card) were recorded as warnings and did not stop the run.\n\n")
	for _, w := range warnings {
		fmt.Fprintf(&b, "- %s\n", w)
	}
	return appendToSummary(websterDir, "audit warnings", b.String())
}

// AppendBackgroundShells appends a "Background shells at the run's end" section listing the shells Master's run ended over, one bullet each in the order given with the shell's label, signal and time outstanding.
// It is a no-op when shells is empty.
func AppendBackgroundShells(websterDir string, shells []shuttleengine.EndedShell) error {
	if len(shells) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("\n\n## Background shells at the run's end\n\nMaster's run ended while these background shells were still outstanding; they may still be running in the session.\n\n")
	for _, shell := range shells {
		fmt.Fprintf(&b, "- `%s` (%s signal, outstanding %s)\n", shell.Label, shell.Signal, shell.Outstanding.Round(time.Second))
	}
	return appendToSummary(websterDir, "background shells", b.String())
}

// appendRebaselineWarning appends warning to the summary as its "Plan rebaselined" section, so the summary a step's Done points at names the rebaseline the run began with.
// An empty warning appends nothing.
func appendRebaselineWarning(websterDir, warning string) error {
	if warning == "" {
		return nil
	}
	return appendToSummary(websterDir, "plan rebaselined", "\n\n## Plan rebaselined\n\n"+warning+"\n")
}

// writeTriageList writes one titled sub-list of identities, or nothing when ids is empty.
func writeTriageList(b *strings.Builder, title string, ids []string) {
	if len(ids) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s:\n\n", title)
	for _, id := range ids {
		fmt.Fprintf(b, "- `%s`\n", id)
	}
}

// appendToSummary appends section to the final-summary artifact; what names the section in errors.
func appendToSummary(websterDir, what, section string) error {
	path := summaryparser.Path(websterDir)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("webster: append %s to summary file %s: %w", what, path, err)
	}
	defer f.Close()

	if _, err := f.WriteString(section); err != nil {
		return fmt.Errorf("webster: append %s to summary file %s: %w", what, path, err)
	}
	return nil
}
