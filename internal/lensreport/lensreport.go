// lensreport.go holds the file name rule of a lens report beside its record.
// PathPattern gives the pattern for a prompt, Path gives one lens's path, and IsReportName checks a directory entry.
// Returned reads whether a report says its lens returned.

package lensreport

import (
	"os"
	"path/filepath"
	"strings"
)

// SpecName is the registered name of the spec that defines a lens report's format.
const SpecName = "twill-lens-report-spec"

// lensPlaceholder is the text that stands for the lens name in a report path pattern.
const lensPlaceholder = "<lens>"

// reportInfix joins a record's stem to the lens name in a report's file name.
const reportInfix = ".lens-"

// reportExt is every report's extension.
const reportExt = ".md"

// blockFence opens and closes a report's opening yaml block.
const blockFence = "```"

// outcomeKey is the key of the opening block's line that says whether the lens returned a final message.
const outcomeKey = "outcome:"

// outcomeReported is the outcome value of a lens that returned a final message.
const outcomeReported = "reported"

// PathPattern returns the path of a lens report beside recordPath, with `<lens>` standing for the lens name.
// The record's extension is dropped, so `x/lens-review.md` gives `x/lens-review.lens-<lens>.md`.
func PathPattern(recordPath string) string {
	return Path(recordPath, lensPlaceholder)
}

// Path returns the path of the report for the lens named lens, beside recordPath.
func Path(recordPath, lens string) string {
	return reportStem(recordPath) + reportInfix + lens + reportExt
}

// IsReportName reports whether name is the file name of a lens report beside a record whose file name is recordName.
// Both are base names; a name with an empty lens is no report.
func IsReportName(recordName, name string) bool {
	lens, ok := strings.CutPrefix(name, reportStem(recordName)+reportInfix)
	if !ok {
		return false
	}
	lens, ok = strings.CutSuffix(lens, reportExt)
	return ok && lens != ""
}

// Returned reports whether the lens report at path says its lens returned a report.
// It reads the outcome line of the file's opening fenced yaml block alone.
// It reports false for an unreadable file, an absent one included, and for a file with no well-formed opening block.
// Any outcome other than `reported`, such as `no report`, also reports false.
func Returned(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	lines := strings.Split(string(content), "\n")
	if !strings.HasPrefix(strings.TrimSpace(lines[0]), blockFence) {
		return false
	}
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == blockFence {
			return false
		}
		if value, ok := strings.CutPrefix(line, outcomeKey); ok {
			return strings.TrimSpace(value) == outcomeReported
		}
	}
	return false
}

// reportStem returns path without its extension.
func reportStem(path string) string {
	return strings.TrimSuffix(path, filepath.Ext(path))
}
