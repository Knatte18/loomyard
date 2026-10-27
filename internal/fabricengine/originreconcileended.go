// originreconcileended.go holds the origin reconcile's ended-entries half.
// It tears down, through the injected seam, the local pair of each run origin has ended, and clears the failed-push records of branches that are no longer local.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// resolvePrime returns the location of the hub's prime worktree.
func resolvePrime(l *lymcwd.Location) (*lymcwd.Location, error) {
	primeName, err := PrimeName(l)
	if err != nil {
		return nil, err
	}
	return lymcwd.ResolveWorktree(filepath.Join(l.HubPath, primeName))
}

// recordedRuns returns the slugs of the runs this machine holds a batten run record of at the prime, in order.
// The run scope reads only its own run.
func recordedRuns(l *lymcwd.Location, scope ReconcileScope) ([]string, error) {
	prime, err := resolvePrime(l)
	if err != nil {
		return nil, err
	}
	candidates := []string{scope.Run}
	if scope.Run == "" {
		dirs, err := os.ReadDir(shedrun.RunsRoot(prime))
		if os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		candidates = candidates[:0]
		for _, dir := range dirs {
			if dir.IsDir() && !shedrun.IsReserved(dir.Name()) {
				candidates = append(candidates, dir.Name())
			}
		}
	}

	var runs []string
	for _, slug := range candidates {
		_, found, err := readRunStatusRows(shedrun.StatusFile(prime, slug))
		if err != nil {
			return nil, err
		}
		if found {
			runs = append(runs, slug)
		}
	}
	return runs, nil
}

// onlyRuns returns the part of ended whose slugs are among runs.
// A run only a replayed commit touched has no batten run record on this machine and is never classified.
func onlyRuns(ended EndedSet, runs []string) EndedSet {
	kept := EndedSet{}
	for slug, how := range ended {
		if slices.Contains(runs, slug) {
			kept[slug] = how
		}
	}
	return kept
}

// pairHasLocalPart reports whether this machine holds any part of slug's pair: its task worktree path, its sibling worktree path, its launchers or either local branch.
// A branch counts when it is named by the slug alone.
func pairHasLocalPart(l *lymcwd.Location, slug string) (bool, error) {
	pair, err := lymcwd.NamedWorktree(l.HubPath, slug)
	if err != nil {
		return false, err
	}
	if pathPresent(WorktreePath(l, slug)) || pathPresent(RecordsWorktreePath(l, slug)) || pathPresent(LauncherDir(pair, slug)) {
		return true, nil
	}
	taskBranch, err := localBranchExists(l.WorktreePath(), slug)
	if err != nil {
		return false, err
	}
	return taskBranch || weftBranchExists(l, RecordsBranchName(slug)), nil
}

// busyWayForward names what lets a removal that found its pair busy go through at the next run.
func busyWayForward(busyOn string) string {
	if busyOn == BusyOnDriver {
		return "wait for the pair's driver to end, which the run's next Parent-Align or Finalize makes certain by stopping it with Halt, then re-run `lym cleanup --apply`"
	}
	return "wait for the holder of the pair's teardown or weft write lock to release it, then re-run `lym cleanup --apply`"
}

// reconcileEnded applies the removal of an ended run to each entry of ended, in slug order, and reports each one in res.Entries as ended.
// ended holds only runs with a batten run record and none this machine's own landing has reached.
// An entry with a local part of its pair is handed to seams.Teardown with the slug and whether an earlier run left a snapshot tag; one with none is reported with no teardown and no network call.
// A busy teardown leaves the pair, and the entry is ended with the busy flag; any other teardown error is on the entry, and no other entry is stopped by it.
// A judge-only scope, and the run scope's own run, whose driver is the caller, tear nothing down.
// A nil Teardown seam tears nothing down either.
// Each teardown's mutations are appended to res's record.
func reconcileEnded(l *lymcwd.Location, scope ReconcileScope, seams OriginSeams, ended EndedSet, res *OriginReport) error {
	for _, slug := range sortedSlugs(ended) {
		entry := EntryOutcome{Slug: slug, State: EntryEnded, How: ended[slug]}
		local, err := pairHasLocalPart(l, slug)
		if err != nil {
			return fmt.Errorf("fabricengine: read whether this machine holds the pair %s: %w", slug, err)
		}
		entry.Local = local

		if local && !scope.JudgeOnly && scope.Run != slug && seams.Teardown != nil {
			earlier, err := SnapshotTagged(l, slug)
			if err != nil {
				return fmt.Errorf("fabricengine: read whether pair %s holds a snapshot tag: %w", slug, err)
			}
			report, err := seams.Teardown(EndedTeardown{Slug: slug, EarlierSnapshot: earlier})
			res.Mutations.Extend(report.Mutations)
			entry.Tags = report.Tags
			entry.Kept = report.Kept
			if err == nil {
				err = report.Error
			}
			switch {
			case report.Busy:
				entry.Busy = true
				entry.WayForward = busyWayForward(report.BusyOn)
			case err != nil:
				entry.Error = err.Error()
			default:
				entry.Removed = true
			}
		}
		res.Entries = append(res.Entries, entry)
	}
	return nil
}

// endedLocalSlugs returns the slugs of the entries a teardown was applied to, other than run's own.
func endedLocalSlugs(entries []EntryOutcome, run string) map[string]bool {
	slugs := map[string]bool{}
	for _, entry := range entries {
		if entry.Local && entry.Slug != run {
			slugs[entry.Slug] = true
		}
	}
	return slugs
}

// withoutRemovedPairs returns branches without those of a pair an ended entry shows removed.
func withoutRemovedPairs(branches []BranchOutcome, entries []EntryOutcome) []BranchOutcome {
	removed := map[string]bool{}
	for _, entry := range entries {
		if entry.Removed {
			removed[entry.Slug] = true
		}
	}
	if len(removed) == 0 {
		return branches
	}
	kept := make([]BranchOutcome, 0, len(branches))
	for _, branch := range branches {
		if !removed[branch.Slug] {
			kept = append(kept, branch)
		}
	}
	return kept
}

// mergeEndedEntries returns classified with each slug an ended entry covers taken from ended, and the ended entries of no classified slug after them.
func mergeEndedEntries(classified, ended []EntryOutcome) []EntryOutcome {
	merged := slices.Clone(classified)
	for _, entry := range ended {
		if i := slices.IndexFunc(merged, func(candidate EntryOutcome) bool { return candidate.Slug == entry.Slug }); i >= 0 {
			merged[i] = entry
			continue
		}
		merged = append(merged, entry)
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Slug < merged[j].Slug })
	return merged
}

// clearStalePushFailures removes every failed-push record of the hub whose branch is no longer local.
func clearStalePushFailures(hubPath string) error {
	failures, err := ListFailedPushes(hubPath)
	if err != nil {
		return err
	}
	for _, failure := range failures {
		local := false
		if _, statErr := os.Stat(failure.RepoPath); statErr == nil {
			if local, err = gitrepo.New(failure.RepoPath).BranchExists(failure.Branch); err != nil {
				return err
			}
		}
		if local {
			continue
		}
		if err := ClearPushFailure(hubPath, failure.Repo, failure.Branch); err != nil {
			return err
		}
	}
	return nil
}
