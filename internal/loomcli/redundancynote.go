// redundancynote.go renders the gate's redundancy report as the note of a code review rubric's redundancy_report marker.

package loomcli

import (
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gatecli"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// noCurrentRedundancyReport opens the note a review gets when no redundancy report holds for HEAD.
const noCurrentRedundancyReport = "No current redundancy report"

// redundancyReportNote returns the closure that reads the redundancy report of the worktree at location.
// The closure reads it each time a review rubric carrying the marker is filled.
// The note lists the report's tests when the report matches HEAD's tree.
// Otherwise it says that no current report exists and why.
func redundancyReportNote(location *lymcwd.Location) func() string {
	return func() string {
		report, found, err := gatecli.ReadRedundancyReport(location)
		if err != nil {
			return fmt.Sprintf("%s: the report could not be read: %v.", noCurrentRedundancyReport, err)
		}
		return renderRedundancyReportNote(report, found, func() (bool, error) {
			fabric, err := fabricengine.Open(location)
			if err != nil {
				return false, err
			}
			head, err := fabric.HeadSHA()
			if err != nil {
				return false, err
			}
			return gatecli.RedundancyReportMatches(location, report, head)
		})
	}
}

// renderRedundancyReportNote renders report as a code review reads it.
// found is whether a report exists, and matchesHead reports whether it holds for HEAD's tree;
// it is called only for a report that judged.
// A matching report renders one line per test and per error line.
// Any other renders one line saying no current report exists and why.
// That line names the gate key of a report written on a red tier.
func renderRedundancyReportNote(report gatecli.RedundancyReport, found bool, matchesHead func() (bool, error)) string {
	if !found {
		return noCurrentRedundancyReport + ": no gate call of this worktree has written one."
	}
	call := fmt.Sprintf("the `%s` gate call of %s", report.GateKey, report.Written.UTC().Format(time.RFC3339))
	switch {
	case report.RedGate:
		return fmt.Sprintf("%s: a tier of %s was red, so its report judged nothing.", noCurrentRedundancyReport, call)
	case report.Note != "":
		return fmt.Sprintf("%s: the report of %s judged nothing: %s.", noCurrentRedundancyReport, call, report.Note)
	}
	matched, err := matchesHead()
	if err != nil {
		return fmt.Sprintf("%s: the report of %s could not be matched against HEAD: %v.", noCurrentRedundancyReport, call, err)
	}
	if !matched {
		return fmt.Sprintf("%s: the report of %s was written for a tree other than HEAD's.", noCurrentRedundancyReport, call)
	}
	lines := []string{fmt.Sprintf("The redundancy report of %s matches HEAD's tree.", call)}
	for _, test := range report.Tests {
		line := fmt.Sprintf("- `%s.%s` in `%s`: %s", test.Package, test.Name, test.File, test.Verdict)
		switch {
		case test.Verdict == gatecli.RedundancyContained:
			line += ", covered by `" + strings.Join(test.Covering, "`, `") + "`"
		case test.Reason != "":
			line += ", " + test.Reason
		}
		lines = append(lines, line)
	}
	for _, failure := range report.Errors {
		lines = append(lines, "- error: "+failure)
	}
	return strings.Join(lines, "\n")
}
