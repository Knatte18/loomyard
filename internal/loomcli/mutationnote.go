// mutationnote.go renders the gate's mutation report as the note of a code review rubric's mutation_report marker.

package loomcli

import (
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gatecli"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// noCurrentMutationReport opens the note a review gets when no mutation report holds for HEAD.
const noCurrentMutationReport = "No current mutation report"

// mutationReportNote returns the closure that reads the mutation report of the worktree at location.
// The closure reads it each time a review rubric carrying the marker is filled.
// The note lists the report's counts and survivors when the report matches HEAD's tree.
// Otherwise it says that no current report exists and why.
func mutationReportNote(location *lymcwd.Location) func() string {
	return func() string {
		report, found, err := gatecli.ReadMutationReport(location)
		if err != nil {
			return fmt.Sprintf("%s: the report could not be read: %v.", noCurrentMutationReport, err)
		}
		return renderMutationReportNote(report, found, func() (bool, error) {
			fabric, err := fabricengine.Open(location)
			if err != nil {
				return false, err
			}
			head, err := fabric.HeadSHA()
			if err != nil {
				return false, err
			}
			return gatecli.MutationReportMatches(location, report, head)
		})
	}
}

// renderMutationReportNote renders report as a code review reads it.
// found is whether a report exists, and matchesHead reports whether it holds for HEAD's tree;
// it is called only for a report that judged and was not written in the prime.
// A matching report renders its review lines under one line naming the gate call that wrote it.
// Any other renders one line saying no current report exists and why.
// That line names the gate key of a report written on a red tier.
func renderMutationReportNote(report gatecli.MutationReport, found bool, matchesHead func() (bool, error)) string {
	if !found {
		return noCurrentMutationReport + ": no gate call of this worktree has written one."
	}
	call := fmt.Sprintf("the `%s` gate call of %s", report.GateKey, report.Written.UTC().Format(time.RFC3339))
	switch {
	case report.RedGate:
		return fmt.Sprintf("%s: a tier of %s was red, so its report judged nothing.", noCurrentMutationReport, call)
	case report.Note != "":
		return fmt.Sprintf("%s: the report of %s judged nothing: %s.", noCurrentMutationReport, call, report.Note)
	case report.Prime:
		return fmt.Sprintf("%s: the report of %s was written in the prime, which has no parent branch, so it matches no HEAD.", noCurrentMutationReport, call)
	}
	matched, err := matchesHead()
	if err != nil {
		return fmt.Sprintf("%s: the report of %s could not be matched against HEAD: %v.", noCurrentMutationReport, call, err)
	}
	if !matched {
		return fmt.Sprintf("%s: the report of %s was written for a tree other than HEAD's.", noCurrentMutationReport, call)
	}
	lines := append([]string{fmt.Sprintf("The mutation report of %s matches HEAD's tree.", call)}, report.ReviewLines(gatecli.MutationReportFileName)...)
	return strings.Join(lines, "\n")
}
