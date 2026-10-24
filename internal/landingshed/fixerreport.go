// fixerreport.go declares the merge fixer's report model and ParseFixerReport, the one reader of the report format contracts/specs/landing-fixer-report-spec.md defines.

package landingshed

import (
	"fmt"
	"os"
	"strings"
)

// FixerVerdict is the verdict on a merge fixer report's first line.
type FixerVerdict string

const (
	// FixerFixed is the verdict of a fixer that made every change the failure needed.
	FixerFixed FixerVerdict = "fixed"
	// FixerEscalate is the verdict of a fixer that found a parent change conflicting with a decision of the record.
	FixerEscalate FixerVerdict = "escalate"
)

// fixerReportSpecName is the spec every fixer report error points the reader to.
const fixerReportSpecName = "landing-fixer-report-spec.md"

// fixerVerdictPrefix opens a fixer report's first line.
const fixerVerdictPrefix = "verdict: "

// FixerReport is the parsed form of a merge fixer's end-of-turn report.
// ParentChanges and Fixes hold one entry per bullet with the bullet marker removed.
// Conflict is the trimmed `## Conflict` text, set on an escalate report only.
type FixerReport struct {
	Verdict       FixerVerdict
	ParentChanges []string
	Fixes         []string
	Conflict      string
}

// ParseFixerReport reads the fixer report at path.
// A missing file is an error wrapping fs.ErrNotExist.
// A first line that is no known verdict, and an escalate report without a non-empty `## Conflict` section, are errors naming the line and the spec.
func ParseFixerReport(path string) (FixerReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FixerReport{}, fmt.Errorf("landingshed: read fixer report %s: %w", path, err)
	}

	lines := strings.Split(string(data), "\n")
	first := strings.TrimSpace(lines[0])
	verdict := FixerVerdict(strings.TrimPrefix(first, fixerVerdictPrefix))
	if !strings.HasPrefix(first, fixerVerdictPrefix) || (verdict != FixerFixed && verdict != FixerEscalate) {
		return FixerReport{}, fmt.Errorf("landingshed: fixer report %s: line 1 %q is no known verdict; the first line is %q or %q, per %s",
			path, first, fixerVerdictPrefix+string(FixerFixed), fixerVerdictPrefix+string(FixerEscalate), fixerReportSpecName)
	}

	report := FixerReport{Verdict: verdict}
	var conflictLines []string
	hasConflictSection := false
	section := ""
	for _, line := range lines[1:] {
		if heading, ok := strings.CutPrefix(line, "## "); ok {
			section = strings.TrimSpace(heading)
			if section == "Conflict" {
				hasConflictSection = true
			}
			continue
		}
		switch section {
		case "Parent changes":
			if bullet, ok := strings.CutPrefix(strings.TrimSpace(line), "- "); ok {
				report.ParentChanges = append(report.ParentChanges, bullet)
			}
		case "Fixes":
			if bullet, ok := strings.CutPrefix(strings.TrimSpace(line), "- "); ok {
				report.Fixes = append(report.Fixes, bullet)
			}
		case "Conflict":
			conflictLines = append(conflictLines, line)
		}
	}

	if verdict == FixerEscalate {
		report.Conflict = strings.TrimSpace(strings.Join(conflictLines, "\n"))
		if !hasConflictSection || report.Conflict == "" {
			return FixerReport{}, fmt.Errorf("landingshed: fixer report %s: an escalate report needs a non-empty \"## Conflict\" section, per %s", path, fixerReportSpecName)
		}
	}
	return report, nil
}
