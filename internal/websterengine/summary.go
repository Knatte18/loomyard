// summary.go implements webster's write-side helpers over the final-summary artifact (AppendIntegrationTriage, AppendIntegrationFix and AppendAuditWarnings append further sections beside the two below):
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
	"strings"
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
// A non-empty regressions list adds each regressing identity with its output tail in a fenced block;
// nil regressions leave the section as the localized-card sentence alone.
func AppendIntegrationFailure(websterDir, offendingCard, offendingSHA string, regressions []IntegrationFailure) error {
	var b strings.Builder
	fmt.Fprintf(&b, "\n\n## Integration suite failed\n\nThe plan-level `## verify:` suite failed. SHA-bisect localized the failure to card `%s` (commit `%s`).\n", offendingCard, offendingSHA)
	if len(regressions) > 0 {
		b.WriteString("\nRegressing failures:\n")
		for _, r := range regressions {
			fmt.Fprintf(&b, "\n### `%s`\n\n```\n%s\n```\n", r.ID, strings.TrimRight(r.Tail, "\n"))
		}
	}
	return appendToSummary(websterDir, "integration failure", b.String())
}

// AppendIntegrationTriage appends a section listing the failures webster's triage did not attribute to this run, by identity only;
// the tails stay in the integration report.
// It is a no-op when both lists are empty.
func AppendIntegrationTriage(websterDir string, flaky, preExisting []string) error {
	if len(flaky) == 0 && len(preExisting) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("\n\n## Integration suite triage\n\nThe plan-level `## verify:` suite failed, but webster's triage did not attribute the failure to this run.\n")
	writeTriageList(&b, "Flaky (passed on rerun)", flaky)
	writeTriageList(&b, "Pre-existing (already failing at the plan's starting commit)", preExisting)
	return appendToSummary(websterDir, "integration triage", b.String())
}

// AppendIntegrationFix appends a section recording the integration-fix attempt: the result, the pre-fix head, each fix commit and the cleared identities.
// A result other than FixResultFixed adds the remaining identities, the detail and a way-forward sentence naming the pre-fix head and the fix commits, so the operator decides whether to keep them.
func AppendIntegrationFix(websterDir string, fix IntegrationFixRecord) error {
	var b strings.Builder
	fmt.Fprintf(&b, "\n\n## Integration suite fix\n\nA fix strand attempted the integration regression; result: `%s`.\nPre-fix head: `%s`.\n", fix.Result, fix.PreFixHead)
	writeTriageList(&b, "Fix commits", fix.Commits)
	writeTriageList(&b, "Cleared regressions", fix.Cleared)
	if fix.Result != FixResultFixed {
		writeTriageList(&b, "Remaining regressions", fix.Remaining)
		if fix.Detail != "" {
			fmt.Fprintf(&b, "\nDetail: %s\n", fix.Detail)
		}
		commits := "none"
		if len(fix.Commits) > 0 {
			commits = "`" + strings.Join(fix.Commits, "`, `") + "`"
		}
		fmt.Fprintf(&b, "\nThe fix commits (%s) stay on the branch; decide whether to keep them. `git reset --hard %s` drops them.\n", commits, fix.PreFixHead)
	}
	return appendToSummary(websterDir, "integration fix", b.String())
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
