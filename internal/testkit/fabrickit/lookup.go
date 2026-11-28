// lookup.go holds the lookups of one entry or one branch in an origin reconcile's report.

package fabrickit

import "github.com/Knatte18/loomyard/internal/fabriclayout"

// EntryOf returns the entry of slug in report, and whether the report holds one.
func EntryOf(report fabriclayout.OriginReport, slug string) (fabriclayout.EntryOutcome, bool) {
	for _, entry := range report.Entries {
		if entry.Slug == slug {
			return entry, true
		}
	}
	return fabriclayout.EntryOutcome{}, false
}

// BranchOf returns the outcome in report of branch on the repository side repo, and whether the report holds one.
func BranchOf(report fabriclayout.OriginReport, repo, branch string) (fabriclayout.BranchOutcome, bool) {
	for _, outcome := range report.Branches {
		if outcome.Repo == repo && outcome.Branch == branch {
			return outcome, true
		}
	}
	return fabriclayout.BranchOutcome{}, false
}
