// remove.go holds the seam-driven body of `lyx fabric remove`: the run-scope origin reconcile of the slug, the verdict it gives, and the choice between the local removal of an ended or foreign run and the ordinary removal.

package fabriccli

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
)

// The repository sides a branch outcome names, as the reconcile fills them.
const (
	sideRecords = "records"
	sideBolt    = "bolt"
)

// localReading is what the local board says about a slug's run, read when origin cannot judge it.
// ownedElsewhere is whether the entry's effective owner is another machine, and done whether the entry reads done.
type localReading struct {
	ownedElsewhere bool
	done           bool
}

// remover carries the seams `lyx fabric remove` wires and its test fakes.
// exemptAll lets every live pause entry pass this process's network gate and returns the restore.
// gone reports whether the slug's task worktree is gone, reconcile runs the run-scope origin reconcile of the slug, local reads the local board for the slug, and teardown removes the pair.
type remover struct {
	exemptAll func() (restore func())
	gone      func(slug string) (bool, error)
	reconcile func(slug string) (fabricengine.OriginReport, error)
	local     func(slug string) (localReading, error)
	teardown  func(ctx context.Context, req pairteardown.Request) (pairteardown.Result, error)
}

// run removes the pair of slug.
// The gate exemption is taken first and restored on return, so the removal's own network calls pass under any pause.
// A pair whose task worktree is gone takes no reconcile, since there is no pair worktree to reconcile: the local board decides its verdict, as for an unreachable origin.
// An ended or owned_elsewhere verdict removes the pair locally, dropping a requested --remote and naming it in the result;
// any other verdict removes it with the quiet wait and the typed flags.
// A refusal or a reconcile error ends nothing.
func (r remover) run(ctx context.Context, slug string, force, remote bool) (pairteardown.Result, error) {
	restore := r.exemptAll()
	defer restore()

	gone, err := r.gone(slug)
	if err != nil {
		return pairteardown.Result{}, err
	}

	var report fabricengine.OriginReport
	if gone {
		report.Unreachable = true
	} else {
		report, err = r.reconcile(slug)
		if err != nil {
			return pairteardown.Result{}, fmt.Errorf("origin reconcile of %q failed: %w; fix the cause it names, then run lyx fabric remove %s again", slug, err, slug)
		}
	}

	var local localReading
	if report.Unreachable {
		local, err = r.local(slug)
		if err != nil {
			return pairteardown.Result{}, fmt.Errorf("read the local board for %q: %w; fix the cause it names, then run lyx fabric remove %s again", slug, err, slug)
		}
	}

	verdict, err := removeVerdict(report, slug, local)
	if err != nil {
		return pairteardown.Result{}, err
	}

	if verdict != fabricengine.EntryEnded && verdict != fabricengine.EntryOwnedElsewhere {
		return r.teardown(ctx, pairteardown.Request{
			Slug:           slug,
			Force:          force,
			Remote:         remote,
			QuietWait:      pairteardown.RemoveQuietWait,
			RefuseWhenBusy: true,
		})
	}

	res, err := r.teardown(ctx, pairteardown.Request{
		Slug:           slug,
		Force:          force,
		Local:          true,
		Verdict:        verdict,
		RefuseWhenBusy: true,
	})
	if remote {
		res.Removal.RemoteSkippedReason = fmt.Sprintf("the origin reconcile read the entry %s, so the removal was local and --remote was dropped", verdict)
	}
	return res, err
}

// removeVerdict turns a reconcile report into the verdict on the slug's entry.
// An ended or owned_elsewhere entry is returned as it is, and an active entry or none gives the empty verdict, the ordinary removal.
// A stopped entry is refused with the way forward of the first branch that stops it, then the re-run of the removal;
// the one exception is a stop made only by gone branches of the slug, which gives the empty verdict, since the removal is what clears a gone branch.
// With an unreachable origin the local reading decides: another machine's ownership reads owned_elsewhere, a done entry reads ended, and neither gives the empty verdict.
func removeVerdict(report fabricengine.OriginReport, slug string, local localReading) (verdict fabricengine.EntryState, refusal error) {
	if report.Unreachable {
		switch {
		case local.ownedElsewhere:
			return fabricengine.EntryOwnedElsewhere, nil
		case local.done:
			return fabricengine.EntryEnded, nil
		}
		return "", nil
	}

	for _, entry := range report.Entries {
		if entry.Slug != slug {
			continue
		}
		switch entry.State {
		case fabricengine.EntryEnded, fabricengine.EntryOwnedElsewhere:
			return entry.State, nil
		case fabricengine.EntryStopped:
			return "", stoppedRemoveRefusal(slug, report.Branches)
		}
		return "", nil
	}
	return "", nil
}

// stoppedRemoveRefusal builds the refusal for a stopped entry from the first branch that stops it, or returns nil when only gone branches of the slug do.
// The entry's own way forward is never used: it can hold the gone branch's text, which names the refused verb.
// A busy records branch of the slug and a busy shared branch stop nothing here, as the reconcile judges them.
func stoppedRemoveRefusal(slug string, branches []fabricengine.BranchOutcome) error {
	for _, branch := range branches {
		if !stopsRemoval(branch.State) {
			continue
		}
		own := branch.Slug == slug
		shared := branch.Repo == sideBolt || (branch.Repo == sideRecords && branch.Prime)
		if !own && !shared {
			continue
		}
		if branch.State == fabricengine.BranchBusy && ((own && branch.Repo == sideRecords) || shared) {
			continue
		}
		if branch.State == fabricengine.BranchGone && own {
			continue
		}
		return fmt.Errorf("cannot remove %q: branch %s is %s against origin; %s; then run lyx fabric remove %s again", slug, branch.Branch, branch.State, branch.WayForward, slug)
	}
	return nil
}

// stopsRemoval reports whether a branch in this state stops its entry.
func stopsRemoval(state fabricengine.BranchState) bool {
	switch state {
	case fabricengine.BranchRewound, fabricengine.BranchRewritten, fabricengine.BranchDiverged, fabricengine.BranchGone,
		fabricengine.BranchDirty, fabricengine.BranchBusy, fabricengine.BranchLive, fabricengine.BranchAnchorStopped:
		return true
	}
	return false
}

// readLocalReading reads the local board for slug: whether its effective owner is another machine and whether its entry reads done.
// An unreadable board, or an owner the board cannot resolve, is returned.
func readLocalReading(prime *lyxcwd.Location, slug string) (localReading, error) {
	options := pairteardown.HubSeamOptions(prime)
	machine, err := options.Machine()
	if err != nil {
		return localReading{}, err
	}
	owners, err := boardengine.ReadOwners(fabricengine.BoardDir(prime.HubPath))
	if err != nil {
		return localReading{}, err
	}
	owner, err := owners.Effective(slug, options.ParentOf)
	if err != nil {
		return localReading{}, err
	}
	board, err := boardengine.OpenHub(prime.HubPath)
	if err != nil {
		return localReading{}, err
	}
	task, found, err := board.GetTask(slug)
	if err != nil {
		return localReading{}, err
	}
	return localReading{
		ownedElsewhere: boardengine.OwnedElsewhere(owner, machine),
		done:           found && task.Status != nil && *task.Status == "done",
	}, nil
}

// reconcileForRemove runs the run-scope origin reconcile of slug from the prime, with the seams pairteardown builds for a hub, and logs each entry of its mutation record.
// A hub whose warp or weft repository has no origin remote has nothing to judge against, so it reads as an unreachable origin and the local board decides.
// The run scope tears no pair down, so the report never shows the slug's pair already removed.
func reconcileForRemove(prime *lyxcwd.Location, td *pairteardown.Teardown, slug string) (fabricengine.OriginReport, error) {
	recordsRoot, err := fabricengine.RecordsRepoRoot(prime)
	if err != nil {
		return fabricengine.OriginReport{}, err
	}
	for _, repoPath := range []string{prime.WorktreePath(), recordsRoot} {
		if _, err := gitrepo.New(repoPath).RemoteURL("origin"); err != nil {
			logger.Info("fabriccli: no origin remote to reconcile against", "slug", slug, "repo", repoPath, "cause", err)
			return fabricengine.OriginReport{Unreachable: true}, nil
		}
	}

	report, err := fabricengine.ReconcileOrigin(prime, fabricengine.ReconcileScope{Run: slug}, td.OriginSeams(pairteardown.HubSeamOptions(prime)), fabricengine.ReconcileOptions{})
	for _, mutation := range report.Mutations.Entries() {
		logger.Info("fabriccli: origin reconcile mutation", "slug", slug, "kind", string(mutation.Kind), "target", mutation.Target, "detail", mutation.Detail)
	}
	return report, err
}
