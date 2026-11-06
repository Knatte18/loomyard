// lensreport.go declares the lens report file name rule, PathPattern for a prompt, Path for one lens and IsReportName for a directory check, and reads a report's outcome with Returned.

package lensreport

import (
	"os"
	"path/filepath"
	"strings"
)

// SpecName is the registered name of the spec that defines a lens report's format.
const SpecName = "twill-lens-report-spec"

// lensPlaceholder is the text PathPattern puts where the lens name goes.
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

// PathPattern returns the path a writer saves a lens fork's report at beside recordPath, with `<lens>` standing for the lens name.
// The record's extension is dropped, so `x/lens-review.md` gives `x/lens-review.lens-<lens>.md`.
func PathPattern(recordPath string) string {
	return Path(recordPath, lensPlaceholder)
}

// Path returns the path a writer saves the report of the lens named lens at beside recordPath.
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
// An unreadable file, an absent one included, a file with no well-formed opening block and a report whose outcome is `no report` did not.
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
