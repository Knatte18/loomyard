// originjudge.go reads what judging this machine's branches against origin needs, and runs the rule over it.

package fabricengine

import (
	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// reconcileReader reads the fields of a reconcile's snapshot and keeps the targets it read.
type reconcileReader struct {
	l     *lymcwd.Location
	scope fabriclayout.ReconcileScope
	opts  ReconcileOptions
	// targets holds, index for index, the targets of the snapshot's reconcile facts.
	targets []originTarget
}

// read fills the named fields of snapshot.
// Only a transport failure of the read of origin is a *fieldReadError, so that alone is retried;
// every other failure ends the run with its own message.
func (r *reconcileReader) read(_ fabriccore.Phase, fields []fabriccore.Field, snapshot *fabriccore.Snapshot) error {
	facts := &snapshot.Reconcile
	for _, field := range fields {
		var err error
		switch field {
		case fabriccore.FieldSeams:
			snapshot.Seams = suppliedSeams()
		case fabriccore.FieldReconcileTargets:
			err = r.readTargets(facts)
		case fabriccore.FieldOriginRefs:
			err = r.readOriginHeads(snapshot)
		case fabriccore.FieldMovedHeads:
			err = r.fetchMoved(facts)
		case fabriccore.FieldBranchFacts:
			err = r.readBranches(facts)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *reconcileReader) readTargets(facts *fabriccore.ReconcileFacts) error {
	targets, pairs, warpDir, recordsDir, pairAbsent, err := originTargets(r.l, r.scope)
	if err != nil {
		return err
	}
	r.targets = targets
	*facts = fabriccore.ReconcileFacts{
		Scope:         r.scope,
		Pairs:         pairs,
		CodeDir:       warpDir,
		RecordsDir:    recordsDir,
		PairAbsent:    pairAbsent,
		Attempts:      r.opts.attempts(),
		RetryInterval: r.opts.RetryInterval,
	}
	for _, target := range targets {
		facts.Targets = append(facts.Targets, fabriccore.BranchFacts{
			Repo:         target.side,
			Branch:       target.branch,
			Worktree:     target.worktree,
			CodeWorktree: target.codeWorktree,
			Slug:         target.slug,
			Prime:        target.prime,
			Tracking:     target.tracking,
		})
	}
	return nil
}

func (r *reconcileReader) readOriginHeads(snapshot *fabriccore.Snapshot) error {
	warpHeads, recordsHeads, err := lsRemoteOnce(snapshot.Reconcile.CodeDir, snapshot.Reconcile.RecordsDir, r.opts)
	if gitexec.IsTransportFailure(err) {
		return &fieldReadError{Field: fabriccore.FieldOriginRefs, Err: err}
	}
	if err != nil {
		return err
	}
	snapshot.Stores = map[fabriccore.Store]fabriccore.StoreRefs{}
	for store, heads := range map[fabriccore.Store]map[string]string{fabriccore.StoreCode: warpHeads, fabriccore.StoreRecords: recordsHeads} {
		refs := fabriccore.Refs{}
		for branch, commit := range heads {
			refs["refs/heads/"+branch] = commit
		}
		snapshot.Stores[store] = fabriccore.StoreRefs{Origin: refs, OriginRead: true}
	}
	for i, target := range r.targets {
		heads := recordsHeads
		if target.side == fabriclayout.RepoCode {
			heads = warpHeads
		}
		snapshot.Reconcile.Targets[i].OriginHead = heads[target.branch]
	}
	return nil
}

// fetchMoved fetches the heads origin moved past their tracking refs, in one fetch per repository.
func (r *reconcileReader) fetchMoved(facts *fabriccore.ReconcileFacts) error {
	moved := map[string]map[string]string{facts.CodeDir: {}, facts.RecordsDir: {}}
	for i, target := range r.targets {
		if head := facts.Targets[i].OriginHead; head != "" && head != target.tracking {
			moved[target.repoDir][target.branch] = head
		}
	}
	for _, dir := range []string{facts.CodeDir, facts.RecordsDir} {
		if err := fetchMovedHeads(dir, moved[dir], r.scope.JudgeOnly, r.opts); err != nil {
			return err
		}
	}
	facts.Fetched = true
	return nil
}

func (r *reconcileReader) readBranches(facts *fabriccore.ReconcileFacts) error {
	for i, target := range r.targets {
		r.opts.verdictHook()
		if err := judgeBranch(target.repoDir, &facts.Targets[i]); err != nil {
			return err
		}
	}
	facts.Judged = true
	return nil
}
