// lensreport.go declares the lens report file name rule: PathPattern for a prompt, IsReportName for a directory check.

package lensreport

import (
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

// PathPattern returns the path a writer saves a lens fork's report at beside recordPath, with `<lens>` standing for the lens name.
// The record's extension is dropped, so `x/lens-review.md` gives `x/lens-review.lens-<lens>.md`.
func PathPattern(recordPath string) string {
	return reportStem(recordPath) + reportInfix + lensPlaceholder + reportExt
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

// reportStem returns path without its extension.
func reportStem(path string) string {
	return strings.TrimSuffix(path, filepath.Ext(path))
}
