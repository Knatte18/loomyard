// fixerreport.go declares the merge fixer's report model and ParseFixerReport,
// and the verdict-and-sections reader that both the fixer's and the fix review's report parsers share.
// ParseFixerReport is the one reader of the report format that contracts/specs/landing-fixer-report-spec.md defines.

package landingshed

import (
	"fmt"
	"os"
	"slices"
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
// A first line that is no known verdict is an error naming the line and the spec.
// An escalate report without a non-empty `## Conflict` section is an error too, naming the spec.
func ParseFixerReport(path string) (FixerReport, error) {
	parsed, err := parseVerdictReport(path, "fixer", fixerReportSpecName, string(FixerFixed), string(FixerEscalate))
	if err != nil {
		return FixerReport{}, err
	}
	report := FixerReport{
		Verdict:       FixerVerdict(parsed.verdict),
		ParentChanges: bullets(parsed.sections["Parent changes"]),
		Fixes:         bullets(parsed.sections["Fixes"]),
	}
	if report.Verdict == FixerEscalate {
		report.Conflict = strings.TrimSpace(strings.Join(parsed.sections["Conflict"], "\n"))
		if report.Conflict == "" {
			return FixerReport{}, fmt.Errorf("landingshed: fixer report %s: an escalate report needs a non-empty \"## Conflict\" section, per %s", path, fixerReportSpecName)
		}
	}
	return report, nil
}

// verdictReport is a session report laid out as a verdict on its first line, then sections under `## ` headings.
type verdictReport struct {
	verdict string
	// sections holds the lines under each heading, keyed by the heading's text.
	sections map[string][]string
}

// parseVerdictReport reads the kind report at path, whose first line is fixerVerdictPrefix and one of verdicts.
// A missing file is an error wrapping fs.ErrNotExist.
// A first line that is no known verdict is an error naming the line, the first two verdicts and spec.
func parseVerdictReport(path, kind, spec string, verdicts ...string) (verdictReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return verdictReport{}, fmt.Errorf("landingshed: read %s report %s: %w", kind, path, err)
	}
	lines := strings.Split(string(data), "\n")
	first := strings.TrimSpace(lines[0])
	verdict, ok := strings.CutPrefix(first, fixerVerdictPrefix)
	if !ok || !slices.Contains(verdicts, verdict) {
		return verdictReport{}, fmt.Errorf("landingshed: %s report %s: line 1 %q is no known verdict; the first line is %q or %q, per %s",
			kind, path, first, fixerVerdictPrefix+verdicts[0], fixerVerdictPrefix+verdicts[1], spec)
	}
	report := verdictReport{verdict: verdict, sections: map[string][]string{}}
	section := ""
	for _, line := range lines[1:] {
		if heading, ok := strings.CutPrefix(line, "## "); ok {
			section = strings.TrimSpace(heading)
			continue
		}
		report.sections[section] = append(report.sections[section], line)
	}
	return report, nil
}

// bullets returns the `- ` items of lines, each with its marker removed.
func bullets(lines []string) []string {
	var items []string
	for _, line := range lines {
		if item, ok := strings.CutPrefix(strings.TrimSpace(line), "- "); ok {
			items = append(items, item)
		}
	}
	return items
}
