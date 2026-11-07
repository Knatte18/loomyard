// verdict.go renders a task's regression check from its committed drift records.
// The result is the markdown section a landing carries in its pull request and landing commit.

package driftengine

import (
	"fmt"
	"strings"
)

// verdictHeading is the heading that opens the regression check section.
const verdictHeading = "## Regression check"

// Verdict renders the task's regression check as a markdown section from its drift records,
// so a hand edit of a record never reaches the landing, and it runs no capture.
// The section names the scope, the commit the counted baseline was captured at, and the last check's counts.
// It names the expectation entries no cell used and the capture files and keys marked as metadata.
// It also names the open ranges the judge looked into and the parent's amendments.
// It returns "" when no scope record exists,
// so a task no Drift row recorded a scope for carries no section, and it creates no directory.
func Verdict(d Deps) (string, error) {
	records, err := ReadRecords(d)
	if err != nil {
		return "", err
	}
	if !records.ScopeFound {
		return "", nil
	}
	baseline, found, err := d.TrailerCommit(BaselineTrailerKey, false)
	if err != nil {
		return "", fmt.Errorf("drift: find the oldest %s commit for the verdict: %w", BaselineTrailerKey, err)
	}
	baseHead := ""
	if found {
		baseHead = baseline.CodeHead
	}
	return renderVerdict(records, baseHead), nil
}

// renderVerdict renders rec as the markdown regression check section.
// baseHead is the commit the counted baseline was captured at, empty when no commit recorded one.
func renderVerdict(rec Records, baseHead string) string {
	scope := rec.Scope
	lines := []string{verdictHeading, "", "- Scope: " + scopeText(scope)}
	if scope.Scope == ScopeOff {
		return strings.Join(lines, "\n")
	}
	switch {
	case scope.Deferred:
		lines = append(lines, "- Capture: deferred, so no compare ran.")
	case scope.LastCheck.At != "" || scope.LastCheck.Head != "":
		mode := scope.LastCheck.Mode
		if mode == "" {
			mode = scope.Diff
		}
		if baseHead != "" {
			lines = append(lines, fmt.Sprintf("- Baseline: captured at `%s`, the task's branch point.", baseHead))
		}
		lines = append(lines, "- Diff: "+diffText(mode))
		lines = append(lines, checkLines(scope.LastCheck)...)
	default:
		lines = append(lines, oldCheckLines(scope)...)
	}
	lines = append(lines, metadataLines(scope.Metadata)...)
	lines = append(lines, rangeLines(rec.Judged.Ranges)...)
	lines = append(lines, amendmentLines(scope.Amendments)...)
	return strings.Join(lines, "\n")
}

// metadataLines renders the capture files and line keys marked as metadata, which no check compared.
func metadataLines(marks Volatile) []string {
	if len(marks.Files)+len(marks.Keys) == 0 {
		return nil
	}
	quote := func(items []string) string {
		quoted := make([]string, len(items))
		for i, item := range items {
			quoted[i] = "`" + item + "`"
		}
		return strings.Join(quoted, ", ")
	}
	var parts []string
	if len(marks.Files) > 0 {
		parts = append(parts, "files "+quote(marks.Files))
	}
	if len(marks.Keys) > 0 {
		parts = append(parts, "keys "+quote(marks.Keys))
	}
	return []string{"- Left out as capture metadata, not results: " + strings.Join(parts, "; ") + "."}
}

// scopeText renders the scope line's value.
func scopeText(scope ScopeRecord) string {
	switch scope.Scope {
	case ScopeOff:
		if scope.ScopeSource != "" {
			return fmt.Sprintf("off, set by `%s`; the task is not regression-checked.", scope.ScopeSource)
		}
		return "off; the task is not regression-checked."
	case ScopePartial:
		quoted := make([]string, len(scope.Skip))
		for i, test := range scope.Skip {
			quoted[i] = "`" + test + "`"
		}
		return "partial, an old scope that left out the cases " + strings.Join(quoted, ", ") + "."
	}
	return "full."
}

// diffText renders the diff line's value.
func diffText(mode string) string {
	if mode == DiffCommand {
		return "command; containment was the judge's, which ruled each failing case of the compare."
	}
	return "lines."
}

// checkLines renders the last check's lines: its outcome and counts, then each expectation entry that covered no cell.
func checkLines(check CheckResult) []string {
	outcome := "did not pass"
	if check.Passed {
		outcome = "passed"
	}
	line := fmt.Sprintf("- Last check: %s at `%s`.", outcome, check.Head)
	if check.Mode == DiffCommand {
		return []string{line + fmt.Sprintf(" The compare exited %d.", check.CompareExit)}
	}
	line += fmt.Sprintf(" %d changed cells, %d expected, %d judged as matching, %d reused from stored rulings, %d unexpected.",
		check.Changed, check.Expected, check.Matched, check.Reused, check.Unexpected)
	lines := []string{line}
	if len(check.UncoveredEntries) > 0 {
		lines = append(lines, "- Expectation entries that covered no cell:")
		for _, entry := range check.UncoveredEntries {
			lines = append(lines, fmt.Sprintf("  - `%s`", entry))
		}
	}
	return lines
}

// rangeLines renders each open range with the covering entry's why and the judge's sentence on what it checked under it.
func rangeLines(ranges []JudgedRange) []string {
	if len(ranges) == 0 {
		return nil
	}
	lines := []string{"- Open ranges:"}
	for _, r := range ranges {
		lines = append(lines, fmt.Sprintf("  - `%s` of `%s`: why: %s; checked: %s", r.Range, r.Case, r.Why, r.Checked))
	}
	return lines
}

// amendmentLines renders each amendment with who made it and, for an `entries` amendment, the entries it added.
func amendmentLines(amendments []Amendment) []string {
	if len(amendments) == 0 {
		return nil
	}
	lines := []string{"- Amendments by the run's parent:"}
	for _, amendment := range amendments {
		if amendment.Entries == "" {
			lines = append(lines, fmt.Sprintf("  - `%s` by %s", amendment.Kind, amendment.By))
			continue
		}
		lines = append(lines, fmt.Sprintf("  - `%s` by %s:", amendment.Kind, amendment.By), "    ```yaml")
		for _, entryLine := range strings.Split(strings.TrimRight(amendment.Entries, "\n"), "\n") {
			lines = append(lines, "    "+entryLine)
		}
		lines = append(lines, "    ```")
	}
	return lines
}

// oldCheckLines renders the check lines of a scope with no recorded last check.
// It reads the old compare, waiver and acceptance fields.
// It says the check ran under the old Drift, or that none ran.
func oldCheckLines(scope ScopeRecord) []string {
	if scope.ComparedAt == "" && scope.WaivedAt == "" && len(scope.Accepted) == 0 {
		return []string{"- Check: not run."}
	}
	lines := []string{"- The check ran under the old Drift, which compared whole profiles."}
	if scope.BaselineHead != "" {
		lines = append(lines, fmt.Sprintf("- Baseline: captured at `%s`, the task's branch point.", scope.BaselineHead))
	}
	lines = append(lines, "- Compare: "+compareText(scope))
	if len(scope.Accepted) == 0 {
		return append(lines, "- Accepted drift: none.")
	}
	lines = append(lines, "- Accepted drift:")
	for _, acceptance := range scope.Accepted {
		line := fmt.Sprintf("  - `%s`, accepted by %s: %s", acceptance.Test, acceptance.By, acceptance.Reason)
		if acceptance.Ruling == RulingUnruled {
			line += " (not yet ruled on by the run's parent)"
		}
		lines = append(lines, line)
	}
	return lines
}

// compareText renders the old compare line's value.
func compareText(scope ScopeRecord) string {
	switch {
	case scope.WaivedAt != "":
		return "waived: " + scope.WaivedReason
	case scope.ComparedAt == "":
		return "not run."
	case scope.ComparePassed:
		return fmt.Sprintf("passed at `%s`.", scope.ComparedHead)
	}
	return fmt.Sprintf("showed drift at `%s`.", scope.ComparedHead)
}
