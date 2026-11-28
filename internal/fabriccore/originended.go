// originended.go holds the rules of removing the pairs of runs origin has ended, and of what that removal leaves.
// They decide which pair is torn down next, what each ended entry reports and which records go with it.

package fabriccore

import (
	"slices"
	"sort"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
)

// SeamTeardown names the seam call that removes the local pair of an ended entry, the slug in the action's Ref.
const SeamTeardown = "teardown"

// FieldEndedRuns is what is read of each run origin has ended before any teardown.
const FieldEndedRuns Field = "ended_runs"

// FieldTeardownReports is what each teardown run so far reported.
const FieldTeardownReports Field = "teardown_reports"

// EndedFacts is what is read for removing the pairs of ended runs.
type EndedFacts struct {
	Scope fabriclayout.ReconcileScope
	// Seam reports that a teardown seam is installed.
	Seam bool
	// Runs are the ended runs in slug order.
	Runs []EndedRun
}

// EndedRun is one run origin has ended.
type EndedRun struct {
	Slug string
	How  string
	// Local reports that this machine holds a part of the run's pair.
	Local bool
	// EarlierSnapshot reports that an earlier snapshot tag of the slug exists; it is read only of a pair torn down.
	EarlierSnapshot bool
	// Report is what the teardown of the pair reported, nil while none ran.
	// Its Error is the error the teardown ended in, from either source.
	Report *fabriclayout.TeardownReport
}

// TearsDown reports whether the pair of run is one to tear down.
// A pair with no local part, a judge-only scope, the scope's own run and a missing teardown seam are not.
func (f EndedFacts) TearsDown(run EndedRun) bool {
	return run.Local && !f.Scope.JudgeOnly && f.Scope.Run != run.Slug && f.Seam
}

// EndedTeardowns is the rule that tears down the pairs of the ended runs, one seam call at a time in slug order.
// A teardown that failed or found its pair busy stops no other.
// It is done once every pair to tear down has a report.
func EndedTeardowns(snapshot Snapshot) Answer {
	for _, run := range snapshot.Ended.Runs {
		if run.Report == nil && snapshot.Ended.TearsDown(run) {
			return Answer{
				Actions: []Action{{Kind: ActionSeamCall, Seam: SeamTeardown, Ref: run.Slug}},
				Reread:  []Field{FieldTeardownReports},
			}
		}
	}
	return Answer{}
}

// EndedEntries returns the outcome of each ended run, in the order the runs are held.
// A run whose pair was not torn down is only ended.
// A busy teardown leaves the pair and says what lets the next run remove it.
// A failed one puts its error on the entry, and any other reads removed.
func EndedEntries(snapshot Snapshot) []fabriclayout.EntryOutcome {
	var entries []fabriclayout.EntryOutcome
	for _, run := range snapshot.Ended.Runs {
		entry := fabriclayout.EntryOutcome{Slug: run.Slug, State: fabriclayout.EntryEnded, How: run.How, Local: run.Local}
		if report := run.Report; report != nil {
			entry.Tags = report.Tags
			entry.Kept = report.Kept
			switch {
			case report.Busy:
				entry.Busy = true
				entry.WayForward = busyWayForward(report.BusyOn)
			case report.Error != nil:
				entry.Error = report.Error.Error()
			default:
				entry.Removed = true
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

// busyWayForward names what lets a removal that found its pair busy go through at the next run.
func busyWayForward(busyOn string) string {
	if busyOn == fabriclayout.BusyOnDriver {
		return "wait for the pair's driver to end, which the run's next Parent-Align or Finalize makes certain by stopping it with Halt, then re-run `lym cleanup --apply`"
	}
	return "wait for the holder of the pair's teardown or weft write lock to release it, then re-run `lym cleanup --apply`"
}

// MergeEndedEntries returns classified with each slug an ended entry covers taken from ended;
// it appends the ended entries of no classified slug after them.
// The result is sorted by slug.
func MergeEndedEntries(classified, ended []fabriclayout.EntryOutcome) []fabriclayout.EntryOutcome {
	merged := slices.Clone(classified)
	for _, entry := range ended {
		if i := slices.IndexFunc(merged, func(candidate fabriclayout.EntryOutcome) bool { return candidate.Slug == entry.Slug }); i >= 0 {
			merged[i] = entry
			continue
		}
		merged = append(merged, entry)
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Slug < merged[j].Slug })
	return merged
}

// WithoutRemovedPairs returns branches without those of a pair an ended entry shows removed.
func WithoutRemovedPairs(branches []fabriclayout.BranchOutcome, entries []fabriclayout.EntryOutcome) []fabriclayout.BranchOutcome {
	removed := map[string]bool{}
	for _, entry := range entries {
		if entry.Removed {
			removed[entry.Slug] = true
		}
	}
	if len(removed) == 0 {
		return branches
	}
	kept := make([]fabriclayout.BranchOutcome, 0, len(branches))
	for _, branch := range branches {
		if !removed[branch.Slug] {
			kept = append(kept, branch)
		}
	}
	return kept
}

// LocalBranchKey is the key of a branch of the repository at repoPath among the branches that are local.
func LocalBranchKey(repoPath, branch string) string {
	return repoPath + "\x00" + branch
}

// StalePushFailures returns the failed-push records whose branch is not local.
// local holds the LocalBranchKey of each branch that is.
func StalePushFailures(records []fabriclayout.FailedPush, local map[string]bool) []fabriclayout.FailedPush {
	var stale []fabriclayout.FailedPush
	for _, record := range records {
		if !local[LocalBranchKey(record.RepoPath, record.Branch)] {
			stale = append(stale, record)
		}
	}
	return stale
}
