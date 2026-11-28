// originreconcileended.go applies the origin reconcile's removal of ended runs.
// It reads which runs have a record and runs the teardown rule over the ended ones, through the injected seam.
// It also clears the failed-push records of branches that are no longer local.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
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
// When scope names a run, only that run is read.
func recordedRuns(l *lymcwd.Location, scope fabriclayout.ReconcileScope) ([]string, error) {
	prime, err := resolvePrime(l)
	if err != nil {
		return nil, err
	}
	candidates := []string{scope.Run}
	if scope.Run == "" {
		paths := installedRunRecordPaths.Load()
		dirs, err := os.ReadDir(paths.RunsRoot(prime))
		if os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		candidates = candidates[:0]
		for _, dir := range dirs {
			if dir.IsDir() && !paths.Reserved(dir.Name()) {
				candidates = append(candidates, dir.Name())
			}
		}
	}

	var runs []string
	for _, slug := range candidates {
		found, err := runRecorded(prime, slug)
		if err != nil {
			return nil, err
		}
		if found {
			runs = append(runs, slug)
		}
	}
	return runs, nil
}

// runRecorded reports whether the prime holds a run status record of slug.
// An absent file is not recorded, and a record the status codec does not read is an error.
func runRecorded(prime *lymcwd.Location, slug string) (bool, error) {
	path := installedRunRecordPaths.Load().StatusFile(prime, slug)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	recorded, err := installedStatusCodec.Load().Recorded(data)
	if err != nil {
		return false, fmt.Errorf("decode %s: %w", path, err)
	}
	return recorded, nil
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

// pairHasLocalPart reports whether this machine holds any part of slug's pair.
// The parts are its task worktree path, its sibling worktree path or either local branch.
// A branch counts when it is named by the slug alone.
func pairHasLocalPart(l *lymcwd.Location, slug string) (bool, error) {
	if pathPresent(WorktreePath(l, slug)) || pathPresent(RecordsWorktreePath(l, slug)) {
		return true, nil
	}
	taskBranch, err := localBranchExists(l.WorktreePath(), slug)
	if err != nil {
		return false, err
	}
	return taskBranch || weftBranchExists(l, fabriclayout.RecordsBranchName(slug)), nil
}

// reconcileEnded runs the teardown rule over the runs of ended and appends each one's entry to res.Entries.
// ended holds only runs with a batten run record and none this machine's own landing has reached.
// Each teardown's mutations are appended to res's record.
func reconcileEnded(l *lymcwd.Location, scope fabriclayout.ReconcileScope, seams OriginSeams, ended EndedSet, res *fabriclayout.OriginReport) error {
	earlier := map[string]bool{}
	reports := map[string]*fabriclayout.TeardownReport{}
	read := func(_ fabriccore.Phase, fields []fabriccore.Field, snapshot *fabriccore.Snapshot) error {
		for _, field := range fields {
			switch field {
			case fabriccore.FieldEndedRuns:
				facts, err := readEndedFacts(l, scope, seams, ended)
				if err != nil {
					return err
				}
				snapshot.Ended = facts
				for _, run := range facts.Runs {
					earlier[run.Slug] = run.EarlierSnapshot
				}
			case fabriccore.FieldTeardownReports:
				for i := range snapshot.Ended.Runs {
					snapshot.Ended.Runs[i].Report = reports[snapshot.Ended.Runs[i].Slug]
				}
			}
		}
		return nil
	}
	runners := map[fabriccore.ActionKind]actionRunner{
		fabriccore.ActionSeamCall: func(action fabriccore.Action) error {
			report, err := seams.Teardown(fabriclayout.EndedTeardown{Slug: action.Ref, EarlierSnapshot: earlier[action.Ref]})
			res.Mutations = fabriclayout.MutationsOf(append(res.Mutations.Entries(), report.Mutations.Entries()...))
			if err != nil {
				report.Error = err
			}
			reports[action.Ref] = &report
			return report.Error
		},
	}

	start := fabriccore.Answer{Phase: fabriccore.PhasePreLock, Read: []fabriccore.Field{fabriccore.FieldEndedRuns}}
	snapshot, err := runRule(fabriccore.EndedTeardowns, start, read, nil, runners, nil)
	if err != nil {
		return err
	}
	res.Entries = append(res.Entries, fabriccore.EndedEntries(snapshot)...)
	return nil
}

// readEndedFacts reads, for each run of ended in slug order, what the teardown rule decides on.
// The snapshot tags are read only of a pair that is torn down.
func readEndedFacts(l *lymcwd.Location, scope fabriclayout.ReconcileScope, seams OriginSeams, ended EndedSet) (fabriccore.EndedFacts, error) {
	facts := fabriccore.EndedFacts{Scope: scope, Seam: seams.Teardown != nil}
	for _, slug := range sortedSlugs(ended) {
		run := fabriccore.EndedRun{Slug: slug, How: ended[slug]}
		local, err := pairHasLocalPart(l, slug)
		if err != nil {
			return facts, fmt.Errorf("fabricengine: read whether this machine holds the pair %s: %w", slug, err)
		}
		run.Local = local
		if facts.TearsDown(run) {
			if run.EarlierSnapshot, err = SnapshotTagged(l, slug); err != nil {
				return facts, fmt.Errorf("fabricengine: read whether pair %s holds a snapshot tag: %w", slug, err)
			}
		}
		facts.Runs = append(facts.Runs, run)
	}
	return facts, nil
}

// endedLocalSlugs returns the slugs of the entries a teardown was applied to, other than run's own.
func endedLocalSlugs(entries []fabriclayout.EntryOutcome, run string) map[string]bool {
	slugs := map[string]bool{}
	for _, entry := range entries {
		if entry.Local && entry.Slug != run {
			slugs[entry.Slug] = true
		}
	}
	return slugs
}

// clearStalePushFailures removes every failed-push record of the hub whose branch is no longer local.
func clearStalePushFailures(hubPath string) error {
	failures, err := ListFailedPushes(hubPath)
	if err != nil {
		return err
	}
	local := map[string]bool{}
	for _, failure := range failures {
		if _, statErr := os.Stat(failure.RepoPath); statErr != nil {
			continue
		}
		exists, err := gitrepo.New(failure.RepoPath).BranchExists(failure.Branch)
		if err != nil {
			return err
		}
		if exists {
			local[fabriccore.LocalBranchKey(failure.RepoPath, failure.Branch)] = true
		}
	}
	for _, failure := range fabriccore.StalePushFailures(failures, local) {
		if err := ClearPushFailure(hubPath, failure.Repo, failure.Branch); err != nil {
			return err
		}
	}
	return nil
}
